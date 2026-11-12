package domain

import (
	"errors"
	"testing"
	"time"
)

func TestAllStatesAreValid(t *testing.T) {
	for _, state := range AllStates() {
		if !state.Valid() {
			t.Fatalf("state %s is not valid", state)
		}
		if _, ok := state.Rank(); !ok {
			t.Fatalf("state %s has no rank", state)
		}
	}
	if State("NOT_A_STATE").Valid() {
		t.Fatal("unknown state must not be valid")
	}
}

func TestCanTransitionAllowed(t *testing.T) {
	tests := []struct {
		from State
		to   State
	}{
		{StateCreated, StateQuoting},
		{StateQuoting, StateQuoted},
		{StateQuoting, StateQuoteUnavailable},
		{StateQuoted, StateAwaitingUserDeposit},
		{StateQuoted, StateCancelled},
		{StateQuoted, StateExpired},
		{StateQuoted, StateFailed},
		{StateAwaitingUserDeposit, StateDepositSubmitted},
		{StateAwaitingUserDeposit, StateCancelled},
		{StateAwaitingUserDeposit, StateExpired},
		{StateAwaitingUserDeposit, StateFailed},
		{StateDepositSubmitted, StateProcessing},
		{StateDepositSubmitted, StateIncompleteDeposit},
		{StateDepositSubmitted, StateRefunded},
		{StateDepositSubmitted, StateFailed},
		{StateIncompleteDeposit, StateProcessing},
		{StateIncompleteDeposit, StateRefunded},
		{StateIncompleteDeposit, StateFailed},
		{StateProcessing, StateSucceeded},
		{StateProcessing, StateRefunded},
		{StateProcessing, StateFailed},
	}

	for _, test := range tests {
		t.Run(string(test.from)+"->"+string(test.to), func(t *testing.T) {
			if err := CanTransition(test.from, test.to); err != nil {
				t.Fatalf("expected %s -> %s to be allowed: %v", test.from, test.to, err)
			}
		})
	}
}

func TestCanTransitionRejected(t *testing.T) {
	tests := []struct {
		from State
		to   State
	}{
		{StateCreated, StateQuoted},
		{StateCreated, StateSucceeded},
		{StateQuoting, StateAwaitingUserDeposit},
		{StateQuoted, StateProcessing},
		{StateQuoted, StateSucceeded},
		{StateAwaitingUserDeposit, StateProcessing},
		{StateDepositSubmitted, StateSucceeded},
		{StateIncompleteDeposit, StateSucceeded},
		{StateProcessing, StateDepositSubmitted},
		{StateProcessing, StateQuoted},
		{StateSucceeded, StateRefunded},
		{StateFailed, StateProcessing},
		{StateCancelled, StateQuoting},
		{StateExpired, StateQuoted},
		{StateQuoteUnavailable, StateQuoted},
	}

	for _, test := range tests {
		t.Run(string(test.from)+"->"+string(test.to), func(t *testing.T) {
			if err := CanTransition(test.from, test.to); err == nil {
				t.Fatalf("expected %s -> %s to be rejected", test.from, test.to)
			}
		})
	}
}

func TestCanTransitionRejectsSameState(t *testing.T) {
	err := CanTransition(StateQuoted, StateQuoted)
	requireCode(t, err, CodeInvalidSwapState)
}

func TestCanTransitionRejectsUnknownStates(t *testing.T) {
	requireCode(t, CanTransition(State("BOGUS"), StateQuoted), CodeInvalidSwapState)
	requireCode(t, CanTransition(StateQuoted, State("BOGUS")), CodeInvalidSwapState)
}

func TestCanTransitionTerminal(t *testing.T) {
	err := CanTransition(StateSucceeded, StateProcessing)
	requireCode(t, err, CodeTerminalState)
	if !errors.Is(err, New(CodeTerminalState, "")) {
		t.Fatal("expected errors.Is to match terminal code")
	}
}

