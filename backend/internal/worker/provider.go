// Package worker contains the background daemon that polls swap providers,
// refreshes the token catalogue, and tracks provider health.
//
// The worker depends on the provider through the small Provider interface rather
// than a concrete HTTP client. That keeps the polling, retry, and scheduling
// logic testable with a fake and lets the Phase 4 1Click adapter drop in without
// changing the worker.
package worker

import (
	"context"
	"strings"
	"time"

	"universal-defi-intelligence/internal/swap/domain"
)

// Clock abstracts the current time so poll scheduling can be tested with a
// deterministic fake instead of sleeping.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production Clock.
type SystemClock struct{}

// Now returns the current UTC time.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// StatusRequest identifies the swap a provider status lookup is about.
type StatusRequest struct {
	SwapID         string
	OriginChain    string
	DepositAddress string
	DepositMemo    string
}

// StatusResult is the normalized provider status observation.
type StatusResult struct {
	Status            domain.ProviderStatus
	SourceTxHash      string
	DestinationTxHash string
	RefundTxHash      string
	Raw               []byte
}

// Provider is the subset of a swap provider the worker needs. Every method must
// map to an idempotent provider read (HTTP GET), because the retry policy may
// call it again.
type Provider interface {
	// Name is the stable provider identifier used in logs and health records.
	Name() string
	// Available reports whether the provider is configured. An unavailable
	// provider pauses polling instead of failing every swap.
	Available() bool
	// GetStatus returns the current provider status for a swap.
	GetStatus(ctx context.Context, req StatusRequest) (StatusResult, error)
	// GetTokens returns the provider token catalogue.
	GetTokens(ctx context.Context) ([]domain.Asset, error)
}

// UnavailableProvider is a placeholder Provider used until the Phase 4 adapter
// is wired. It reports every call as PROVIDER_NOT_CONFIGURED so the worker can
// run without a partner credential.
type UnavailableProvider struct {
	providerName string
}

// NewUnavailableProvider returns a placeholder provider.
func NewUnavailableProvider(name string) *UnavailableProvider {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "unconfigured"
	}
	return &UnavailableProvider{providerName: name}
}

// Name implements Provider.
func (p *UnavailableProvider) Name() string { return p.providerName }

// Available implements Provider.
func (p *UnavailableProvider) Available() bool { return false }

// GetStatus implements Provider.
func (p *UnavailableProvider) GetStatus(context.Context, StatusRequest) (StatusResult, error) {
	return StatusResult{}, domain.New(domain.CodeProviderNotConfigured, "No swap provider is configured.")
}

// GetTokens implements Provider.
func (p *UnavailableProvider) GetTokens(context.Context) ([]domain.Asset, error) {
	return nil, domain.New(domain.CodeProviderNotConfigured, "No swap provider is configured.")
}
