package worker

import (
	"context"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

// AdapterProvider adapts a providers.Adapter to the worker's small Provider
// interface. It keeps the worker free of HTTP and credential concerns.
type AdapterProvider struct {
	adapter providers.Adapter
}

// NewAdapterProvider wraps a provider adapter for the worker.
func NewAdapterProvider(adapter providers.Adapter) *AdapterProvider {
	return &AdapterProvider{adapter: adapter}
}

// Name implements Provider.
func (p *AdapterProvider) Name() string {
	if p.adapter == nil {
		return "unconfigured"
	}
	return p.adapter.Name()
}

// Available implements Provider.
//
// The provider is considered available when it can serve either the catalogue
// or status lookups. The 1Click catalogue works without a partner JWT, so a
// missing JWT does not disable catalogue refresh; status calls return
// PROVIDER_NOT_CONFIGURED and the poller pauses swaps instead.
func (p *AdapterProvider) Available() bool {
	if p.adapter == nil {
		return false
	}
	caps := p.adapter.Capabilities()
	return caps.Catalogue || caps.Status
}

// GetStatus implements Provider.
func (p *AdapterProvider) GetStatus(ctx context.Context, req StatusRequest) (StatusResult, error) {
	if p.adapter == nil {
		return StatusResult{}, domain.New(domain.CodeProviderNotConfigured, "No swap provider is configured.")
	}
	result, err := p.adapter.GetStatus(ctx, providers.StatusRequest{
		DepositAddress: req.DepositAddress,
		DepositMemo:    req.DepositMemo,
	})
	if err != nil {
		return StatusResult{}, err
	}
	return StatusResult{
		Status:            result.ProviderStatus,
		SourceTxHash:      result.SourceTxHash,
		DestinationTxHash: result.DestinationTxHash,
		RefundTxHash:      result.RefundTxHash,
		Raw:               result.Raw,
	}, nil
}

// GetTokens implements Provider.
func (p *AdapterProvider) GetTokens(ctx context.Context) ([]domain.Asset, error) {
	if p.adapter == nil {
		return nil, domain.New(domain.CodeProviderNotConfigured, "No swap provider is configured.")
	}
	return p.adapter.GetTokens(ctx)
}
