package httpapi

import (
	"context"
	"io"
	"log/slog"
	"math"
	"net/http"
	"testing"
	"time"

	"universal-defi-intelligence/internal/auth"
	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/swap/memstore"
	"universal-defi-intelligence/internal/swap/providers/alternative"
	"universal-defi-intelligence/internal/swap/service"
	"universal-defi-intelligence/internal/trading"
)

type wavePrices struct{ now time.Time }

func (w wavePrices) Name() string { return "test prices" }

func (w wavePrices) History(_ context.Context, coin string, sample time.Duration, points int) ([]trading.PricePoint, error) {
	drift := 0.002
	if len(coin)%2 == 0 {
		drift = -0.002
	}
	out := make([]trading.PricePoint, points)
	start := w.now.Add(-sample * time.Duration(points))
	for i := range out {
		out[i] = trading.PricePoint{
			At:    start.Add(sample * time.Duration(i+1)),
			Price: 50 * math.Exp(drift*float64(i)) * (1 + 0.02*math.Sin(float64(i)/5)),
		}
	}
	return out, nil
}

func newTradingAPI(t *testing.T, withEngine bool) *testAPI {
	t.Helper()
	swaps, _ := service.New(service.Config{Store: memstore.New(), Provider: alternative.New(), QuoteMode: config.QuoteModeDemo})
	opts := HandlerOptions{
		Swaps:  swaps,
		Auth:   auth.NewManager(auth.Config{Registry: swaps.Registry()}),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if withEngine {
		now := time.Now().UTC()
		opts.Trading = trading.NewEngine(trading.Config{Source: wavePrices{now: now}})
	}
	handler, err := NewHandler(opts)
	if err != nil {
		t.Fatal(err)
	}
	return &testAPI{t: t, handler: NewRouter(handler, slog.New(slog.NewTextHandler(io.Discard, nil)),
		RouterOptions{RateLimitRPS: 1e6, RateLimitBurst: 1 << 20})}
}

func TestTradingRoutesServeContract(t *testing.T) {
	api := newTradingAPI(t, true)

	overview := decode[trading.OverviewResponse](t, api.do("GET", "/api/v1/trading/overview", nil, nil))
	if overview.ModelVersion == nil || overview.Universe == nil || (*overview.Universe)["total"] != len(trading.Universe) {
		t.Fatalf("overview = %+v", overview)
	}

	signals := decode[trading.SignalsResponse](t, api.do("GET", "/api/v1/trading/signals?timeframe=4h", nil, nil))
	if len(signals.Bullish)+len(signals.Bearish) == 0 || len(signals.Filters["timeframe"]) != 2 {
		t.Fatalf("signals = %+v", signals)
	}
	var id string
	if len(signals.Bullish) > 0 {
		id = signals.Bullish[0].AssetID
	} else {
		id = signals.Bearish[0].AssetID
	}

	for _, path := range []string{
		"/api/v1/trading/assets/" + id,
		"/api/v1/trading/orderblocks/" + id + "?timeframe=1d",
		"/api/v1/trading/liquidity/" + id,
		"/api/v1/trading/scenarios/" + id,
		"/api/v1/trading/history?direction=bullish",
		"/api/v1/trading/backtests",
		"/api/v1/trading/top-movers",
	} {
		if rec := api.do("GET", path, nil, nil); rec.Code != http.StatusOK {
			t.Fatalf("%s = %d %s", path, rec.Code, rec.Body.String())
		}
	}

	expectError(t, api.do("GET", "/api/v1/trading/assets/nowhere:none", nil, nil), http.StatusNotFound, "NOT_FOUND")
	expectError(t, api.do("GET", "/api/v1/trading/signals?timeframe=1h", nil, nil), http.StatusBadRequest, "VALIDATION_FAILED")
	expectError(t, api.do("GET", "/api/v1/trading/history?direction=sideways", nil, nil), http.StatusBadRequest, "VALIDATION_FAILED")
}

func TestTradingUnavailableInFixtureMode(t *testing.T) {
	api := newTradingAPI(t, false)
	expectError(t, api.do("GET", "/api/v1/trading/overview", nil, nil), http.StatusServiceUnavailable, "MARKET_DATA_UNAVAILABLE")
}

func mustDemoService(t *testing.T) *service.Service {
	t.Helper()
	swaps, err := service.New(service.Config{Store: memstore.New(), Provider: alternative.New(), QuoteMode: config.QuoteModeDemo})
	if err != nil {
		t.Fatal(err)
	}
	return swaps
}
