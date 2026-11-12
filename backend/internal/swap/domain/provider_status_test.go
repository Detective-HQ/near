package domain

import "testing"

func TestMapProviderStatus(t *testing.T) {
	tests := []struct {
		status               ProviderStatus
		hasSourceTransaction bool
		want                 State
	}{
		{ProviderStatusPendingDeposit, false, StateAwaitingUserDeposit},
		{ProviderStatusPendingDeposit, true, StateDepositSubmitted},
		{ProviderStatusKnownDepositTx, false, StateDepositSubmitted},
		{ProviderStatusKnownDepositTx, true, StateDepositSubmitted},
		{ProviderStatusIncomplete, false, StateIncompleteDeposit},
		{ProviderStatusProcessing, false, StateProcessing},
		{ProviderStatusSuccess, false, StateSucceeded},
		{ProviderStatusRefunded, false, StateRefunded},
		{ProviderStatusFailed, false, StateFailed},
	}

	for _, test := range tests {
		t.Run(string(test.status), func(t *testing.T) {
			got, err := MapProviderStatus(test.status, test.hasSourceTransaction)
			if err != nil {
				t.Fatalf("MapProviderStatus(%s, %t): %v", test.status, test.hasSourceTransaction, err)
			}
			if got != test.want {
				t.Fatalf("MapProviderStatus(%s, %t) = %s, want %s", test.status, test.hasSourceTransaction, got, test.want)
			}
		})
	}
}

func TestMapProviderStatusNormalizesWhitespace(t *testing.T) {
	got, err := MapProviderStatus("  SUCCESS  ", false)
	if err != nil {
		t.Fatalf("MapProviderStatus: %v", err)
	}
	if got != StateSucceeded {
		t.Fatalf("state = %s, want SUCCEEDED", got)
	}
}

func TestMapProviderStatusUnknown(t *testing.T) {
	for _, status := range []ProviderStatus{"", "COMPLETED", "success", "PENDING"} {
		_, err := MapProviderStatus(status, false)
		requireCode(t, err, CodeUnknownProviderStatus)
	}
}

func TestProviderStatusHelpers(t *testing.T) {
	if !ProviderStatusSuccess.Valid() {
		t.Fatal("SUCCESS should be valid")
	}
	if ProviderStatus("success").Valid() {
		t.Fatal("lowercase success should not be valid")
	}
	if !ProviderStatusSuccess.Terminal() || !ProviderStatusRefunded.Terminal() || !ProviderStatusFailed.Terminal() {
		t.Fatal("settlement statuses should be terminal")
	}
	if ProviderStatusProcessing.Terminal() || ProviderStatusPendingDeposit.Terminal() {
		t.Fatal("in-flight statuses should not be terminal")
	}
	if got := ProviderStatus("  PROCESSING ").Normalize(); got != ProviderStatusProcessing {
		t.Fatalf("Normalize() = %q", got)
	}
}

func TestMapProviderStatusWithSource(t *testing.T) {
	got, err := MapProviderStatusWithSource(ProviderStatusPendingDeposit, "0xabc")
	if err != nil {
		t.Fatalf("MapProviderStatusWithSource: %v", err)
	}
	if got != StateDepositSubmitted {
		t.Fatalf("state = %s, want DEPOSIT_SUBMITTED", got)
	}

	got, err = MapProviderStatusWithSource(ProviderStatusPendingDeposit, "   ")
	if err != nil {
		t.Fatalf("MapProviderStatusWithSource: %v", err)
	}
	if got != StateAwaitingUserDeposit {
		t.Fatalf("state = %s, want AWAITING_USER_DEPOSIT", got)
	}
}
