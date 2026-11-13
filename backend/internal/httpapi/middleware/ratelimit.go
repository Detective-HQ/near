package middleware

import (
	"net/http"
	"sync"
	"time"
)

// maxTrackedClients bounds the limiter's memory. When it is full, idle
// entries are swept; if every entry is still active, new clients share one
// overflow bucket instead of growing the map without bound.
const maxTrackedClients = 50_000

// TokenBucket is a thread-safe token-bucket rate limiter.
type TokenBucket struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

// NewTokenBucket creates a full bucket.
func NewTokenBucket(rps float64, burst int) *TokenBucket {
	return &TokenBucket{rate: rps, burst: float64(burst), tokens: float64(burst), last: time.Now()}
}

// Allow reports whether one request may proceed now.
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	now := time.Now()
	tb.tokens += now.Sub(tb.last).Seconds() * tb.rate
	tb.last = now
	if tb.tokens > tb.burst {
		tb.tokens = tb.burst
	}
	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

type clientEntry struct {
	limiter  *TokenBucket
	lastSeen time.Time
}

// IPRateLimiter limits requests per resolved client IP.
type IPRateLimiter struct {
	mu        sync.Mutex
	clients   map[string]*clientEntry
	overflow  *TokenBucket
	rate      float64
	burst     int
	idleAfter time.Duration
	lastSweep time.Time
}

// NewIPRateLimiter creates a per-IP limiter without background goroutines.
func NewIPRateLimiter(rps float64, burst int) *IPRateLimiter {
	return &IPRateLimiter{
		clients:   make(map[string]*clientEntry),
		overflow:  NewTokenBucket(rps, burst),
		rate:      rps,
		burst:     burst,
		idleAfter: 10 * time.Minute,
		lastSweep: time.Now(),
	}
}

// Allow reports whether a request from ip may proceed.
func (i *IPRateLimiter) Allow(ip string) bool {
	return i.limiterFor(ip).Allow()
}

func (i *IPRateLimiter) limiterFor(ip string) *TokenBucket {
	i.mu.Lock()
	defer i.mu.Unlock()
	now := time.Now()
	if entry, ok := i.clients[ip]; ok {
		entry.lastSeen = now
		return entry.limiter
	}
	if now.Sub(i.lastSweep) > time.Minute || len(i.clients) >= maxTrackedClients {
		cutoff := now.Add(-i.idleAfter)
		for key, entry := range i.clients {
			if entry.lastSeen.Before(cutoff) {
				delete(i.clients, key)
			}
		}
		i.lastSweep = now
	}
	if len(i.clients) >= maxTrackedClients {
		return i.overflow
	}
	entry := &clientEntry{limiter: NewTokenBucket(i.rate, i.burst), lastSeen: now}
	i.clients[ip] = entry
	return entry.limiter
}

// RateLimit enforces per-client limits. Health probes are exempt so
// orchestrators can always reach them.
func RateLimit(limiter *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				next.ServeHTTP(w, r)
				return
			}
			if !limiter.Allow(ClientIP(r)) {
				w.Header().Set("Retry-After", "1")
				WriteError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests. Please wait and retry.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
