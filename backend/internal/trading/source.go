package trading

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// PricePoint is one sampled aggregate price.
type PricePoint struct {
	At    time.Time
	Price float64
}

// PriceSource returns sampled price history for one coin.
type PriceSource interface {
	Name() string
	History(ctx context.Context, coinKey string, sample time.Duration, points int) ([]PricePoint, error)
}

// DefiLlamaPrices reads https://coins.llama.fi/chart/{coin}.
type DefiLlamaPrices struct {
	BaseURL string
	Client  *http.Client
	Now     func() time.Time
}

// Name implements PriceSource.
func (d *DefiLlamaPrices) Name() string { return "DeFiLlama coins API" }

// MaxPointsPerRequest is the upstream cap per request.
const MaxPointsPerRequest = 500

// History fetches up to points samples spaced by sample, ending now.
func (d *DefiLlamaPrices) History(ctx context.Context, coinKey string, sample time.Duration, points int) ([]PricePoint, error) {
	if points <= 0 || points > MaxPointsPerRequest {
		return nil, fmt.Errorf("points must be between 1 and %d", MaxPointsPerRequest)
	}
	period, err := periodParam(sample)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(d.BaseURL, "/")
	if base == "" {
		base = "https://coins.llama.fi"
	}
	now := time.Now
	if d.Now != nil {
		now = d.Now
	}
	start := now().Add(-sample * time.Duration(points)).Unix()
	endpoint := fmt.Sprintf("%s/chart/%s?start=%d&span=%d&period=%s",
		base, url.PathEscape(coinKey), start, points, period)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	client := d.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("price source returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Coins map[string]struct {
			Prices []struct {
				Timestamp int64   `json:"timestamp"`
				Price     float64 `json:"price"`
			} `json:"prices"`
		} `json:"coins"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode price history: %w", err)
	}
	coin, ok := payload.Coins[coinKey]
	if !ok {
		return nil, errors.New("price source has no history for this asset")
	}
	out := make([]PricePoint, 0, len(coin.Prices))
	for _, p := range coin.Prices {
		if p.Price > 0 && p.Timestamp > 0 {
			out = append(out, PricePoint{At: time.Unix(p.Timestamp, 0).UTC(), Price: p.Price})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

func periodParam(sample time.Duration) (string, error) {
	switch sample {
	case time.Hour:
		return "1h", nil
	case 4 * time.Hour:
		return "4h", nil
	case 24 * time.Hour:
		return "1d", nil
	default:
		return "", fmt.Errorf("unsupported sample interval %s", sample)
	}
}
