package middleware

import (
	"log/slog"
	"net/http"
	"regexp"
	"time"
)

var (
	jwtPattern     = regexp.MustCompile(`eyJ[a-zA-Z0-9_-]{10,}\.eyJ[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]+`)
	hexKeyPattern  = regexp.MustCompile(`(?i)(?:0x)?[0-9a-f]{64}`)
	ethAddrPattern = regexp.MustCompile(`(?i)0x[a-f0-9]{40}`)
)

// statusRecorder captures the response status code for logging.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.statusCode = code
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *statusRecorder) Write(p []byte) (int, error) {
	n, err := rec.ResponseWriter.Write(p)
	rec.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer (flush,
// deadlines) through the recorder.
func (rec *statusRecorder) Unwrap() http.ResponseWriter { return rec.ResponseWriter }

// RedactString masks JWTs, 32-byte hex secrets/hashes and EVM addresses.
func RedactString(s string) string {
	s = jwtPattern.ReplaceAllString(s, "[REDACTED_JWT]")
	s = hexKeyPattern.ReplaceAllString(s, "[REDACTED_KEY]")
	return ethAddrPattern.ReplaceAllStringFunc(s, func(addr string) string {
		return addr[:6] + "..." + addr[len(addr)-4:]
	})
}

// Logger logs one structured line per request with sensitive values redacted.
// Request bodies and headers are never logged.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rec, r)

			level := slog.LevelInfo
			if rec.statusCode >= 500 {
				level = slog.LevelError
			} else if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				level = slog.LevelDebug
			}
			logger.Log(r.Context(), level, "http request",
				"method", r.Method,
				"path", RedactString(r.URL.Path),
				"query", RedactString(r.URL.RawQuery),
				"status", rec.statusCode,
				"bytes", rec.bytes,
				"durationMs", float64(time.Since(start).Microseconds())/1000,
				"requestId", GetRequestID(r),
				"clientIp", ClientIP(r),
			)
		})
	}
}
