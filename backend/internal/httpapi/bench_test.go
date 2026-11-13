package httpapi

import (
	"fmt"
	"net/http/httptest"
	"testing"
)

func seedSwaps(b *testing.B, api *testAPI, n int) {
	b.Helper()
	for i := 0; i < n; i++ {
		rec := api.do("POST", "/api/v1/quotes", map[string]any{
			"originAssetId": "demo:near:near", "destinationAssetId": "demo:base:usdc", "amountIn": "10",
		}, nil)
		quote := decode[struct {
			Quote SwapQuoteDTO `json:"quote"`
		}](&testing.T{}, rec).Quote
		api.do("POST", "/api/v1/swaps", map[string]any{"quoteId": quote.ID},
			map[string]string{"Idempotency-Key": fmt.Sprintf("bench-key-%06d", i)})
	}
}

func benchGET(b *testing.B, path string, headers map[string]string) {
	api := newTestAPI(&testing.T{}, false, RouterOptions{RateLimitRPS: 1e9, RateLimitBurst: 1 << 30})
	seedSwaps(b, api, 50)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", path, nil)
		req.RemoteAddr = "203.0.113.10:5555"
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		api.handler.ServeHTTP(rec, req)
		if rec.Code != 200 {
			b.Fatalf("status %d", rec.Code)
		}
	}
}

func BenchmarkActivity50(b *testing.B) { benchGET(b, "/api/v1/activity?limit=50", nil) }
func BenchmarkActivity50Gzip(b *testing.B) {
	benchGET(b, "/api/v1/activity?limit=50", map[string]string{"Accept-Encoding": "gzip"})
}
func BenchmarkAssets(b *testing.B) { benchGET(b, "/api/v1/assets", nil) }
func BenchmarkChains(b *testing.B) { benchGET(b, "/api/v1/chains", nil) }

func BenchmarkQuote(b *testing.B) {
	api := newTestAPI(&testing.T{}, false, RouterOptions{RateLimitRPS: 1e9, RateLimitBurst: 1 << 30})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := api.do("POST", "/api/v1/quotes", map[string]any{
			"originAssetId": "demo:ethereum:eth", "destinationAssetId": "demo:base:usdc", "amountIn": "1.25",
		}, nil)
		if rec.Code != 201 {
			b.Fatalf("status %d", rec.Code)
		}
	}
}
