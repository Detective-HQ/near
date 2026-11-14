package middleware

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

const clientIPKey contextKey = "clientIp"

// RealIP resolves the client address once per request and stores it in the
// context. X-Forwarded-For is honored only when the direct peer is a trusted
// proxy; the client is then the right-most address that is not itself a
// trusted proxy. An untrusted caller can therefore never spoof its address to
// dodge rate limiting or forge log attribution.
func RealIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := resolveClientIP(r, trusted)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey, ip)))
		})
	}
}

// ClientIP returns the resolved client address.
func ClientIP(r *http.Request) string {
	if value, ok := r.Context().Value(clientIPKey).(string); ok && value != "" {
		return value
	}
	return peerAddr(r).String()
}

func peerAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func isTrusted(addr netip.Addr, trusted []netip.Prefix) bool {
	if !addr.IsValid() {
		return false
	}
	for _, prefix := range trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func resolveClientIP(r *http.Request, trusted []netip.Prefix) string {
	peer := peerAddr(r)
	if !isTrusted(peer, trusted) {
		if peer.IsValid() {
			return peer.String()
		}
		return "unknown"
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		addr = addr.Unmap()
		if !isTrusted(addr, trusted) {
			return addr.String()
		}
	}
	return peer.String()
}
