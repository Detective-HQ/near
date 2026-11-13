package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"universal-defi-intelligence/internal/auth"
)

func TestMetricsEndpointRecordsRoutesAndRequiresToken(t *testing.T) {
	swaps := mustDemoService(t)
	h, err := NewHandler(HandlerOptions{Swaps: swaps, Auth: auth.NewManager(auth.Config{Registry: swaps.Registry()}),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	api := &testAPI{t: t, handler: NewRouter(h, slog.New(slog.NewTextHandler(io.Discard, nil)),
		RouterOptions{RateLimitRPS: 1e6, RateLimitBurst: 1 << 20, MetricsToken: "scrape-secret"})}

	api.do("GET", "/api/v1/swaps/swp_missing", nil, nil)
	api.do("GET", "/api/v1/health", nil, nil)

	if rec := api.do("GET", "/metrics", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("metrics without token = %d", rec.Code)
	}
	rec := api.do("GET", "/metrics", nil, map[string]string{"Authorization": "Bearer scrape-secret"})
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`udi_http_requests_total{method="GET",route="GET /api/v1/swaps/{id}",status="4xx"}`,
		`udi_http_request_duration_seconds_bucket{method="GET",route="GET /api/v1/health"`,
		"go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q", want)
		}
	}
	if strings.Contains(body, "swp_missing") {
		t.Fatal("raw ids leaked into metric labels")
	}
}
