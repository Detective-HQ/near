package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/httpapi/middleware"
)

// Backwards-compatible aliases for existing packages/callers
func GetRequestID(r *http.Request) string {
	return middleware.GetRequestID(r)
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return middleware.RequestID(next)
}

func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return middleware.SecurityHeaders(next)
}

func CORSMiddleware(next http.Handler) http.Handler {
	return middleware.CORS(middleware.CORSConfig{AllowAll: true})(next)
}

func RecovererMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return middleware.Recoverer(logger)
}

func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return middleware.Logger(logger)
}

// NewRouter sets up all endpoints and applies the hardened middleware chain in order:
// RequestID -> Logger -> Recoverer -> CORS -> RateLimit -> Auth -> Handler.
func NewRouter(h *Handler, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// Liveness and readiness probes
	mux.HandleFunc("/healthz", h.Healthz)
	mux.HandleFunc("/readyz", h.Readyz)

	// API routes
	mux.HandleFunc("/api/v1/health", h.Health)
	mux.HandleFunc("/api/v1/chains", h.Chains)
	mux.HandleFunc("/api/v1/assets", h.Assets)
	mux.HandleFunc("/api/v1/markets/overview", h.MarketsOverview)
	mux.HandleFunc("/api/v1/intelligence/near", h.IntelligenceNear)
	mux.HandleFunc("/api/v1/research/scenarios", h.ResearchScenarios)
	mux.HandleFunc("/api/v1/risk/status", h.RiskStatus)

	// Auth routes
	mux.HandleFunc("/api/v1/auth/nonce", h.AuthNonce)
	mux.HandleFunc("/api/v1/auth/verify", h.AuthVerify)
	mux.HandleFunc("/api/v1/auth/logout", h.AuthLogout)

	// Swap & Activity routes
	mux.HandleFunc("/api/v1/activity", h.Activity)
	mux.HandleFunc("/api/v1/quotes", h.Quotes)
	mux.HandleFunc("/api/v1/swaps", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/swaps" {
			h.Swaps(w, r)
			return
		}
		h.SwapByID(w, r)
	})
	mux.HandleFunc("/api/v1/swaps/", h.SwapByID)

	// Catch-all
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("API route %s was not found.", r.URL.Path))
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Universal DeFi Intelligence Go API Server. Use /api/v1/health for status."))
	})

	rateLimiter := middleware.NewIPRateLimiter(50.0, 100) // 50 req/sec, burst 100

	// Middleware chain: RequestID -> Logger -> Recoverer -> CORS -> SecurityHeaders -> RateLimit -> Auth -> Handler
	var handler http.Handler = mux
	if h.authManager != nil {
		handler = middleware.Authenticate(h.authManager)(handler)
	}
	handler = middleware.RateLimit(rateLimiter)(handler)
	handler = middleware.SecurityHeaders(handler)
	handler = middleware.CORS(middleware.CORSConfig{AllowAll: true})(handler)
	handler = middleware.Recoverer(logger)(handler)
	handler = middleware.Logger(logger)(handler)
	handler = middleware.RequestID(handler)

	return handler
}

// NewServer configures a hardened http.Server with bounded timeouts and header limits.
func NewServer(cfg *config.Config, handler http.Handler) *http.Server {
	addr := ":8081"
	readHeaderTimeout := 5 * time.Second
	readTimeout := 15 * time.Second
	writeTimeout := 15 * time.Second
	idleTimeout := 60 * time.Second
	maxHeaderBytes := 1 << 20 // 1 MB

	if cfg != nil {
		if cfg.HTTP.Addr != "" {
			addr = cfg.HTTP.Addr
		}
		if cfg.HTTP.ReadHeaderTimeout > 0 {
			readHeaderTimeout = cfg.HTTP.ReadHeaderTimeout
		}
		if cfg.HTTP.ReadTimeout > 0 {
			readTimeout = cfg.HTTP.ReadTimeout
		}
		if cfg.HTTP.WriteTimeout > 0 {
			writeTimeout = cfg.HTTP.WriteTimeout
		}
		if cfg.HTTP.IdleTimeout > 0 {
			idleTimeout = cfg.HTTP.IdleTimeout
		}
		if cfg.HTTP.MaxHeaderBytes > 0 {
			maxHeaderBytes = cfg.HTTP.MaxHeaderBytes
		}
	}

	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
}
