package worker_test

import (
	"context"
	"testing"
	"time"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/worker"
)

func TestUnavailableProviderReportsNotConfigured(t *testing.T) {
	provider := worker.NewUnavailableProvider("oneclick")

	if got := provider.Name(); got != "oneclick" {
		t.Fatalf("Name = %q, want oneclick", got)
	}
	if provider.Available() {
		t.Fatal("Available = true, want false")
	}

	if _, err := provider.GetStatus(context.Background(), worker.StatusRequest{SwapID: "swp_1"}); !domain.IsCode(err, domain.CodeProviderNotConfigured) {
		t.Fatalf("GetStatus error = %v, want PROVIDER_NOT_CONFIGURED", err)
	}
	if _, err := provider.GetTokens(context.Background()); !domain.IsCode(err, domain.CodeProviderNotConfigured) {
		t.Fatalf("GetTokens error = %v, want PROVIDER_NOT_CONFIGURED", err)
	}
}

func TestNewUnavailableProviderDefaultName(t *testing.T) {
	cases := []string{"", "   "}
	for _, name := range cases {
		if got := worker.NewUnavailableProvider(name).Name(); got != "unconfigured" {
			t.Fatalf("Name(%q) = %q, want unconfigured", name, got)
		}
	}
	if got := worker.NewUnavailableProvider("  oneclick  ").Name(); got != "oneclick" {
		t.Fatalf("Name = %q, want trimmed oneclick", got)
	}
}

func TestSystemClockReturnsUTC(t *testing.T) {
	before := time.Now().UTC()
	now := worker.SystemClock{}.Now()
	after := time.Now().UTC()

	if now.Location() != time.UTC {
		t.Fatalf("clock location = %s, want UTC", now.Location())
	}
	if now.Before(before) || now.After(after) {
		t.Fatalf("clock returned %s outside [%s, %s]", now, before, after)
	}
}
