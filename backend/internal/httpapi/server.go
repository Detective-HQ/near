package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"universal-defi-intelligence/internal/config"
)

type contextKey string

const requestIDKey contextKey = "requestId"

func GetRequestID(r *http.Request) string {
	if v := r.Context().Value(requestIDKey); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	if incoming := r.Header.Get("X-Request-Id"); incoming != "" {
		return incoming
	}
	return newRequestID()
}

func newRequestID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = newRequestID()
		}
		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		w.Header().Set("X-Request-Id", reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, HEAD")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key, Accept, Authorization, X-Request-Id")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func RecovererMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					reqID := GetRequestID(r)
					logger.Error("panic recovered", "error", err, "requestId", reqID, "path", r.URL.Path)
					writeError(w, r, http.StatusInternalServerError, "INTERNAL", "An unexpected internal error occurred.")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.statusCode = code
	rec.ResponseWriter.WriteHeader(code)
}

func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rec, r)
			duration := time.Since(start)

			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.statusCode,
				"duration", duration.String(),
				"requestId", GetRequestID(r),
			)
		})
	}
}

func NewRouter(h *Handler, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/health", h.Health)
	mux.HandleFunc("/api/v1/chains", h.Chains)
	mux.HandleFunc("/api/v1/assets", h.Assets)
	mux.HandleFunc("/api/v1/markets/overview", h.MarketsOverview)
	mux.HandleFunc("/api/v1/intelligence/near", h.IntelligenceNear)
	mux.HandleFunc("/api/v1/research/scenarios", h.ResearchScenarios)
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

	// Catch-all for other paths
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("API route %s was not found.", r.URL.Path))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Universal DeFi Intelligence Go API Server. Use /api/v1/health for status."))
	})

	var handler http.Handler = mux
	handler = CORSMiddleware(handler)
	handler = SecurityHeadersMiddleware(handler)
	handler = LoggingMiddleware(logger)(handler)
	handler = RecovererMiddleware(logger)(handler)
	handler = RequestIDMiddleware(handler)

	return handler
}

func NewServer(cfg *config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		MaxHeaderBytes:    cfg.HTTP.MaxHeaderBytes,
	}
}
