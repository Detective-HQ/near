package providers_test

import (
	"context"
	"testing"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

type fakeAdapter struct {
	name   string
	caps   providers.Capabilities
	health providers.Health
	err    error
	quote  providers.QuoteResult
	calls  int
}

func (f *fakeAdapter) Name() string                         { return f.name }
func (f *fakeAdapter) Capabilities() providers.Capabilities { return f.caps }
func (f *fakeAdapter) Health() providers.Health             { return f.health }

func (f *fakeAdapter) GetTokens(context.Context) ([]domain.Asset, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return []domain.Asset{{ID: f.name}}, nil
}

func (f *fakeAdapter) GetDryQuote(context.Context, providers.DryQuoteRequest) (providers.QuoteResult, error) {
	f.calls++
	if f.err != nil {
		return providers.QuoteResult{}, f.err
	}
	return f.quote, nil
}

func (f *fakeAdapter) GetExecutableQuote(context.Context, providers.ExecutableQuoteRequest) (providers.QuoteResult, error) {
	f.calls++
	if f.err != nil {
		return providers.QuoteResult{}, f.err
	}
	return f.quote, nil
}

func (f *fakeAdapter) GetStatus(context.Context, providers.StatusRequest) (providers.StatusResult, error) {
	f.calls++
	if f.err != nil {
		return providers.StatusResult{}, f.err
	}
	return providers.StatusResult{ProviderStatus: domain.ProviderStatusProcessing}, nil
}

func (f *fakeAdapter) SubmitDepositTx(context.Context, providers.DepositRequest) (providers.StatusResult, error) {
	f.calls++
	if f.err != nil {
		return providers.StatusResult{}, f.err
	}
	return providers.StatusResult{ProviderStatus: domain.ProviderStatusKnownDepositTx}, nil
}

func TestChainFallsBackOnNoRoute(t *testing.T) {
	t.Parallel()
	primary := &fakeAdapter{
		name: "primary",
		caps: providers.Capabilities{DryQuote: true},
		err:  domain.New(domain.CodeNoRoute, "no route"),
	}
	secondary := &fakeAdapter{
		name:  "secondary",
		caps:  providers.Capabilities{DryQuote: true},
		quote: providers.QuoteResult{Quote: domain.Quote{ProviderName: "secondary"}},
	}
	chain := providers.NewChain(primary, secondary)

	result, err := chain.GetDryQuote(context.Background(), providers.DryQuoteRequest{})
	if err != nil {
		t.Fatalf("GetDryQuote() error: %v", err)
	}
	if result.Quote.ProviderName != "secondary" {
		t.Fatalf("provider = %q", result.Quote.ProviderName)
	}
	if primary.calls != 1 || secondary.calls != 1 {
		t.Fatalf("calls = %d/%d", primary.calls, secondary.calls)
	}
}

func TestChainDoesNotHideRealProviderFailures(t *testing.T) {
	t.Parallel()
	primary := &fakeAdapter{
		name: "primary",
		caps: providers.Capabilities{DryQuote: true},
		err:  domain.New(domain.CodeProviderAuthFailed, "bad credentials"),
	}
	secondary := &fakeAdapter{
		name:  "secondary",
		caps:  providers.Capabilities{DryQuote: true},
		quote: providers.QuoteResult{Quote: domain.Quote{ProviderName: "secondary"}},
	}
	chain := providers.NewChain(primary, secondary)

	_, err := chain.GetDryQuote(context.Background(), providers.DryQuoteRequest{})
	if !domain.IsCode(err, domain.CodeProviderAuthFailed) {
		t.Fatalf("err = %v, want PROVIDER_AUTH_FAILED", err)
	}
	if secondary.calls != 0 {
		t.Fatalf("secondary should not be called, got %d", secondary.calls)
	}
}

func TestChainReportsExhaustion(t *testing.T) {
	t.Parallel()
	chain := providers.NewChain()
	if _, err := chain.GetTokens(context.Background()); !domain.IsCode(err, domain.CodeProviderNotConfigured) {
		t.Fatalf("err = %v", err)
	}
	if chain.Name() != "unconfigured" {
		t.Fatalf("name = %q", chain.Name())
	}
}

func TestChainAggregatesCapabilities(t *testing.T) {
	t.Parallel()
	chain := providers.NewChain(
		&fakeAdapter{name: "a", caps: providers.Capabilities{Catalogue: true}},
		&fakeAdapter{name: "b", caps: providers.Capabilities{DryQuote: true, SignatureVerification: true}, health: providers.Health{Configured: true}},
	)
	caps := chain.Capabilities()
	if !caps.Catalogue || !caps.DryQuote || !caps.SignatureVerification {
		t.Fatalf("caps = %+v", caps)
	}
	if !chain.Health().Configured {
		t.Fatal("chain should be configured when any adapter is")
	}
	if chain.JoinNames() != "a,b" {
		t.Fatalf("names = %q", chain.JoinNames())
	}
}
