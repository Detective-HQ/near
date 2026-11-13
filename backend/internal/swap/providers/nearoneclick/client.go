// Package nearoneclick implements the NEAR Intents 1Click provider adapter.
//
// The adapter keeps the partner JWT server-side, bounds every response, applies
// per-request timeouts, validates provider payloads, and maps upstream failures
// to typed domain errors. It never returns upstream response bodies to clients.
package nearoneclick

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

const (
	defaultBaseURL          = "https://1click.chaindefuser.com"
	defaultTimeout          = 10 * time.Second
	defaultMaxResponseBytes = 2_000_000
	maxSlippageBPS          = 5000
	maxAddressLength        = 512
	maxTxHashLength         = 256
	depositModeSimple       = "SIMPLE"
	depositModeMemo         = "MEMO"
)

// Config configures the 1Click client. Zero values fall back to safe defaults.
type Config struct {
	BaseURL          string
	JWT              string
	Timeout          time.Duration
	MaxResponseBytes int64
	QuotePublicKey   string
	Logger           *slog.Logger
	HTTPClient       *http.Client
	Now              func() time.Time
}

// Client is an authenticated 1Click HTTP adapter.
type Client struct {
	root             string
	jwt              string
	timeout          time.Duration
	maxResponseBytes int64
	quotePublicKey   string
	logger           *slog.Logger
	httpClient       *http.Client
	now              func() time.Time
}

// New validates the configuration and returns a 1Click adapter.
func New(cfg Config) (*Client, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil {
		return nil, domain.Wrap(domain.CodeValidationFailed, "The provider base URL is invalid.", err)
	}
	if endpoint.Query().Encode() != "" || endpoint.Fragment != "" {
		return nil, domain.New(domain.CodeValidationFailed,
			"The provider base URL must not include a query or fragment.")
	}
	if endpoint.Scheme != "https" && endpoint.Hostname() != "localhost" && endpoint.Hostname() != "127.0.0.1" {
		return nil, domain.New(domain.CodeValidationFailed, "The provider base URL must use HTTPS.")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if timeout < 100*time.Millisecond {
		timeout = 100 * time.Millisecond
	}
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}

	maxBytes := cfg.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxResponseBytes
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	return &Client{
		root:             strings.TrimRight(endpoint.String(), "/"),
		jwt:              strings.TrimSpace(cfg.JWT),
		timeout:          timeout,
		maxResponseBytes: maxBytes,
		quotePublicKey:   strings.TrimSpace(cfg.QuotePublicKey),
		logger:           logger,
		httpClient:       client,
		now:              now,
	}, nil
}

// Name implements providers.Adapter.
func (c *Client) Name() string { return providerName }

// Capabilities implements providers.Adapter.
func (c *Client) Capabilities() providers.Capabilities {
	return providers.Capabilities{
		Catalogue:             true,
		DryQuote:              true,
		ExecutableQuote:       true,
		Status:                true,
		SubmitDeposit:         true,
		SignatureVerification: c.quotePublicKey != "",
	}
}

// Health implements providers.Adapter.
func (c *Client) Health() providers.Health {
	return providers.Health{
		Configured:            c.jwt != "",
		SignatureVerification: c.quotePublicKey != "",
	}
}

// GetTokens fetches and validates the provider catalogue. The catalogue
// endpoint does not require the partner JWT.
func (c *Client) GetTokens(ctx context.Context) ([]domain.Asset, error) {
	body, _, err := c.request(ctx, http.MethodGet, "/v0/tokens", nil, false)
	if err != nil {
		return nil, err
	}
	return normalizeTokens(body, c.now())
}

// GetDryQuote requests a non-executable quote. The response must not carry
// deposit instructions.
func (c *Client) GetDryQuote(ctx context.Context, req providers.DryQuoteRequest) (providers.QuoteResult, error) {
	payload, exp, err := c.buildQuoteRequest(req, "", "")
	if err != nil {
		return providers.QuoteResult{}, err
	}
	body, _, err := c.request(ctx, http.MethodPost, "/v0/quote", payload, true)
	if err != nil {
		return providers.QuoteResult{}, err
	}
	return normalizeQuote(body, exp, c.quotePublicKey, c.now())
}