func TestTransitionMatrixIsExhaustiveAndMonotonic(t *testing.T) {
	allowed := map[State]map[State]bool{
		StateCreated:             {StateQuoting: true},
		StateQuoting:             {StateQuoted: true, StateQuoteUnavailable: true},
		StateQuoted:              {StateAwaitingUserDeposit: true, StateCancelled: true, StateExpired: true, StateFailed: true},
		StateAwaitingUserDeposit: {StateDepositSubmitted: true, StateCancelled: true, StateExpired: true, StateFailed: true},
		StateDepositSubmitted:    {StateProcessing: true, StateIncompleteDeposit: true, StateRefunded: true, StateFailed: true},
		StateIncompleteDeposit:   {StateProcessing: true, StateRefunded: true, StateFailed: true},
		StateProcessing:          {StateSucceeded: true, StateRefunded: true, StateFailed: true},
	}

	states := AllStates()
	for _, from := range states {
		for _, to := range states {
			err := CanTransition(from, to)
			want := allowed[from][to]
			if want && err != nil {
				t.Fatalf("expected %s -> %s to be allowed: %v", from, to, err)
			}
			if !want && err == nil {
				t.Fatalf("expected %s -> %s to be rejected", from, to)
			}
			if want {
				fromRank, _ := from.Rank()
				toRank, _ := to.Rank()
				if toRank < fromRank {
					t.Fatalf("transition %s -> %s is not monotonic: rank %d -> %d", from, to, fromRank, toRank)
				}
			}
		}
	}
}

