package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders adds hardened headers to every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// CORSConfig configures cross-origin access.
type CORSConfig struct {
	// AllowedOrigins is an exact-match allowlist (scheme://host[:port]).
	AllowedOrigins []string
}

// CORS answers only allowlisted origins and never uses a wildcard. With an
// empty allowlist no CORS headers are emitted at all (same-origin only).
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		allowed[strings.ToLower(strings.TrimRight(strings.TrimSpace(origin), "/"))] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			_, ok := allowed[strings.ToLower(origin)]
			if origin != "" && ok {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Add("Vary", "Origin")
				h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key, Accept, Authorization, X-Request-Id")
				h.Set("Access-Control-Expose-Headers", "X-Request-Id")
				h.Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				if !ok {
					WriteError(w, r, http.StatusForbidden, "FORBIDDEN", "Origin is not allowed.")
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// MaxBody caps request bodies; reads past the limit fail with
// *http.MaxBytesError, which handlers map to 413 BODY_TOO_LARGE.
func MaxBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				WriteError(w, r, http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", "Request body is too large.")
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}
