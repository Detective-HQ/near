package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

type panicErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"requestId"`
}

// Recoverer recovers from unhandled panics and outputs a standardized RFC-compliant error JSON.
func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					reqID := GetRequestID(r)
					stack := string(debug.Stack())
					logger.Error("panic recovered in http handler",
						"error", fmt.Sprint(err),
						"requestId", reqID,
						"path", r.URL.Path,
						"stack", stack,
					)

					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.Header().Set("Cache-Control", "no-store")
					w.Header().Set("X-Request-Id", reqID)
					w.WriteHeader(http.StatusInternalServerError)

					resp := panicErrorResponse{
						RequestID: reqID,
					}
					resp.Error.Code = "INTERNAL"
					resp.Error.Message = "An unexpected internal error occurred."
					_ = json.NewEncoder(w).Encode(resp)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

