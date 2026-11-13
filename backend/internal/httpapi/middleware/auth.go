package middleware

import (
	"context"
	"net/http"
	"strings"

	"universal-defi-intelligence/internal/auth"
)

// SessionContextKey stores the authenticated session.
const SessionContextKey contextKey = "authSession"

// SessionCookieName is the HttpOnly cookie carrying the bearer token.
const SessionCookieName = "udi_session"

// GetSession returns the authenticated session, if any.
func GetSession(r *http.Request) *auth.Session {
	if sess, ok := r.Context().Value(SessionContextKey).(*auth.Session); ok {
		return sess
	}
	return nil
}

// Principal returns the owner identity of the caller ("" when anonymous).
func Principal(r *http.Request) string {
	return GetSession(r).Principal()
}

// Authenticate attaches the session for a valid bearer token or session
// cookie. Invalid or expired tokens are treated as anonymous.
func Authenticate(mgr *auth.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if mgr != nil {
				if token := ExtractToken(r); token != "" {
					if sess, err := mgr.GetSession(r.Context(), token); err == nil && sess != nil {
						r = r.WithContext(context.WithValue(r.Context(), SessionContextKey, sess))
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuth rejects anonymous callers when authentication is enforced.
func RequireAuth(mgr *auth.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if mgr != nil && mgr.AuthRequired() && GetSession(r) == nil {
				WriteError(w, r, http.StatusUnauthorized, "UNAUTHORIZED",
					"Authentication required. Sign in with your wallet via /api/v1/auth/verify.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ExtractToken reads a bearer token from Authorization or the session cookie.
func ExtractToken(r *http.Request) string {
	if header := r.Header.Get("Authorization"); header != "" {
		scheme, token, ok := strings.Cut(header, " ")
		if ok && strings.EqualFold(scheme, "Bearer") {
			return strings.TrimSpace(token)
		}
	}
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}
