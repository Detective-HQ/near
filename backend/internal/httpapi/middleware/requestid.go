package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type contextKey string

const RequestIDKey contextKey = "requestId"

// GetRequestID returns the request ID from the request context or header.
func GetRequestID(r *http.Request) string {
	if v := r.Context().Value(RequestIDKey); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	if incoming := r.Header.Get("X-Request-Id"); incoming != "" {
		return incoming
	}
	return NewRequestID()
}

// NewRequestID generates a new cryptographically random 12-byte hex ID.
func NewRequestID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// RequestID returns middleware that injects a request ID into each request.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = NewRequestID()
		}
		ctx := context.WithValue(r.Context(), RequestIDKey, reqID)
		w.Header().Set("X-Request-Id", reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