func TestApplyTransition(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	swap := NewSwap("swp_1", "qte_1", now)
	payload := map[string]any{"providerQuoteId": "pq_1"}

	updated, event, err := Apply(swap, TransitionInput{
		To:             StateQuoting,
		ProviderStatus: ProviderStatusPendingDeposit,
		Payload:        payload,
		Now:            now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if updated.State != StateQuoting {
		t.Fatalf("state = %s, want QUOTING", updated.State)
	}
	if updated.ProviderStatus != ProviderStatusPendingDeposit {
		t.Fatalf("provider status = %s", updated.ProviderStatus)
	}
	if updated.RealSettlement {
		t.Fatal("non-settlement transition set RealSettlement")
	}
	if !updated.UpdatedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("updatedAt = %s", updated.UpdatedAt)
	}
	if !updated.CreatedAt.Equal(now) {
		t.Fatalf("createdAt changed to %s", updated.CreatedAt)
	}
	if event.SwapID != "swp_1" || event.OldState != StateCreated || event.NewState != StateQuoting {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.Payload["providerQuoteId"] != "pq_1" {
		t.Fatalf("payload not copied: %+v", event.Payload)
	}

	payload["providerQuoteId"] = "mutated"
	if event.Payload["providerQuoteId"] != "pq_1" {
		t.Fatal("event payload aliases the input map")
	}
}

func TestApplyRejectsInvalidTransition(t *testing.T) {
	swap := NewSwap("swp_1", "qte_1", time.Now())
	_, _, err := Apply(swap, TransitionInput{To: StateSucceeded})
	requireCode(t, err, CodeInvalidSwapState)
	if swap.State != StateCreated {
		t.Fatal("failed transition mutated the swap")
	}
}

func TestApplyRequiresVerifiedSettlement(t *testing.T) {
	now := time.Now().UTC()
	swap := NewSwap("swp_1", "qte_1", now)
	swap.State = StateProcessing

	_, _, err := Apply(swap, TransitionInput{To: StateSucceeded, Now: now})
	requireCode(t, err, CodeInvalidSwapState)

	updated, event, err := Apply(swap, TransitionInput{
		To:                      StateSucceeded,
		ProviderStatus:          ProviderStatusSuccess,
		VerifiedProviderOutcome: true,
		Now:                     now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("verified settlement failed: %v", err)
	}
	if !updated.RealSettlement {
		t.Fatal("verified success must set RealSettlement")
	}
	if event.NewState != StateSucceeded {
		t.Fatalf("event state = %s", event.NewState)
	}
}

func TestApplyRequiresVerifiedRefund(t *testing.T) {
	now := time.Now().UTC()
	swap := NewSwap("swp_1", "qte_1", now)
	swap.State = StateProcessing

	_, _, err := Apply(swap, TransitionInput{To: StateRefunded, Now: now})
	requireCode(t, err, CodeInvalidSwapState)

	updated, _, err := Apply(swap, TransitionInput{
		To:                      StateRefunded,
		ProviderStatus:          ProviderStatusRefunded,
		VerifiedProviderOutcome: true,
		Now:                     now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("verified refund failed: %v", err)
	}
	if updated.RealSettlement {
		t.Fatal("refund must not set RealSettlement")
	}
}

func TestApplyRejectsTransitionFromTerminal(t *testing.T) {
	swap := NewSwap("swp_1", "qte_1", time.Now())
	swap.State = StateSucceeded
	_, _, err := Apply(swap, TransitionInput{To: StateRefunded, VerifiedProviderOutcome: true})
	requireCode(t, err, CodeTerminalState)
}

func TestApplyDefaultsTimestamp(t *testing.T) {
	swap := NewSwap("swp_1", "qte_1", time.Time{})
	if swap.CreatedAt.IsZero() {
		t.Fatal("NewSwap did not set CreatedAt")
	}
	updated, event, err := Apply(swap, TransitionInput{To: StateQuoting})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if updated.UpdatedAt.IsZero() || event.CreatedAt.IsZero() {
		t.Fatal("Apply did not set timestamps")
	}
	if updated.UpdatedAt.Location() != time.UTC {
		t.Fatalf("timestamp is not UTC: %s", updated.UpdatedAt)
	}
}

func TestInitialEventHasEmptyOldState(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	swap := NewSwap("swp_1", "qte_1", now)
	event := InitialEvent(swap)
	if event.OldState != "" {
		t.Fatalf("initial event old state = %q, want empty", event.OldState)
	}
	if event.NewState != StateCreated {
		t.Fatalf("initial event new state = %s", event.NewState)
	}
	if !event.CreatedAt.Equal(now) {
		t.Fatalf("event time = %s, want %s", event.CreatedAt, now)
	}
}

func TestParseState(t *testing.T) {
	tests := []struct {
		value string
		want  State
		ok    bool
	}{
		{"created", StateCreated, true},
		{"  PROCESSING ", StateProcessing, true},
		{"QUOTE_UNAVAILABLE", StateQuoteUnavailable, true},
		{"bogus", "", false},
		{"", "", false},
	}
	for _, test := range tests {
		got, ok := ParseState(test.value)
		if ok != test.ok || got != test.want {
			t.Fatalf("ParseState(%q) = (%s, %t), want (%s, %t)", test.value, got, ok, test.want, test.ok)
		}
	}
}

func TestTerminalStates(t *testing.T) {
	terminal := []State{StateQuoteUnavailable, StateSucceeded, StateRefunded, StateFailed, StateCancelled, StateExpired}
	for _, state := range terminal {
		if !state.Terminal() {
			t.Fatalf("%s should be terminal", state)
		}
	}
	nonTerminal := []State{StateCreated, StateQuoting, StateQuoted, StateAwaitingUserDeposit, StateDepositSubmitted, StateIncompleteDeposit, StateProcessing}
	for _, state := range nonTerminal {
		if state.Terminal() {
			t.Fatalf("%s should not be terminal", state)
		}
	}
}

func TestApplySetsMissingCreatedAt(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	swap := Swap{ID: "swp_1", State: StateCreated}
	updated, _, err := Apply(swap, TransitionInput{To: StateQuoting, Now: now})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !updated.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %s, want %s", updated.CreatedAt, now)
	}
}

func TestStateGoString(t *testing.T) {
	if got := StateProcessing.GoString(); got != `domain.State("PROCESSING")` {
		t.Fatalf("GoString() = %q", got)
	}
}

func TestApplyProviderUpdateRecordsSameStateObservation(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	swap := NewSwap("swp_1", "qte_1", now)
	swap.State = StateIncompleteDeposit
	swap.ProviderStatus = ProviderStatusIncomplete
	payload := map[string]any{"reason": "partial deposit"}

	updated, event, recorded, err := ApplyProviderUpdate(swap, ProviderUpdateInput{
		Status:  ProviderStatusIncomplete,
		Payload: payload,
		Now:     now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ApplyProviderUpdate: %v", err)
	}
	if !recorded {
		t.Fatal("same-state observation was not recorded")
	}
	if updated.State != StateIncompleteDeposit {
		t.Fatalf("state = %s, want INCOMPLETE_DEPOSIT", updated.State)
	}
	if !updated.UpdatedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("updatedAt = %s", updated.UpdatedAt)
	}
	if event.OldState != StateIncompleteDeposit || event.NewState != StateIncompleteDeposit {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.ProviderStatus != ProviderStatusIncomplete {
		t.Fatalf("event provider status = %s", event.ProviderStatus)
	}
	payload["reason"] = "mutated"
	if event.Payload["reason"] != "partial deposit" {
		t.Fatal("event payload aliases the input map")
	}
}

func TestApplyProviderUpdateMovesForward(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	swap := NewSwap("swp_1", "qte_1", now)
	swap.State = StateAwaitingUserDeposit

	updated, event, recorded, err := ApplyProviderUpdate(swap, ProviderUpdateInput{
		Status:               ProviderStatusKnownDepositTx,
		HasSourceTransaction: true,
		Now:                  now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ApplyProviderUpdate: %v", err)
	}
	if !recorded {
		t.Fatal("forward observation was not recorded")
	}
	if updated.State != StateDepositSubmitted {
		t.Fatalf("state = %s, want DEPOSIT_SUBMITTED", updated.State)
	}
	if event.OldState != StateAwaitingUserDeposit || event.NewState != StateDepositSubmitted {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestApplyProviderUpdateIgnoresStaleStatus(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	swap := NewSwap("swp_1", "qte_1", now)
	swap.State = StateProcessing
	swap.ProviderStatus = ProviderStatusProcessing
	before := swap

	updated, event, recorded, err := ApplyProviderUpdate(swap, ProviderUpdateInput{
		Status: ProviderStatusPendingDeposit,
		Now:    now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ApplyProviderUpdate: %v", err)
	}
	if recorded {
		t.Fatal("stale provider status must not be recorded")
	}
	if event.SwapID != "" || event.NewState != "" || !event.CreatedAt.IsZero() {
		t.Fatalf("stale provider status produced an event: %+v", event)
	}
	if updated != before {
		t.Fatalf("stale provider status mutated the swap: %+v", updated)
	}
}

func TestApplyProviderUpdateIgnoresSameRankRegression(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	swap := NewSwap("swp_1", "qte_1", now)
	swap.State = StateIncompleteDeposit
	before := swap

	updated, _, recorded, err := ApplyProviderUpdate(swap, ProviderUpdateInput{
		Status:               ProviderStatusKnownDepositTx,
		HasSourceTransaction: true,
		Now:                  now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ApplyProviderUpdate: %v", err)
	}
	if recorded {
		t.Fatal("same-rank regression must not be recorded")
	}
	if updated != before {
		t.Fatalf("same-rank regression mutated the swap: %+v", updated)
	}
}

func TestApplyProviderUpdateRejectsSkip(t *testing.T) {
	swap := NewSwap("swp_1", "qte_1", time.Now())
	swap.State = StateAwaitingUserDeposit

	_, _, recorded, err := ApplyProviderUpdate(swap, ProviderUpdateInput{
		Status: ProviderStatusProcessing,
	})
	requireCode(t, err, CodeInvalidSwapState)
	if recorded {
		t.Fatal("invalid skip must not be recorded")
	}
}

func TestApplyProviderUpdateRequiresVerifiedSettlement(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	swap := NewSwap("swp_1", "qte_1", now)
	swap.State = StateProcessing

	_, _, recorded, err := ApplyProviderUpdate(swap, ProviderUpdateInput{
		Status: ProviderStatusSuccess,
		Now:    now.Add(time.Minute),
	})
	requireCode(t, err, CodeInvalidSwapState)
	if recorded {
		t.Fatal("unverified settlement must not be recorded")
	}

	updated, event, recorded, err := ApplyProviderUpdate(swap, ProviderUpdateInput{
		Status:                  ProviderStatusSuccess,
		VerifiedProviderOutcome: true,
		Now:                     now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("verified settlement failed: %v", err)
	}
	if !recorded || updated.State != StateSucceeded || !updated.RealSettlement {
		t.Fatalf("unexpected settlement result: %+v recorded=%t", updated, recorded)
	}
	if event.NewState != StateSucceeded {
		t.Fatalf("event state = %s", event.NewState)
	}
}

func TestApplyProviderUpdateUnknownStatus(t *testing.T) {
	swap := NewSwap("swp_1", "qte_1", time.Now())
	swap.State = StateProcessing

	_, _, recorded, err := ApplyProviderUpdate(swap, ProviderUpdateInput{
		Status: "COMPLETED",
	})
	requireCode(t, err, CodeUnknownProviderStatus)
	if recorded {
		t.Fatal("unknown provider status must not be recorded")
	}
}

func TestApplyProviderUpdateRejectsUnknownCurrentState(t *testing.T) {
	swap := Swap{ID: "swp_1", State: State("BOGUS")}
	_, _, recorded, err := ApplyProviderUpdate(swap, ProviderUpdateInput{Status: ProviderStatusProcessing})
	requireCode(t, err, CodeInvalidSwapState)
	if recorded {
		t.Fatal("unknown current state must not be recorded")
	}
}
