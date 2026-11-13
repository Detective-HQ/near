// Package defillama implements an authenticated, bounded HTTP connector for
// public DeFiLlama analytics endpoints.
package defillama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"universal-defi-intelligence/internal/analytics/domain"
)

const (
	DefaultAPIBaseURL        = "https://api.llama.fi"
	DefaultStablecoinBaseURL = "https://stablecoins.llama.fi"
	MaxResponseBytes         = 4_000_000
	MethodologyVersion       = "defillama-v1"
	DefaultTimeout           = 10 * time.Second
	DefaultStaleThreshold    = 36 * time.Hour
)

// Config configures the DeFiLlama client.
type Config struct {
	APIBaseURL        string
	StablecoinBaseURL string
	Timeout           time.Duration
	StaleThreshold    time.Duration
	HTTPClient        *http.Client
	Logger            *slog.Logger
	Now               func() time.Time
}

// Client fetches and normalizes public DeFi metrics from DeFiLlama.
type Client struct {
	apiBaseURL        string
	stablecoinBaseURL string
	timeout           time.Duration
	staleThreshold    time.Duration
	httpClient        *http.Client
	logger            *slog.Logger
	now               func() time.Time

	mu     sync.RWMutex
	cached map[string]*cachedSnapshot
}

type cachedSnapshot struct {
	snapshot  domain.Snapshot
	expiresAt time.Time
}

