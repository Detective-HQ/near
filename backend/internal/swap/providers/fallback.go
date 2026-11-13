package providers

import (
	"context"
	"strings"

	"universal-defi-intelligence/internal/swap/domain"
)

// Chain tries adapters in order. It only falls through when the current
// adapter reports NO_ROUTE or PROVIDER_NOT_CONFIGURED; real upstream failures
// (auth, rate limit, timeout) are returned immediately so a retry cannot be
// silently redirected to a different provider.
//
// Adapters are expected to advertise only routes they truly support. A fallback
// provider that returns NO_ROUTE for every pair is a valid last entry.
type Chain struct {
	adapters []Adapter
}

// NewChain returns a fallback chain. Empty and nil adapters are ignored.
func NewChain(adapters ...Adapter) *Chain {
	filtered := make([]Adapter, 0, len(adapters))
	for _, adapter := range adapters {
		if adapter != nil {
			filtered = append(filtered, adapter)
		}
	}
	return &Chain{adapters: filtered}
}

// Name implements Adapter. It reports the primary adapter so catalogue rows and
// health records stay attributed to the configured provider.
func (c *Chain) Name() string {
	if len(c.adapters) == 0 {
		return "unconfigured"
	}
	return c.adapters[0].Name()
}

// Capabilities implements Adapter.
func (c *Chain) Capabilities() Capabilities {
	var out Capabilities
	for _, adapter := range c.adapters {
		caps := adapter.Capabilities()
		out.Catalogue = out.Catalogue || caps.Catalogue
		out.DryQuote = out.DryQuote || caps.DryQuote
		out.ExecutableQuote = out.ExecutableQuote || caps.ExecutableQuote
		out.Status = out.Status || caps.Status
		out.SubmitDeposit = out.SubmitDeposit || caps.SubmitDeposit
		out.SignatureVerification = out.SignatureVerification || caps.SignatureVerification
	}
	return out
}

// Health implements Adapter.
func (c *Chain) Health() Health {
	var out Health
	for _, adapter := range c.adapters {
		health := adapter.Health()
		out.Configured = out.Configured || health.Configured
		out.SignatureVerification = out.SignatureVerification || health.SignatureVerification
	}
	return out
}

// GetTokens implements Adapter.
func (c *Chain) GetTokens(ctx context.Context) ([]domain.Asset, error) {
	var lastErr error
	for _, adapter := range c.adapters {
		if !adapter.Capabilities().Catalogue {
			continue
		}
		assets, err := adapter.GetTokens(ctx)
		if err == nil {
			return assets, nil
		}
		if !fallbackEligible(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, exhaustion(lastErr)
}

// GetDryQuote implements Adapter.
func (c *Chain) GetDryQuote(ctx context.Context, req DryQuoteRequest) (QuoteResult, error) {
	var lastErr error
	for _, adapter := range c.adapters {
		if !adapter.Capabilities().DryQuote {
			continue
		}
		quote, err := adapter.GetDryQuote(ctx, req)
		if err == nil {
			return quote, nil
		}
		if !fallbackEligible(err) {
			return QuoteResult{}, err
		}
		lastErr = err
	}
	return QuoteResult{}, exhaustion(lastErr)
}

// GetExecutableQuote implements Adapter.
func (c *Chain) GetExecutableQuote(ctx context.Context, req ExecutableQuoteRequest) (QuoteResult, error) {
	var lastErr error
	for _, adapter := range c.adapters {
		if !adapter.Capabilities().ExecutableQuote {
			continue
		}
		quote, err := adapter.GetExecutableQuote(ctx, req)
		if err == nil {
			return quote, nil
		}
		if !fallbackEligible(err) {
			return QuoteResult{}, err
		}
		lastErr = err
	}
	return QuoteResult{}, exhaustion(lastErr)
}

// GetStatus implements Adapter.
func (c *Chain) GetStatus(ctx context.Context, req StatusRequest) (StatusResult, error) {
	var lastErr error
	for _, adapter := range c.adapters {
		if !adapter.Capabilities().Status {
			continue
		}
		status, err := adapter.GetStatus(ctx, req)
		if err == nil {
			return status, nil
		}
		if !fallbackEligible(err) {
			return StatusResult{}, err
		}
		lastErr = err
	}
	return StatusResult{}, exhaustion(lastErr)
}

// SubmitDepositTx implements Adapter. It does not fall through to another
// provider: a deposit belongs to the provider that issued its address.
func (c *Chain) SubmitDepositTx(ctx context.Context, req DepositRequest) (StatusResult, error) {
	for _, adapter := range c.adapters {
		if !adapter.Capabilities().SubmitDeposit {
			continue
		}
		return adapter.SubmitDepositTx(ctx, req)
	}
	return StatusResult{}, domain.New(domain.CodeProviderNotConfigured,
		"No provider is configured to accept deposit notifications.")
}

func fallbackEligible(err error) bool {
	code := domain.CodeOf(err)
	return code == domain.CodeNoRoute || code == domain.CodeProviderNotConfigured
}

func exhaustion(lastErr error) error {
	if lastErr != nil {
		return lastErr
	}
	return domain.New(domain.CodeProviderNotConfigured,
		"No provider in the fallback chain supports this operation.")
}

// JoinNames returns a comma-separated adapter name list for logs and health.
func (c *Chain) JoinNames() string {
	names := make([]string, 0, len(c.adapters))
	for _, adapter := range c.adapters {
		names = append(names, adapter.Name())
	}
	return strings.Join(names, ",")
}
