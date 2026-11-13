// Package metrics exposes Prometheus metrics for the API and worker.
//
// Labels are bounded: routes use the registered pattern (never the raw path),
// so swap or asset ids cannot explode label cardinality.
package metrics

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds every collector. A dedicated registry (not the global one)
// keeps tests isolated.
var Registry = prometheus.NewRegistry()

var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "udi_http_requests_total",
		Help: "HTTP requests by route pattern, method and status class.",
	}, []string{"route", "method", "status"})

	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "udi_http_request_duration_seconds",
		Help:    "HTTP request latency by route pattern.",
		Buckets: []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"route", "method"})

	providerCalls = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "udi_provider_calls_total",
		Help: "Swap provider calls by outcome code (ok or the domain error code).",
	}, []string{"outcome"})

	swapTransitions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "udi_swap_transitions_total",
		Help: "Swap lifecycle transitions recorded by the worker, by destination state.",
	}, []string{"state"})

	swapsStuck = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "udi_swaps_stuck_total",
		Help: "Swaps that exceeded the poll age ceiling and need manual review.",
	})

	tradingRefresh = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "udi_trading_refresh_duration_seconds",
		Help:    "Duration of a full trading universe scan by timeframe and result.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 45},
	}, []string{"timeframe", "result"})
)

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		httpRequests, httpDuration, providerCalls, swapTransitions, swapsStuck, tradingRefresh,
	)
}

// statusWriter records the response status.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Instrument wraps a handler registered under route.
func Instrument(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		httpDuration.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
		httpRequests.WithLabelValues(route, r.Method, strconv.Itoa(sw.status/100)+"xx").Inc()
	})
}

// ObserveProvider records one provider call outcome.
func ObserveProvider(outcome string) {
	if outcome == "" {
		outcome = "ok"
	}
	providerCalls.WithLabelValues(outcome).Inc()
}

// ObserveTransition records one persisted swap state change.
func ObserveTransition(state string) { swapTransitions.WithLabelValues(state).Inc() }

// ObserveStuck records a swap that stopped being polled.
func ObserveStuck() { swapsStuck.Inc() }

// ObserveTradingRefresh records a trading scan.
func ObserveTradingRefresh(timeframe string, ok bool, d time.Duration) {
	result := "ok"
	if !ok {
		result = "error"
	}
	tradingRefresh.WithLabelValues(timeframe, result).Observe(d.Seconds())
}

// Handler serves the registry. When token is non-empty the scraper must send
// "Authorization: Bearer <token>"; metrics then stay private on a public port.
func Handler(token string) http.Handler {
	inner := promhttp.HandlerFor(Registry, promhttp.HandlerOpts{})
	if token == "" {
		return inner
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	})
}
