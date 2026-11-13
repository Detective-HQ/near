package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recoverer converts a handler panic into a 500 error envelope and logs the stack.
func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					if recovered == http.ErrAbortHandler {
						panic(recovered)
					}
					logger.Error("panic recovered in http handler",
						"error", fmt.Sprint(recovered),
						"requestId", GetRequestID(r),
						"path", r.URL.Path,
						"stack", string(debug.Stack()),
					)
					WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "An unexpected internal error occurred.")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
