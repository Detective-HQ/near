package repository_test

import (
	"context"
	"testing"

	"universal-defi-intelligence/internal/swap/domain"
)

func awaitingSwap(quoteID, id, key string) domain.Swap {
	swap := testSwap(quoteID, id, key, testPayloadHashA)
	swap.State = domain.StateAwaitingUserDeposit
	return swap
}

func TestRecordDepositIsAtomicAndUnique(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	seedQuote(t, store, "qte_dep")
	swap := mustCreateSwap(t, store, awaitingSwap("qte_dep", "swp_dep_a", "key-dep-a"), "key-dep-a")
	other := mustCreateSwap(t, store, awaitingSwap("qte_dep", "swp_dep_b", "key-dep-b"), "key-dep-b")

	updated, deposit, duplicate, err := store.RecordDeposit(ctx, swap.ID, "ethereum", "0xaaa",
		domain.ProviderStatusKnownDepositTx, map[string]any{"source": "test"})
	if err != nil {
		t.Fatalf("RecordDeposit: %v", err)
	}
	if duplicate || deposit.ID == 0 || updated.State != domain.StateDepositSubmitted || updated.SourceTxHash != "0xaaa" {
		t.Fatalf("unexpected result swap=%+v deposit=%+v dup=%v", updated, deposit, duplicate)
	}

	again, _, duplicate, err := store.RecordDeposit(ctx, swap.ID, "ethereum", "0xaaa", domain.ProviderStatusKnownDepositTx, nil)
	if err != nil || !duplicate || again.State != domain.StateDepositSubmitted {
		t.Fatalf("replay = %+v dup=%v err=%v", again, duplicate, err)
	}
	_, _, _, err = store.RecordDeposit(ctx, swap.ID, "ethereum", "0xbbb", domain.ProviderStatusKnownDepositTx, nil)
	mustCode(t, err, domain.CodeConflict)

	// One on-chain deposit cannot fund two swaps.
	_, _, _, err = store.RecordDeposit(ctx, other.ID, "ethereum", "0xaaa", domain.ProviderStatusKnownDepositTx, nil)
	mustCode(t, err, domain.CodeConflict)
	reloaded, err := store.GetSwapByID(ctx, other.ID)
	if err != nil || reloaded.State != domain.StateAwaitingUserDeposit || reloaded.SourceTxHash != "" {
		t.Fatalf("failed deposit leaked partial state: %+v %v", reloaded, err)
	}

	events, err := store.ListEventsBySwapIDs(ctx, []string{swap.ID, other.ID})
	if err != nil || len(events[swap.ID]) != 2 || len(events[other.ID]) != 1 {
		t.Fatalf("events = %d/%d err=%v", len(events[swap.ID]), len(events[other.ID]), err)
	}
	txs, err := store.ListTransactionsBySwapIDs(ctx, []string{swap.ID})
	if err != nil || len(txs[swap.ID]) != 1 {
		t.Fatalf("transactions = %v err=%v", txs, err)
	}
	quotes, err := store.GetQuotes(ctx, []string{"qte_dep", "qte_missing"})
	if err != nil || len(quotes) != 1 {
		t.Fatalf("quotes = %v err=%v", quotes, err)
	}
}

func TestSimulatedSwapsAreNeverPolledOrSettled(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	seedQuote(t, store, "qte_sim")
	sim := awaitingSwap("qte_sim", "swp_sim", "key-sim")
	sim.Simulated = true
	sim.SessionID = ""
	created := mustCreateSwap(t, store, sim, "key-sim")
	if !created.Simulated {
		t.Fatal("simulated flag not persisted")
	}

	claimed, err := store.ClaimDueSwaps(ctx, created.NextPollAt.Add(1), 10, 30e9)
	if err != nil {
		t.Fatal(err)
	}
	for _, swap := range claimed {
		if swap.ID == created.ID {
			t.Fatal("simulated swap was claimed for provider polling")
		}
	}

	state := created.State
	for _, step := range []struct {
		to     domain.State
		status domain.ProviderStatus
	}{
		{domain.StateDepositSubmitted, domain.ProviderStatusKnownDepositTx},
		{domain.StateProcessing, domain.ProviderStatusProcessing},
		{domain.StateSucceeded, domain.ProviderStatusSuccess},
	} {
		updated, _, err := store.UpdateSwapState(ctx, created.ID, state, step.to, step.status, nil)
		if err != nil {
			t.Fatalf("-> %s: %v", step.to, err)
		}
		state = updated.State
		if updated.RealSettlement {
			t.Fatal("simulated swap marked as real settlement")
		}
	}

	anonymous, _, err := store.ListSwapsBySession(ctx, "", 10, "")
	if err != nil || len(anonymous) != 1 {
		t.Fatalf("anonymous listing = %d err=%v", len(anonymous), err)
	}
}

func TestGetSwapByIdempotencyKey(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	seedQuote(t, store, "qte_idem")
	created := mustCreateSwap(t, store, awaitingSwap("qte_idem", "swp_idem", "key-idem"), "key-idem")
	got, err := store.GetSwapByIdempotencyKey(ctx, "key-idem")
	if err != nil || got.ID != created.ID {
		t.Fatalf("got %+v err=%v", got, err)
	}
	_, err = store.GetSwapByIdempotencyKey(ctx, "key-missing")
	mustCode(t, err, domain.CodeSwapNotFound)
}
