package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

type contextKey string

// RequestIDKey stores the request id in the context.
const RequestIDKey contextKey = "requestId"

// GetRequestID returns the request id assigned by the RequestID middleware.
func GetRequestID(r *http.Request) string {
	if v, ok := r.Context().Value(RequestIDKey).(string); ok && v != "" {
		return v
	}
	return ""
}

// NewRequestID generates a random 12-byte hex id.
func NewRequestID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// validRequestID accepts short, log-safe ids only; anything else is replaced
// so a caller cannot inject arbitrary text into logs or response headers.
func validRequestID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// RequestID assigns every request an id and echoes it in X-Request-Id.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-Id")
		if !validRequestID(reqID) {
			reqID = NewRequestID()
		}
		w.Header().Set("X-Request-Id", reqID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), RequestIDKey, reqID)))
	})
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"requestId,omitempty"`
}

// WriteError writes the standard JSON error envelope.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	body := errorBody{RequestID: GetRequestID(r)}
	body.Error.Code = code
	body.Error.Message = message
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