// GetExecutableQuote requests a quote with deposit instructions.
func (c *Client) GetExecutableQuote(ctx context.Context, req providers.ExecutableQuoteRequest) (providers.QuoteResult, error) {
	if req.Deadline.IsZero() || !req.Deadline.After(c.now()) {
		return providers.QuoteResult{}, domain.New(domain.CodeInvalidDeadline,
			"The requested swap deadline is invalid.")
	}
	mode := strings.TrimSpace(req.DepositMode)
	if mode == "" {
		mode = depositModeSimple
	}
	if mode != depositModeSimple && mode != depositModeMemo {
		return providers.QuoteResult{}, domain.New(domain.CodeInvalidDepositMode,
			"The deposit mode is not supported.")
	}
	deadline := req.Deadline.UTC().Format(time.RFC3339)

	payload, exp, err := c.buildQuoteRequest(req.DryQuoteRequest, deadline, mode)
	if err != nil {
		return providers.QuoteResult{}, err
	}
	body, _, err := c.request(ctx, http.MethodPost, "/v0/quote", payload, true)
	if err != nil {
		return providers.QuoteResult{}, err
	}
	return normalizeQuote(body, exp, c.quotePublicKey, c.now())
}

// GetStatus polls the provider for a deposit status.
func (c *Client) GetStatus(ctx context.Context, req providers.StatusRequest) (providers.StatusResult, error) {
	address := strings.TrimSpace(req.DepositAddress)
	if address == "" || len(address) > maxAddressLength {
		return providers.StatusResult{}, domain.New(domain.CodeInvalidDeposit,
			"A valid provider deposit address is required.")
	}
	memo := strings.TrimSpace(req.DepositMemo)
	query := url.Values{}
	query.Set("depositAddress", address)
	if memo != "" {
		query.Set("depositMemo", memo)
	}

	body, _, err := c.request(ctx, http.MethodGet, "/v0/status?"+query.Encode(), nil, true)
	if err != nil {
		return providers.StatusResult{}, err
	}
	return normalizeStatus(body, c.now())
}

type quoteRequestBody struct {
	Dry               bool   `json:"dry"`
	SwapType          string `json:"swapType"`
	SlippageTolerance int    `json:"slippageTolerance"`
	OriginAsset       string `json:"originAsset"`
	DepositType       string `json:"depositType"`
	DestinationAsset  string `json:"destinationAsset"`
	Amount            string `json:"amount"`
	RefundTo          string `json:"refundTo"`
	RefundType        string `json:"refundType"`
	Recipient         string `json:"recipient"`
	RecipientType     string `json:"recipientType"`
	Deadline          string `json:"deadline,omitempty"`
	DepositMode       string `json:"depositMode,omitempty"`
}

