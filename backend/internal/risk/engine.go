// Package risk implements the risk monitoring engine tracking slippage limits,
// catalogue freshness, provider error rates, circuit breaker states, and poll health.
package risk

import (
	"sync"
	"time"
)

// CircuitState represents the operational state of external providers.
type CircuitState string

const (
	CircuitClosed   CircuitState = "CLOSED"    // Healthy / normal
	CircuitHalfOpen CircuitState = "HALF_OPEN" // Degraded, testing recovery
	CircuitOpen     CircuitState = "OPEN"      // Tripped / provider suspended
)

// Config configures the risk engine parameters.
type Config struct {
	MaxSlippageBps     int           // Max allowed slippage before quote rejected (e.g. 500 = 5%)
	WarningSlippageBps int           // Slippage threshold for warning flag (e.g. 200 = 2%)
	CatalogueMaxAge    time.Duration // Time before asset catalogue is marked stale (e.g. 2 hours)
	ErrorRateThreshold float64       // 5xx/429 fraction before circuit breaker trips (e.g. 0.25 = 25%)
}

// RiskReport represents the public status report exposed at /api/v1/risk/status.
type RiskReport struct {
	Status                 string       `json:"status"` // HEALTHY, DEGRADED, UNHEALTHY
	CircuitBreaker         CircuitState `json:"circuitBreaker"`
	ProviderErrorRate      float64      `json:"providerErrorRate"`
	LastSuccessfulPoll     *string      `json:"lastSuccessfulPoll"`
	CatalogueStale         bool         `json:"catalogueStale"`
	CatalogueLastRefreshed *string      `json:"catalogueLastRefreshed"`
	MaxSlippageBps         int          `json:"maxSlippageBps"`
	WarningSlippageBps     int          `json:"warningSlippageBps"`
	ActiveAlerts           []string     `json:"activeAlerts"`
}

// Engine coordinates real-time risk checks and observability metrics.
type Engine struct {
	mu                     sync.RWMutex
	maxSlippageBps         int
	warningSlippageBps     int
	catalogueMaxAge        time.Duration
	errorRateThreshold     float64
	circuitState           CircuitState
	catalogueLastRefreshed time.Time
	lastSuccessfulPoll     time.Time
	providerCalls          int
	providerErrors         int // 429 or 5xx count
}

// NewEngine creates a new risk engine.
func NewEngine(cfg Config) *Engine {
	maxSlippage := cfg.MaxSlippageBps
	if maxSlippage <= 0 {
		maxSlippage = 500 // 5.00%
	}
	warnSlippage := cfg.WarningSlippageBps
	if warnSlippage <= 0 {
		warnSlippage = 200 // 2.00%
	}
	catMaxAge := cfg.CatalogueMaxAge
	if catMaxAge <= 0 {
		catMaxAge = 2 * time.Hour
	}
	errThreshold := cfg.ErrorRateThreshold
	if errThreshold <= 0 {
		errThreshold = 0.25 // 25% error rate trips circuit breaker
	}

	return &Engine{
		maxSlippageBps:         maxSlippage,
		warningSlippageBps:     warnSlippage,
		catalogueMaxAge:        catMaxAge,
		errorRateThreshold:     errThreshold,
		circuitState:           CircuitClosed,
		catalogueLastRefreshed: time.Now().UTC(),
		lastSuccessfulPoll:     time.Now().UTC(),
	}
}

// RecordProviderCall records the outcome of an external provider invocation.
func (e *Engine) RecordProviderCall(statusCode int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.providerCalls++
	if statusCode == 429 || statusCode >= 500 {
		e.providerErrors++
	}

	// Update circuit breaker
	if e.providerCalls >= 10 {
		rate := float64(e.providerErrors) / float64(e.providerCalls)
		if rate >= e.errorRateThreshold {
			e.circuitState = CircuitOpen
		} else if rate >= (e.errorRateThreshold / 2) {
			e.circuitState = CircuitHalfOpen
		} else {
			e.circuitState = CircuitClosed
		}

		// Decay window periodically to prevent lockup
		if e.providerCalls > 100 {
			e.providerCalls /= 2
			e.providerErrors /= 2
		}
	}
}

// RecordCatalogueRefresh updates the timestamp of the last asset catalogue sync.
func (e *Engine) RecordCatalogueRefresh(t time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.catalogueLastRefreshed = t
}

// RecordPollSuccess updates the timestamp of the last successful background status poll.
func (e *Engine) RecordPollSuccess(t time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastSuccessfulPoll = t
}

// CheckSlippage validates requested slippage against maximum risk limits.
// Returns (exceedsMax bool, warning bool).
func (e *Engine) CheckSlippage(slippageBps int) (exceedsMax bool, hasWarning bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if slippageBps > e.maxSlippageBps {
		return true, true
	}
	if slippageBps > e.warningSlippageBps {
		return false, true
	}
	return false, false
}

// CheckExpiry calculates whether a deadline is critically close to expiring.
func (e *Engine) CheckExpiry(expiresAt time.Time, criticalDuration time.Duration) (expired bool, critical bool) {
	now := time.Now().UTC()
	if !expiresAt.IsZero() && now.After(expiresAt) {
		return true, true
	}
	if criticalDuration <= 0 {
		criticalDuration = 30 * time.Second
	}
	if !expiresAt.IsZero() && expiresAt.Sub(now) < criticalDuration {
		return false, true
	}
	return false, false
}

// Status compiles the real-time operational risk status.
func (e *Engine) Status() RiskReport {
	e.mu.RLock()
	defer e.mu.RUnlock()

	now := time.Now().UTC()
	var alerts []string

	// Check catalogue freshness
	catStale := false
	if !e.catalogueLastRefreshed.IsZero() && now.Sub(e.catalogueLastRefreshed) > e.catalogueMaxAge {
		catStale = true
		alerts = append(alerts, "Asset catalogue is stale; refresher may be degraded")
	}

	// Compute error rate
	errRate := 0.0
	if e.providerCalls > 0 {
		errRate = float64(e.providerErrors) / float64(e.providerCalls)
	}

	if e.circuitState == CircuitOpen {
		alerts = append(alerts, "Provider circuit breaker is TRIPPED; high 429/5xx error rate detected")
	} else if e.circuitState == CircuitHalfOpen {
		alerts = append(alerts, "Provider error rate elevated; running in DEGRADED mode")
	}

	overallStatus := "HEALTHY"
	if e.circuitState == CircuitOpen || catStale {
		overallStatus = "UNHEALTHY"
	} else if e.circuitState == CircuitHalfOpen || errRate > 0.05 {
		overallStatus = "DEGRADED"
	}

	var lastPollStr *string
	if !e.lastSuccessfulPoll.IsZero() {
		s := e.lastSuccessfulPoll.Format(time.RFC3339)
		lastPollStr = &s
	}

	var catRefreshedStr *string
	if !e.catalogueLastRefreshed.IsZero() {
		s := e.catalogueLastRefreshed.Format(time.RFC3339)
		catRefreshedStr = &s
	}

	return RiskReport{
		Status:                 overallStatus,
		CircuitBreaker:         e.circuitState,
		ProviderErrorRate:      errRate,
		LastSuccessfulPoll:     lastPollStr,
		CatalogueStale:         catStale,
		CatalogueLastRefreshed: catRefreshedStr,
		MaxSlippageBps:         e.maxSlippageBps,
		WarningSlippageBps:     e.warningSlippageBps,
		ActiveAlerts:           alerts,
	}
}
