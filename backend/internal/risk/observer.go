package risk

import (
	"net/http"
	"time"

	"universal-defi-intelligence/internal/metrics"
	"universal-defi-intelligence/internal/swap/domain"
)

// ObserveProviderCall feeds a provider call outcome into the circuit breaker.
// Client-side validation failures (4xx other than 429) do not count against
// the provider; timeouts, rate limits and upstream failures do.
func (e *Engine) ObserveProviderCall(err error) {
	if err == nil {
		metrics.ObserveProvider("")
	} else {
		metrics.ObserveProvider(string(domain.CodeOf(err)))
	}
	if err == nil {
		e.RecordProviderCall(http.StatusOK)
		return
	}
	if domain.IsCode(err, domain.CodeProviderRateLimited) {
		e.RecordProviderCall(http.StatusTooManyRequests)
		return
	}
	e.RecordProviderCall(domain.HTTPStatusOf(err))
}

// ObserveCatalogueRefresh records a successful catalogue load.
func (e *Engine) ObserveCatalogueRefresh(at time.Time) {
	e.RecordCatalogueRefresh(at)
}
