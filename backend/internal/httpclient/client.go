// Package httpclient builds the outbound HTTP clients used for providers,
// RPC endpoints and analytics sources.
package httpclient

import (
	"net"
	"net/http"
	"time"
)

// New returns a client with a pooled keep-alive transport. Repeated upstream
// calls reuse warm TLS connections instead of paying a new handshake (often
// 100-300 ms) per request. Redirects are never followed, so an upstream cannot
// bounce a request carrying credentials to another host.
func New(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: time.Second,
		ResponseHeaderTimeout: timeout,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