// New creates an initialized DeFiLlama connector.
func New(cfg Config) *Client {
	apiURL := strings.TrimRight(strings.TrimSpace(cfg.APIBaseURL), "/")
	if apiURL == "" {
		apiURL = DefaultAPIBaseURL
	}
	stableURL := strings.TrimRight(strings.TrimSpace(cfg.StablecoinBaseURL), "/")
	if stableURL == "" {
		stableURL = DefaultStablecoinBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	staleThreshold := cfg.StaleThreshold
	if staleThreshold <= 0 {
		staleThreshold = DefaultStaleThreshold
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}

	return &Client{
		apiBaseURL:        apiURL,
		stablecoinBaseURL: stableURL,
		timeout:           timeout,
		staleThreshold:    staleThreshold,
		httpClient:        httpClient,
		logger:            logger,
		now:               now,
		cached:            make(map[string]*cachedSnapshot),
	}
}

// Name implements connectors.Connector.
func (c *Client) Name() string {
	return "defillama"
}

// Fetch retrieves and aggregates metrics for the target chain.
func (c *Client) Fetch(ctx context.Context, chain string) (domain.Snapshot, error) {
	chain = strings.TrimSpace(chain)
	if chain == "" {
		chain = "near"
	}
	chainKey := strings.ToLower(chain)

	now := c.now()
	retrievedAt := now

	// 1. Fetch chain catalogue to verify chain name and get current TVL
	catalogueChainName, currentTVL, catalogueErr := c.fetchChainCatalogue(ctx, chainKey)
	canonicalChain := catalogueChainName
	if canonicalChain == "" {
		canonicalChain = strings.Title(chainKey)
	}

	// 2. Fetch TVL history
	series, latestTVL, tvlObservedAt, tvlErr := c.fetchHistoricalTVL(ctx, canonicalChain)
	if currentTVL == nil && latestTVL != nil {
		currentTVL = latestTVL
	}

	// 3. Fetch DEX 24h volume
	dexVolume, dexURL, dexErr := c.fetchDEXVolume(ctx, canonicalChain)

	// 4. Fetch Stablecoin supply
	stableSupply, stableURL, stableErr := c.fetchStablecoinSupply(ctx, canonicalChain)

	// 5. Fetch Protocol fees 24h
	fees24h, feesURL, feesErr := c.fetchOverviewMetric(ctx, canonicalChain, "dailyFees")

	// 6. Fetch Protocol revenue 24h
	revenue24h, revenueURL, revenueErr := c.fetchOverviewMetric(ctx, canonicalChain, "dailyRevenue")

	metrics := make([]domain.MetricObservation, 0, 5)

	// TVL observation
	tvlStatus := domain.StatusAvailable
	tvlErrMsg := ""
	if currentTVL == nil {
		tvlStatus = domain.StatusUnavailable
		if catalogueErr != nil {
			tvlErrMsg = catalogueErr.Error()
		} else if tvlErr != nil {
			tvlErrMsg = tvlErr.Error()
		} else {
			tvlErrMsg = "No usable TVL observation returned."
		}
	} else if !tvlObservedAt.IsZero() && now.Sub(tvlObservedAt) > c.staleThreshold {
		tvlStatus = domain.StatusStale
	}

	tvlSourceURL := fmt.Sprintf("%s/v2/historicalChainTvl/%s", c.apiBaseURL, url.PathEscape(canonicalChain))
	metrics = append(metrics, domain.MetricObservation{
		MetricKey:          "tvl",
		ChainKey:           chainKey,
		Value:              currentTVL,
		Unit:               "USD",
		SourceName:         "DeFiLlama",
		SourceURL:          tvlSourceURL,
		Status:             tvlStatus,
		MethodologyVersion: MethodologyVersion,
		ObservedAt:         tvlObservedAt,
		RetrievedAt:        retrievedAt,
		Label:              "Total value locked",
		Definition:         "Aggregate value of assets deposited in protocols on this chain.",
		Scope:              canonicalChain + " ecosystem",
		Error:              tvlErrMsg,
	})

	// DEX Volume
	dexStatus := domain.StatusAvailable
	dexErrMsg := ""
	if dexVolume == nil {
		dexStatus = domain.StatusUnavailable
		if dexErr != nil {
			dexErrMsg = dexErr.Error()
		} else {
			dexErrMsg = "No DEX volume data returned."
		}
	}
	metrics = append(metrics, domain.MetricObservation{
		MetricKey:          "dex_volume_24h",
		ChainKey:           chainKey,
		Value:              dexVolume,
		Unit:               "USD / rolling 24 hours",
		SourceName:         "DeFiLlama",
		SourceURL:          dexURL,
		Status:             dexStatus,
		MethodologyVersion: MethodologyVersion,
		ObservedAt:         now,
		RetrievedAt:        retrievedAt,
		Label:              "DEX volume",
		Definition:         "Provider-reported aggregate DEX volume over rolling 24-hour window.",
		Scope:              canonicalChain + " DeFi",
		Error:              dexErrMsg,
	})

	// Stablecoin Supply
	stableStatus := domain.StatusAvailable
	stableErrMsg := ""
	if stableSupply == nil {
		stableStatus = domain.StatusUnavailable
		if stableErr != nil {
			stableErrMsg = stableErr.Error()
		} else {
			stableErrMsg = "No stablecoin supply data returned."
		}
	}
	metrics = append(metrics, domain.MetricObservation{
		MetricKey:          "stablecoin_supply",
		ChainKey:           chainKey,
		Value:              stableSupply,
		Unit:               "USD",
		SourceName:         "DeFiLlama Stablecoins",
		SourceURL:          stableURL,
		Status:             stableStatus,
		MethodologyVersion: MethodologyVersion,
		ObservedAt:         now,
		RetrievedAt:        retrievedAt,
		Label:              "Stablecoin supply",
		Definition:         "Provider-reported circulating stablecoin market cap on this chain.",
		Scope:              canonicalChain + " ecosystem",
		Error:              stableErrMsg,
	})

	// Protocol Fees 24h
	feesStatus := domain.StatusAvailable
	feesErrMsg := ""
	if fees24h == nil {
		feesStatus = domain.StatusUnavailable
		if feesErr != nil {
			feesErrMsg = feesErr.Error()
		} else {
			feesErrMsg = "No protocol fees data returned."
		}
	}
	metrics = append(metrics, domain.MetricObservation{
		MetricKey:          "protocol_fees_24h",
		ChainKey:           chainKey,
		Value:              fees24h,
		Unit:               "USD / rolling 24 hours",
		SourceName:         "DeFiLlama Fees",
		SourceURL:          feesURL,
		Status:             feesStatus,
		MethodologyVersion: MethodologyVersion,
		ObservedAt:         now,
		RetrievedAt:        retrievedAt,
		Label:              "Protocol fees",
		Definition:         "Aggregate fees collected by protocols over rolling 24 hours.",
		Scope:              canonicalChain + " DeFi",
		Error:              feesErrMsg,
	})

	// Protocol Revenue 24h
	revenueStatus := domain.StatusAvailable
	revenueErrMsg := ""
	if revenue24h == nil {
		revenueStatus = domain.StatusUnavailable
		if revenueErr != nil {
			revenueErrMsg = revenueErr.Error()
		} else {
			revenueErrMsg = "No protocol revenue data returned."
		}
	}
	metrics = append(metrics, domain.MetricObservation{
		MetricKey:          "protocol_revenue_24h",
		ChainKey:           chainKey,
		Value:              revenue24h,
		Unit:               "USD / rolling 24 hours",
		SourceName:         "DeFiLlama Fees",
		SourceURL:          revenueURL,
		Status:             revenueStatus,
		MethodologyVersion: MethodologyVersion,
		ObservedAt:         now,
		RetrievedAt:        retrievedAt,
		Label:              "Protocol revenue",
		Definition:         "Aggregate revenue retained by protocols over rolling 24 hours.",
		Scope:              canonicalChain + " DeFi",
		Error:              revenueErrMsg,
	})

	// Aggregate snapshot status
	availableCount := 0
	staleCount := 0
	for _, m := range metrics {
		if m.Value != nil {
			availableCount++
		}
		if m.Status == domain.StatusStale {
			staleCount++
		}
	}

	overallStatus := domain.StatusAvailable
	if availableCount == 0 {
		overallStatus = domain.StatusUnavailable
	} else if staleCount > 0 {
		overallStatus = domain.StatusStale
	} else if availableCount < len(metrics) {
		overallStatus = domain.StatusPartial
	}

	notice := "Live public data from DeFiLlama. Each metric retains source URL and timestamp."
	if overallStatus == domain.StatusPartial {
		notice = "Some metrics could not be retrieved from DeFiLlama. No fallback fixtures substituted."
	} else if overallStatus == domain.StatusUnavailable {
		notice = "DeFiLlama endpoints currently unreachable. No fallback fixtures substituted."
	}

	snapshot := domain.Snapshot{
		ID:                 fmt.Sprintf("snap_%s_%d", chainKey, now.Unix()),
		Mode:               "live",
		Provider:           "DeFiLlama",
		Chain:              canonicalChain,
		Status:             overallStatus,
		Notice:             notice,
		MethodologyVersion: MethodologyVersion,
		CapturedAt:         now,
		RetrievedAt:        retrievedAt,
		Metrics:            metrics,
		TVLSeries:          series,
	}

	c.mu.Lock()
	c.cached[chainKey] = &cachedSnapshot{
		snapshot:  snapshot,
		expiresAt: now.Add(5 * time.Minute),
	}
	c.mu.Unlock()

	return snapshot, nil
}

func (c *Client) fetchChainCatalogue(ctx context.Context, chainKey string) (string, *float64, error) {
	reqURL := c.apiBaseURL + "/v2/chains"
	body, err := c.get(ctx, reqURL)
	if err != nil {
		return "", nil, err
	}

	var chains []struct {
		Name string   `json:"name"`
		TVL  *float64 `json:"tvl"`
	}
	if err := json.Unmarshal(body, &chains); err != nil {
		return "", nil, err
	}

	for _, entry := range chains {
		if strings.EqualFold(entry.Name, chainKey) {
			return entry.Name, entry.TVL, nil
		}
	}

	return "", nil, fmt.Errorf("chain %q not found in DeFiLlama catalogue", chainKey)
}

func (c *Client) fetchHistoricalTVL(ctx context.Context, chainName string) ([]domain.SeriesPoint, *float64, time.Time, error) {
	reqURL := fmt.Sprintf("%s/v2/historicalChainTvl/%s", c.apiBaseURL, url.PathEscape(chainName))
	body, err := c.get(ctx, reqURL)
	if err != nil {
		return nil, nil, time.Time{}, err
	}

	var points []struct {
		Date int64   `json:"date"`
		TVL  float64 `json:"tvl"`
	}
	if err := json.Unmarshal(body, &points); err != nil {
		return nil, nil, time.Time{}, err
	}

	if len(points) == 0 {
		return nil, nil, time.Time{}, errors.New("empty historical TVL array")
	}

	// Keep up to recent 90 points
	start := 0
	if len(points) > 90 {
		start = len(points) - 90
	}

	series := make([]domain.SeriesPoint, 0, len(points)-start)
	for i := start; i < len(points); i++ {
		t := time.Unix(points[i].Date, 0).UTC()
		series = append(series, domain.SeriesPoint{
			Date:  t.Format("2006-01-02"),
			Value: points[i].TVL,
		})
	}

	last := points[len(points)-1]
	lastObserved := time.Unix(last.Date, 0).UTC()
	val := last.TVL

	return series, &val, lastObserved, nil
}

func (c *Client) fetchDEXVolume(ctx context.Context, chainName string) (*float64, string, error) {
	reqURL := fmt.Sprintf("%s/overview/dexs/%s?excludeTotalDataChart=true&excludeTotalDataChartBreakdown=true",
		c.apiBaseURL, url.PathEscape(chainName))
	body, err := c.get(ctx, reqURL)
	if err != nil {
		return nil, reqURL, err
	}

	var data struct {
		Total24h *float64 `json:"total24h"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, reqURL, err
	}

	return data.Total24h, reqURL, nil
}

func (c *Client) fetchStablecoinSupply(ctx context.Context, chainName string) (*float64, string, error) {
	reqURL := c.stablecoinBaseURL + "/stablecoinchains"
	body, err := c.get(ctx, reqURL)
	if err != nil {
		return nil, reqURL, err
	}

	var chains []struct {
		Name                string `json:"name"`
		TotalCirculatingUSD *struct {
			PeggedUSD *float64 `json:"peggedUSD"`
		} `json:"totalCirculatingUSD"`
	}
	if err := json.Unmarshal(body, &chains); err != nil {
		return nil, reqURL, err
	}

	for _, entry := range chains {
		if strings.EqualFold(entry.Name, chainName) {
			if entry.TotalCirculatingUSD != nil && entry.TotalCirculatingUSD.PeggedUSD != nil {
				return entry.TotalCirculatingUSD.PeggedUSD, reqURL, nil
			}
		}
	}

	return nil, reqURL, fmt.Errorf("chain %q not found in stablecoin chains", chainName)
}

func (c *Client) fetchOverviewMetric(ctx context.Context, chainName, dataType string) (*float64, string, error) {
	reqURL := fmt.Sprintf("%s/overview/fees/%s?dataType=%s", c.apiBaseURL, url.PathEscape(chainName), url.QueryEscape(dataType))
	body, err := c.get(ctx, reqURL)
	if err != nil {
		return nil, reqURL, err
	}

	var data struct {
		Total24h *float64 `json:"total24h"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, reqURL, err
	}

	return data.Total24h, reqURL, nil
}

func (c *Client) get(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("user-agent", "udi-analytics/1.0")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d from %s", res.StatusCode, reqURL)
	}

	data, err := io.ReadAll(io.LimitReader(res.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxResponseBytes {
		return nil, errors.New("response exceeded max byte limit")
	}

	return data, nil
}
