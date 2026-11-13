// Package alternative is a fallback provider stub.
//
// It implements the full adapter contract without calling any external service.
// It exists so the fallback chain has a concrete second entry and so callers can
// distinguish "this provider does not support the route" (NO_ROUTE) from "this
// provider is not configured" (PROVIDER_NOT_CONFIGURED). No asset is advertised
// until a real fallback provider is integrated.
package alternative

import (
	"context"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

const providerName = "alternative"

// Provider is the unconfigured fallback adapter.
type Provider struct{}

// New returns the fallback adapter.
func New() *Provider { return &Provider{} }

// Name implements providers.Adapter.
func (*Provider) Name() string { return providerName }

// Capabilities implements providers.Adapter.
func (*Provider) Capabilities() providers.Capabilities { return providers.Capabilities{} }

// Health implements providers.Adapter.
func (*Provider) Health() providers.Health { return providers.Health{} }

// GetTokens implements providers.Adapter.
func (*Provider) GetTokens(context.Context) ([]domain.Asset, error) {
	return nil, notConfigured()
}

// GetDryQuote implements providers.Adapter.
func (*Provider) GetDryQuote(context.Context, providers.DryQuoteRequest) (providers.QuoteResult, error) {
	return providers.QuoteResult{}, noRoute()
}

// GetExecutableQuote implements providers.Adapter.
func (*Provider) GetExecutableQuote(context.Context, providers.ExecutableQuoteRequest) (providers.QuoteResult, error) {
	return providers.QuoteResult{}, noRoute()
}

// GetStatus implements providers.Adapter.
func (*Provider) GetStatus(context.Context, providers.StatusRequest) (providers.StatusResult, error) {
	return providers.StatusResult{}, notConfigured()
}

// SubmitDepositTx implements providers.Adapter.
func (*Provider) SubmitDepositTx(context.Context, providers.DepositRequest) (providers.StatusResult, error) {
	return providers.StatusResult{}, notConfigured()
}

func noRoute() error {
	return domain.New(domain.CodeNoRoute, "No fallback provider supports this route.")
}

func notConfigured() error {
	return domain.New(domain.CodeProviderNotConfigured, "No fallback provider is configured.")
}
