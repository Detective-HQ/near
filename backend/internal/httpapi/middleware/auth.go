package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"universal-defi-intelligence/internal/auth"
)

const SessionContextKey contextKey = "authSession"

// GetSession extracts the authenticated Session from request context, if present.
func GetSession(r *http.Request) *auth.Session {
	if val := r.Context().Value(SessionContextKey); val != nil {
		if sess, ok := val.(*auth.Session); ok {
			return sess
		}
	}
	return nil
}

// Authenticate extracts session tokens from Authorization header or cookie and attaches session to context.
func Authenticate(mgr *auth.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if mgr == nil {
				next.ServeHTTP(w, r)
				return
			}

			token := extractToken(r)
			if token != "" {
				if sess, err := mgr.GetSession(token); err == nil && sess != nil {
					ctx := context.WithValue(r.Context(), SessionContextKey, sess)
					r = r.WithContext(ctx)
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuth enforces that a valid session exists if auth is strictly enabled.
func RequireAuth(mgr *auth.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if mgr != nil && mgr.AuthRequired() {
				sess := GetSession(r)
				if sess == nil {
					reqID := GetRequestID(r)
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.Header().Set("Cache-Control", "no-store")
					w.Header().Set("X-Request-Id", reqID)
					w.WriteHeader(http.StatusUnauthorized)

					var resp struct {
						Error struct {
							Code    string `json:"code"`
							Message string `json:"message"`
						} `json:"error"`
						RequestID string `json:"requestId"`
					}
					resp.RequestID = reqID
					resp.Error.Code = "UNAUTHORIZED"
					resp.Error.Message = "Authentication required. Please authenticate via /api/v1/auth/verify."
					_ = json.NewEncoder(w).Encode(resp)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func extractToken(r *http.Request) string {
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}
	if cookie, err := r.Cookie("session"); err == nil && cookie != nil {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}
