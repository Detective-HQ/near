package alternative_test

import (
	"context"
	"testing"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
	"universal-defi-intelligence/internal/swap/providers/alternative"
)

func TestStubReportsNoRouteAndNotConfigured(t *testing.T) {
	t.Parallel()
	adapter := alternative.New()
	if adapter.Name() != "alternative" {
		t.Fatalf("name = %q", adapter.Name())
	}
	if caps := adapter.Capabilities(); caps.Catalogue || caps.DryQuote || caps.Status {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
	if adapter.Health().Configured {
		t.Fatal("stub must not report itself as configured")
	}

	if _, err := adapter.GetTokens(context.Background()); !domain.IsCode(err, domain.CodeProviderNotConfigured) {
		t.Fatalf("GetTokens err = %v", err)
	}
	if _, err := adapter.GetDryQuote(context.Background(), providers.DryQuoteRequest{}); !domain.IsCode(err, domain.CodeNoRoute) {
		t.Fatalf("GetDryQuote err = %v", err)
	}
	if _, err := adapter.GetExecutableQuote(context.Background(), providers.ExecutableQuoteRequest{}); !domain.IsCode(err, domain.CodeNoRoute) {
		t.Fatalf("GetExecutableQuote err = %v", err)
	}
	if _, err := adapter.GetStatus(context.Background(), providers.StatusRequest{}); !domain.IsCode(err, domain.CodeProviderNotConfigured) {
		t.Fatalf("GetStatus err = %v", err)
	}
	if _, err := adapter.SubmitDepositTx(context.Background(), providers.DepositRequest{}); !domain.IsCode(err, domain.CodeProviderNotConfigured) {
		t.Fatalf("SubmitDepositTx err = %v", err)
	}
}