func (c *Client) buildQuoteRequest(
	req providers.DryQuoteRequest,
	deadline, depositMode string,
) (quoteRequestBody, quoteExpectation, error) {
	origin := req.OriginAsset
	destination := req.DestinationAsset
	if strings.TrimSpace(origin.ID) == "" || strings.TrimSpace(destination.ID) == "" {
		return quoteRequestBody{}, quoteExpectation{}, domain.New(domain.CodeUnknownAsset,
			"Choose two assets from the live provider catalogue.")
	}
	if origin.ID == destination.ID {
		return quoteRequestBody{}, quoteExpectation{}, domain.New(domain.CodeInvalidAssetPair,
			"Choose two different assets from the live provider catalogue.")
	}
	recipient := strings.TrimSpace(req.Recipient)
	if recipient == "" || len(recipient) > 256 {
		return quoteRequestBody{}, quoteExpectation{}, domain.New(domain.CodeRecipientRequired,
			"Enter a destination address for the quote.")
	}
	refundTo := strings.TrimSpace(req.RefundTo)
	if refundTo == "" || len(refundTo) > 256 {
		return quoteRequestBody{}, quoteExpectation{}, domain.New(domain.CodeRefundRequired,
			"Enter a refund address for the quote.")
	}
	if req.AmountAtomic == nil || req.AmountAtomic.Sign() <= 0 {
		return quoteRequestBody{}, quoteExpectation{}, domain.New(domain.CodeInvalidAmount,
			"Amount must be a positive integer in the asset's smallest unit.")
	}
	if req.SlippageBPS < 0 || req.SlippageBPS > maxSlippageBPS {
		return quoteRequestBody{}, quoteExpectation{}, domain.New(domain.CodeInvalidSlippage,
			"Slippage must be between 0 and 5000 basis points.")
	}

	dry := deadline == ""
	payload := quoteRequestBody{
		Dry:               dry,
		SwapType:          "EXACT_INPUT",
		SlippageTolerance: req.SlippageBPS,
		OriginAsset:       origin.ID,
		DepositType:       "ORIGIN_CHAIN",
		DestinationAsset:  destination.ID,
		Amount:            req.AmountAtomic.String(),
		RefundTo:          refundTo,
		RefundType:        "ORIGIN_CHAIN",
		Recipient:         recipient,
		RecipientType:     "DESTINATION_CHAIN",
		Deadline:          deadline,
		DepositMode:       depositMode,
	}
	exp := quoteExpectation{
		OriginAsset:      origin,
		DestinationAsset: destination,
		AmountAtomic:     req.AmountAtomic,
		Recipient:        recipient,
		RefundTo:         refundTo,
		SlippageBPS:      req.SlippageBPS,
		Dry:              dry,
		Deadline:         deadline,
		DepositMode:      depositMode,
	}
	return payload, exp, nil
}

func (c *Client) request(
	ctx context.Context,
	method, path string,
	body any,
	authenticated bool,
) ([]byte, http.Header, error) {
	if authenticated && c.jwt == "" {
		return nil, nil, domain.New(domain.CodeProviderNotConfigured,
			"Live quotes and status lookups require a server-side 1Click partner JWT.")
	}

	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, nil, domain.Wrap(domain.CodeInternal, "Could not encode the provider request.", err)
		}
		requestBody = bytes.NewReader(encoded)
	}

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	requestID := uuid.NewString()
	req, err := http.NewRequestWithContext(callCtx, method, c.root+path, requestBody)
	if err != nil {
		return nil, nil, domain.Wrap(domain.CodeInternal, "Could not build the provider request.", err)
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("user-agent", "udi-backend/1.0")
	req.Header.Set("x-request-id", requestID)
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}
	if authenticated && c.jwt != "" {
		req.Header.Set("authorization", "Bearer "+c.jwt)
	}

	startedAt := time.Now()
	response, err := c.httpClient.Do(req)
	if err != nil {
		code := domain.CodeProviderDelayed
		if isTimeout(callCtx, err) {
			code = domain.CodeRPCTimeout
		}
		c.logger.Warn("provider request failed",
			"path", path,
			"request_id", requestID,
			"code", string(code),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
		if code == domain.CodeRPCTimeout {
			return nil, nil, domain.Wrap(code, "The swap provider did not respond in time.", err)
		}
		return nil, nil, domain.Wrap(code, "The swap provider could not be reached.", err)
	}
	defer func() { _ = response.Body.Close() }()

	responseBody, readErr := readBounded(response.Body, c.maxResponseBytes)
	c.logger.Debug("provider request completed",
		"path", path,
		"status", response.StatusCode,
		"request_id", requestID,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)
	if readErr != nil {
		return nil, nil, readErr
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil, mapHTTPError(response.StatusCode, response.Header)
	}
	if !json.Valid(responseBody) {
		return nil, nil, domain.New(domain.CodeProviderInvalidResponse,
			"The swap provider returned invalid JSON.")
	}
	return responseBody, response.Header, nil
}

func readBounded(body io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, domain.Wrap(domain.CodeProviderDelayed, "Could not read the provider response.", err)
	}
	if int64(len(data)) > limit {
		return nil, domain.New(domain.CodeProviderResponseTooLarge,
			"The swap provider returned an oversized response.")
	}
	return data, nil
}

func isTimeout(ctx context.Context, err error) bool {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
