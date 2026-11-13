package worker_test

import (
	"context"
	"testing"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
	"universal-defi-intelligence/internal/worker"
)

type fakeAdapter struct {
	caps   providers.Capabilities
	tokens []domain.Asset
	status providers.StatusResult
	err    error
}

func (f *fakeAdapter) Name() string                         { return "fake" }
func (f *fakeAdapter) Capabilities() providers.Capabilities { return f.caps }
func (f *fakeAdapter) Health() providers.Health             { return providers.Health{} }

func (f *fakeAdapter) GetTokens(context.Context) ([]domain.Asset, error) {
	return f.tokens, f.err
}

func (f *fakeAdapter) GetDryQuote(context.Context, providers.DryQuoteRequest) (providers.QuoteResult, error) {
	return providers.QuoteResult{}, f.err
}

func (f *fakeAdapter) GetExecutableQuote(context.Context, providers.ExecutableQuoteRequest) (providers.QuoteResult, error) {
	return providers.QuoteResult{}, f.err
}

func (f *fakeAdapter) GetStatus(context.Context, providers.StatusRequest) (providers.StatusResult, error) {
	return f.status, f.err
}

func (f *fakeAdapter) SubmitDepositTx(context.Context, providers.DepositRequest) (providers.StatusResult, error) {
	return providers.StatusResult{}, f.err
}

func TestAdapterProviderBridgesStatusAndTokens(t *testing.T) {
	t.Parallel()
	adapter := &fakeAdapter{
		caps:   providers.Capabilities{Catalogue: true, Status: true},
		tokens: []domain.Asset{{ID: "a"}},
		status: providers.StatusResult{
			ProviderStatus:    domain.ProviderStatusProcessing,
			SourceTxHash:      "src",
			DestinationTxHash: "dst",
			RefundTxHash:      "ref",
			Raw:               []byte(`{"status":"PROCESSING"}`),
		},
	}
	provider := worker.NewAdapterProvider(adapter)
	if provider.Name() != "fake" || !provider.Available() {
		t.Fatalf("unexpected provider metadata: %q %t", provider.Name(), provider.Available())
	}

	result, err := provider.GetStatus(context.Background(), worker.StatusRequest{DepositAddress: "address"})
	if err != nil {
		t.Fatalf("GetStatus() error: %v", err)
	}
	if result.Status != domain.ProviderStatusProcessing || result.SourceTxHash != "src" || result.RefundTxHash != "ref" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if string(result.Raw) == "" {
		t.Fatal("raw response should be forwarded")
	}

	tokens, err := provider.GetTokens(context.Background())
	if err != nil || len(tokens) != 1 {
		t.Fatalf("GetTokens() = %v, %v", tokens, err)
	}
}

func TestNilAdapterProviderIsUnavailable(t *testing.T) {
	t.Parallel()
	provider := worker.NewAdapterProvider(nil)
	if provider.Name() != "unconfigured" || provider.Available() {
		t.Fatalf("unexpected metadata: %q %t", provider.Name(), provider.Available())
	}
	if _, err := provider.GetStatus(context.Background(), worker.StatusRequest{}); !domain.IsCode(err, domain.CodeProviderNotConfigured) {
		t.Fatalf("err = %v", err)
	}
	if _, err := provider.GetTokens(context.Background()); !domain.IsCode(err, domain.CodeProviderNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}
