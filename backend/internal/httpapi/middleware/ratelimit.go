package middleware

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

type rateLimitErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"requestId"`
}

// TokenBucket implements a thread-safe token-bucket rate limiter without external dependencies.
type TokenBucket struct {
	mu     sync.Mutex
	rate   float64 // tokens per second
	burst  float64 // maximum bucket size
	tokens float64 // current tokens
	last   time.Time
}

// NewTokenBucket creates a new token bucket limiter.
func NewTokenBucket(rps float64, burst int) *TokenBucket {
	return &TokenBucket{
		rate:   rps,
		burst:  float64(burst),
		tokens: float64(burst),
		last:   time.Now(),
	}
}

// Allow reports whether a request is allowed under the rate limit.
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.last).Seconds()
	tb.last = now

	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.burst {
		tb.tokens = tb.burst
	}

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}
	return false
}

type clientEntry struct {
	limiter  *TokenBucket
	lastSeen time.Time
}

// IPRateLimiter limits incoming requests per client IP.
type IPRateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientEntry
	r       float64
	b       int
}

// NewIPRateLimiter creates an IP-based rate limiter with token bucket parameters.
func NewIPRateLimiter(rps float64, burst int) *IPRateLimiter {
	lim := &IPRateLimiter{
		clients: make(map[string]*clientEntry),
		r:       rps,
		b:       burst,
	}

	go lim.cleanup(10 * time.Minute)
	return lim
}

func (i *IPRateLimiter) getLimiter(ip string) *TokenBucket {
	i.mu.Lock()
	defer i.mu.Unlock()

	entry, exists := i.clients[ip]
	if !exists {
		entry = &clientEntry{
			limiter:  NewTokenBucket(i.r, i.b),
			lastSeen: time.Now(),
		}
		i.clients[ip] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter
}

func (i *IPRateLimiter) cleanup(maxAge time.Duration) {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		i.mu.Lock()
		cutoff := time.Now().Add(-maxAge)
		for ip, entry := range i.clients {
			if entry.lastSeen.Before(cutoff) {
				delete(i.clients, ip)
			}
		}
		i.mu.Unlock()
	}
}

// RateLimit returns HTTP middleware enforcing IP-based rate limiting.
func RateLimit(limiter *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil {
				next.ServeHTTP(w, r)
				return
			}

			ip := clientIP(r)
			l := limiter.getLimiter(ip)
			if !l.Allow() {
				reqID := GetRequestID(r)
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Request-Id", reqID)
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)

				resp := rateLimitErrorResponse{RequestID: reqID}
				resp.Error.Code = "RATE_LIMITED"
				resp.Error.Message = "Too many requests. Please wait and retry."
				_ = json.NewEncoder(w).Encode(resp)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
