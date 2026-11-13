package middleware

import (
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	// Redaction patterns
	jwtPattern     = regexp.MustCompile(`eyJ[a-zA-Z0-9_-]{10,}\.eyJ[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]+`)
	hexKeyPattern  = regexp.MustCompile(`(?i)(?:0x)?[0-9a-f]{64}`)
	ethAddrPattern = regexp.MustCompile(`(?i)0x[a-f0-9]{40}`)
)

// statusRecorder captures the response status code for logging.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.statusCode = code
	rec.ResponseWriter.WriteHeader(code)
}

// RedactString sanitizes sensitive values such as JWTs, private keys, and addresses.
func RedactString(s string) string {
	s = jwtPattern.ReplaceAllString(s, "[REDACTED_JWT]")
	s = hexKeyPattern.ReplaceAllString(s, "[REDACTED_KEY]")
	s = ethAddrPattern.ReplaceAllStringFunc(s, func(addr string) string {
		if len(addr) >= 10 {
			return addr[:6] + "..." + addr[len(addr)-4:]
		}
		return "[REDACTED_ADDR]"
	})
	return s
}

// Logger returns structured logging middleware that redacts sensitive values.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID := GetRequestID(r)

			rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rec, r)
			duration := time.Since(start)

			sanitizedPath := RedactString(r.URL.Path)
			rawQuery := r.URL.RawQuery
			if rawQuery != "" {
				rawQuery = RedactString(rawQuery)
			}

			logger.Info("http request",
				"method", r.Method,
				"path", sanitizedPath,
				"query", rawQuery,
				"status", rec.statusCode,
				"duration", duration.String(),
				"requestId", reqID,
				"clientIp", clientIP(r),
			)
		})
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	return r.RemoteAddr
}
