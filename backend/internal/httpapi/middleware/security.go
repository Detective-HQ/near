package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders adds hardened security headers to all responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// CORSConfig configures cross-origin resource sharing.
type CORSConfig struct {
	AllowedOrigins []string
	AllowAll       bool
}

// CORS returns CORS middleware with strict allowlisting and safe defaults.
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	allowedMap := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		allowedMap[strings.ToLower(strings.TrimSpace(o))] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				lowerOrigin := strings.ToLower(origin)
				allowed := false
				if cfg.AllowAll {
					allowed = true
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else if _, ok := allowedMap[lowerOrigin]; ok {
					allowed = true
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}

				if allowed {
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, HEAD")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key, Accept, Authorization, X-Request-Id")
					w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id")
					w.Header().Set("Access-Control-Max-Age", "86400")
				}
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

