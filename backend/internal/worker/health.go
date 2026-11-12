package worker

import (
	"sort"
	"sync"
	"time"

	"universal-defi-intelligence/internal/swap/domain"
)

// ProviderHealth is a point-in-time view of one provider's health.
type ProviderHealth struct {
	Provider      string
	Total         int64
	Failures      int64
	RateLimited   int64
	ServerErrors  int64
	LastSuccess   time.Time
	LastFailure   time.Time
	LastErrorCode domain.Code
	CatalogueAge  time.Time
	CatalogueSize int
}

// Health tracks provider outcomes for readiness checks and the risk endpoint.
// It is safe for concurrent use by the poller and the catalogue refresher.
type Health struct {
	mu        sync.Mutex
	providers map[string]*providerCounters
}

type providerCounters struct {
	total         int64
	failures      int64
	rateLimited   int64
	serverErrors  int64
	lastSuccess   time.Time
	lastFailure   time.Time
	lastErrorCode domain.Code
	catalogueAge  time.Time
	catalogueSize int
}

// NewHealth creates an empty health tracker.
func NewHealth() *Health {
	return &Health{providers: make(map[string]*providerCounters)}
}

func (h *Health) counter(provider string) *providerCounters {
	if h.providers == nil {
		h.providers = make(map[string]*providerCounters)
	}
	counter, ok := h.providers[provider]
	if !ok {
		counter = &providerCounters{}
		h.providers[provider] = counter
	}
	return counter
}

// RecordSuccess records a successful provider call.
func (h *Health) RecordSuccess(provider string, at time.Time) {
	if h == nil {
		return
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	counter := h.counter(provider)
	counter.total++
	counter.lastSuccess = at.UTC()
	counter.lastErrorCode = ""
}

// RecordError records a failed provider call and classifies the failure.
func (h *Health) RecordError(provider string, code domain.Code, at time.Time) {
	if h == nil {
		return
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	counter := h.counter(provider)
	counter.total++
	counter.failures++
	counter.lastFailure = at.UTC()
	counter.lastErrorCode = code
	switch code {
	case domain.CodeProviderRateLimited:
		counter.rateLimited++
	case domain.CodeProviderDelayed, domain.CodeRPCTimeout, domain.CodeUnknownProviderStatus, domain.CodeProviderInvalidResponse:
		counter.serverErrors++
	}
}

// RecordCatalogue records a successful catalogue refresh.
func (h *Health) RecordCatalogue(provider string, size int, at time.Time) {
	if h == nil {
		return
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	counter := h.counter(provider)
	counter.catalogueAge = at.UTC()
	counter.catalogueSize = size
}

// Snapshot returns the health of one provider.
func (h *Health) Snapshot(provider string) (ProviderHealth, bool) {
	if h == nil {
		return ProviderHealth{}, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	counter, ok := h.providers[provider]
	if !ok {
		return ProviderHealth{}, false
	}
	return counter.snapshot(provider), true
}

// SnapshotAll returns every tracked provider, sorted by name.
func (h *Health) SnapshotAll() []ProviderHealth {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]ProviderHealth, 0, len(h.providers))
	for name, counter := range h.providers {
		out = append(out, counter.snapshot(name))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out
}

// CatalogueStale reports whether the provider's catalogue is older than
// maxAge. An unknown catalogue age is always stale.
func (h *Health) CatalogueStale(provider string, maxAge time.Duration, now time.Time) bool {
	if h == nil {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	counter, ok := h.providers[provider]
	if !ok || counter.catalogueAge.IsZero() {
		return true
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return now.Sub(counter.catalogueAge) > maxAge
}

func (c *providerCounters) snapshot(provider string) ProviderHealth {
	return ProviderHealth{
		Provider:      provider,
		Total:         c.total,
		Failures:      c.failures,
		RateLimited:   c.rateLimited,
		ServerErrors:  c.serverErrors,
		LastSuccess:   c.lastSuccess,
		LastFailure:   c.lastFailure,
		LastErrorCode: c.lastErrorCode,
		CatalogueAge:  c.catalogueAge,
		CatalogueSize: c.catalogueSize,
	}
}
