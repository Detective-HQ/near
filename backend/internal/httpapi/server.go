package httpapi

import (
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/httpapi/middleware"
	"universal-defi-intelligence/internal/metrics"
)

// RouterOptions configures the edge middleware.
type RouterOptions struct {
	CORSAllowedOrigins []string
	TrustedProxies     []netip.Prefix
	MaxBodyBytes       int64
	RateLimitRPS       float64
	RateLimitBurst     int
	MetricsToken       string
}

// RouterOptionsFromConfig derives router options from validated config.
func RouterOptionsFromConfig(cfg *config.Config) RouterOptions {
	if cfg == nil {
		return RouterOptions{}
	}
	return RouterOptions{
		CORSAllowedOrigins: cfg.Security.CORSAllowedOrigins,
		TrustedProxies:     cfg.Security.TrustedProxies,
		MaxBodyBytes:       cfg.Security.MaxBodyBytes,
		RateLimitRPS:       cfg.Security.RateLimitRPS,
		RateLimitBurst:     cfg.Security.RateLimitBurst,
		MetricsToken:       cfg.Security.MetricsToken,
	}
}

// NewRouter registers every route and applies the middleware chain:
//
//	RequestID -> RealIP -> Logger -> Recoverer -> Gzip -> SecurityHeaders -> CORS ->
//	RateLimit -> MaxBody -> Authenticate -> [RequireAuth] -> handler
func NewRouter(h *Handler, logger *slog.Logger, opts RouterOptions) http.Handler {
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = 64 << 10
	}
	if opts.RateLimitRPS <= 0 {
		opts.RateLimitRPS = 20
	}
	if opts.RateLimitBurst <= 0 {
		opts.RateLimitBurst = 40
	}

	protected := func(handler http.Handler) http.Handler {
		return middleware.RequireAuth(h.auth)(handler)
	}

	mux := http.NewServeMux()
	for _, route := range h.Routes() {
		var handler http.Handler = route.Handler
		if route.Protected {
			handler = protected(route.Handler)
		}
		pattern := route.Method + " " + route.Path
		mux.Handle(pattern, metrics.Instrument(pattern, handler))
	}
	mux.Handle("GET /metrics", metrics.Handler(opts.MetricsToken))

	// Unknown API paths get the JSON envelope; everything else is a plain 404.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		// A known path with the wrong method is a 405, not a 404.
		var allowed []string
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			if method == r.Method {
				continue
			}
			probe := r.Clone(r.Context())
			probe.Method = method
			if _, pattern := mux.Handler(probe); pattern != "" && pattern != "/api/" {
				allowed = append(allowed, method)
			}
		}
		if len(allowed) > 0 {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			middleware.WriteError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed.")
			return
		}
		middleware.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "API route was not found.")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	var handler http.Handler = mux
	handler = middleware.Authenticate(h.auth)(handler)
	handler = middleware.MaxBody(opts.MaxBodyBytes)(handler)
	handler = middleware.RateLimit(middleware.NewIPRateLimiter(opts.RateLimitRPS, opts.RateLimitBurst))(handler)
	handler = middleware.CORS(middleware.CORSConfig{AllowedOrigins: opts.CORSAllowedOrigins})(handler)
	handler = middleware.SecurityHeaders(handler)
	handler = middleware.Gzip(handler)
	handler = middleware.Recoverer(logger)(handler)
	handler = middleware.Logger(logger)(handler)
	handler = middleware.RealIP(opts.TrustedProxies)(handler)
	handler = middleware.RequestID(handler)
	return handler
}

// Route is one registered endpoint. docs/openapi.yaml must document exactly
// this table (enforced by TestOpenAPIMatchesRoutes).
type Route struct {
	Method    string
	Path      string
	Handler   http.HandlerFunc
	Protected bool // requires a session when AUTH_REQUIRED=true
}

// Routes returns the API route table.
func (h *Handler) Routes() []Route {
	return []Route{
		{"GET", "/healthz", h.Healthz, false},
		{"GET", "/readyz", h.Readyz, false},
		{"GET", "/api/v1/health", h.Health, false},
		{"GET", "/api/v1/chains", h.Chains, false},
		{"GET", "/api/v1/assets", h.Assets, false},
		{"GET", "/api/v1/markets/overview", h.MarketsOverview, false},
		{"GET", "/api/v1/intelligence/near", h.IntelligenceNear, false},
		{"GET", "/api/v1/research/scenarios", h.ResearchScenarios, false},
		{"GET", "/api/v1/risk/status", h.RiskStatus, false},
		{"GET", "/api/v1/portfolio", h.Portfolio, false},
		{"GET", "/api/v1/trading/overview", h.TradingOverview, false},
		{"GET", "/api/v1/trading/signals", h.TradingSignals, false},
		{"GET", "/api/v1/trading/top-movers", h.TradingTopMovers, false},
		{"GET", "/api/v1/trading/history", h.TradingHistory, false},
		{"GET", "/api/v1/trading/backtests", h.TradingBacktests, false},
		{"GET", "/api/v1/trading/assets/{id}", h.TradingAsset, false},
		{"GET", "/api/v1/trading/orderblocks/{id}", h.TradingStructure, false},
		{"GET", "/api/v1/trading/liquidity/{id}", h.TradingLiquidity, false},
		{"GET", "/api/v1/trading/scenarios/{id}", h.TradingScenario, false},
		{"POST", "/api/v1/auth/nonce", h.AuthNonce, false},
		{"POST", "/api/v1/auth/verify", h.AuthVerify, false},
		{"POST", "/api/v1/auth/logout", h.AuthLogout, false},
		{"GET", "/api/v1/activity", h.Activity, true},
		{"POST", "/api/v1/quotes", h.Quotes, true},
		{"POST", "/api/v1/swaps", h.Swaps, true},
		{"GET", "/api/v1/swaps/{id}", h.SwapByID, true},
		{"POST", "/api/v1/swaps/{id}/deposit", h.Deposit, true},
		{"POST", "/api/v1/swaps/{id}/cancel", h.Cancel, true},
		{"POST", "/api/v1/swaps/{id}/simulate", h.Simulate, true},
	}
}

// NewServer configures an http.Server with bounded timeouts and header limits.
func NewServer(cfg *config.Config, handler http.Handler) *http.Server {
	server := &http.Server{
		Addr:              ":8081",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	if cfg != nil {
		if cfg.HTTP.Addr != "" {
			server.Addr = cfg.HTTP.Addr
		}
		if cfg.HTTP.ReadHeaderTimeout > 0 {
			server.ReadHeaderTimeout = cfg.HTTP.ReadHeaderTimeout
		}
		if cfg.HTTP.ReadTimeout > 0 {
			server.ReadTimeout = cfg.HTTP.ReadTimeout
		}
		if cfg.HTTP.WriteTimeout > 0 {
			server.WriteTimeout = cfg.HTTP.WriteTimeout
		}
		if cfg.HTTP.IdleTimeout > 0 {
			server.IdleTimeout = cfg.HTTP.IdleTimeout
		}
		if cfg.HTTP.MaxHeaderBytes > 0 {
			server.MaxHeaderBytes = cfg.HTTP.MaxHeaderBytes
		}
	}
	return server
}
