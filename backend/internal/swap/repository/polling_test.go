package repository_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/repository"
)

func forceSwapPolling(t *testing.T, ts *testSchema, id string, state domain.State, nextPollAt time.Time) {
	t.Helper()
	if _, err := ts.pool.Exec(context.Background(),
		`UPDATE swaps SET state = $2, next_poll_at = $3, updated_at = $3 WHERE id = $1`,
		id, string(state), nextPollAt.UTC(),
	); err != nil {
		t.Fatalf("force swap %s polling state: %v", id, err)
	}
}

func seedPollableSwap(t *testing.T, store *repository.Store, ts *testSchema, id string, state domain.State, nextPollAt time.Time) {
	t.Helper()
	quoteID := "qte_" + id
	seedQuote(t, store, quoteID)
	swap := testSwap(quoteID, id, "idem_"+id, testPayloadHashA)
	mustCreateSwap(t, store, swap, "idem_"+id)
	forceSwapPolling(t, ts, id, state, nextPollAt)
}

func TestClaimDueSwapsLeasesUntilExpiry(t *testing.T) {
	store, ts := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedPollableSwap(t, store, ts, "swp_lease", domain.StateDepositSubmitted, now.Add(-time.Minute))

	first, err := store.ClaimDueSwaps(ctx, now, 10, 30*time.Second)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if len(first) != 1 || first[0].ID != "swp_lease" {
		t.Fatalf("first claim = %+v, want one swap swp_lease", first)
	}
	if !first[0].NextPollAt.After(now) {
		t.Fatalf("lease did not push next_poll_at forward: %s", first[0].NextPollAt)
	}

	second, err := store.ClaimDueSwaps(ctx, now, 10, 30*time.Second)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("second claim returned %d swaps, want 0 while leased", len(second))
	}

	after := now.Add(31 * time.Second)
	third, err := store.ClaimDueSwaps(ctx, after, 10, 30*time.Second)
	if err != nil {
		t.Fatalf("third claim: %v", err)
	}
	if len(third) != 1 || third[0].ID != "swp_lease" {
		t.Fatalf("third claim = %+v, want the swap after the lease expired", third)
	}
}

func TestClaimDueSwapsSkipsStuckTerminalAndUnpollable(t *testing.T) {
	store, ts := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	due := now.Add(-time.Minute)

	seedPollableSwap(t, store, ts, "swp_due", domain.StateProcessing, due)
	seedPollableSwap(t, store, ts, "swp_terminal", domain.StateSucceeded, due)
	seedPollableSwap(t, store, ts, "swp_created", domain.StateCreated, due)
	seedPollableSwap(t, store, ts, "swp_stuck", domain.StateDepositSubmitted, due)

	if _, err := ts.pool.Exec(ctx, `UPDATE swaps SET stuck_at = $2 WHERE id = $1`, "swp_stuck", now); err != nil {
		t.Fatalf("mark stuck: %v", err)
	}

	claimed, err := store.ClaimDueSwaps(ctx, now, 10, 30*time.Second)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != "swp_due" {
		t.Fatalf("claimed = %+v, want only swp_due", claimed)
	}
}

func TestClaimDueSwapsSingleClaimAcrossWorkers(t *testing.T) {
	store, ts := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedPollableSwap(t, store, ts, "swp_race", domain.StateDepositSubmitted, now.Add(-time.Minute))

	var waitGroup sync.WaitGroup
	var mu sync.Mutex
	var claimedIDs []string
	for worker := 0; worker < 4; worker++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			swaps, err := store.ClaimDueSwaps(ctx, now, 10, 30*time.Second)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, swap := range swaps {
				claimedIDs = append(claimedIDs, swap.ID)
			}
		}()
	}
	waitGroup.Wait()

	if len(claimedIDs) != 1 {
		t.Fatalf("claimed %v, want exactly one claim across workers", claimedIDs)
	}
}

func TestRecordPollResultAdvancesAndWritesEvent(t *testing.T) {
	store, ts := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedPollableSwap(t, store, ts, "swp_record", domain.StateDepositSubmitted, now)

	updated, event, err := store.RecordPollResult(ctx, repository.PollResult{
		SwapID:         "swp_record",
		OldState:       domain.StateDepositSubmitted,
		NewState:       domain.StateProcessing,
		ProviderStatus: domain.ProviderStatusProcessing,
		SourceTxHash:   "0xdeadbeef",
		Payload:        map[string]any{"providerStatus": "PROCESSING"},
		NextPollAt:     now.Add(time.Minute),
		ObservedAt:     now,
	})
	if err != nil {
		t.Fatalf("record poll result: %v", err)
	}
	if updated.State != domain.StateProcessing {
		t.Fatalf("updated state = %s, want PROCESSING", updated.State)
	}
	if updated.SourceTxHash != "0xdeadbeef" {
		t.Fatalf("source tx hash = %q, want 0xdeadbeef", updated.SourceTxHash)
	}
	if event.ID == 0 || event.OldState != domain.StateDepositSubmitted || event.NewState != domain.StateProcessing {
		t.Fatalf("event = %+v", event)
	}

	events, err := store.ListEvents(ctx, "swp_record")
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	advanced := 0
	for _, item := range events {
		if item.OldState == domain.StateDepositSubmitted && item.NewState == domain.StateProcessing {
			advanced++
		}
	}
	if advanced != 1 {
		t.Fatalf("PROCESSING transition events = %d, want 1", advanced)
	}
}

func TestRecordPollResultStateConflict(t *testing.T) {
	store, ts := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedPollableSwap(t, store, ts, "swp_conflict", domain.StateDepositSubmitted, now)

	_, _, err := store.RecordPollResult(ctx, repository.PollResult{
		SwapID:         "swp_conflict",
		OldState:       domain.StateProcessing,
		NewState:       domain.StateSucceeded,
		ProviderStatus: domain.ProviderStatusSuccess,
		ObservedAt:     now,
	})
	mustCode(t, err, domain.CodeStateConflict)
}

func TestRecordPollResultRejectsIllegalTransition(t *testing.T) {
	store, ts := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedPollableSwap(t, store, ts, "swp_illegal", domain.StateExpired, now)

	_, _, err := store.RecordPollResult(ctx, repository.PollResult{
		SwapID:         "swp_illegal",
		OldState:       domain.StateExpired,
		NewState:       domain.StateProcessing,
		ProviderStatus: domain.ProviderStatusProcessing,
		ObservedAt:     now,
	})
	mustCode(t, err, domain.CodeTerminalState)
}

func TestRecordPollResultRequiresVerifiedSuccess(t *testing.T) {
	store, ts := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedPollableSwap(t, store, ts, "swp_success", domain.StateProcessing, now)

	_, _, err := store.RecordPollResult(ctx, repository.PollResult{
		SwapID:         "swp_success",
		OldState:       domain.StateProcessing,
		NewState:       domain.StateSucceeded,
		ProviderStatus: domain.ProviderStatusProcessing,
		ObservedAt:     now,
	})
	mustCode(t, err, domain.CodeInvalidSwapState)
}

func TestRescheduleSwapUpdatesScheduleOnly(t *testing.T) {
	store, ts := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedPollableSwap(t, store, ts, "swp_resched", domain.StateProcessing, now)
	next := now.Add(2 * time.Minute)

	updated, err := store.RescheduleSwap(ctx, "swp_resched", next, 3, domain.CodeProviderRateLimited, now)
	if err != nil {
		t.Fatalf("reschedule: %v", err)
	}
	if updated.State != domain.StateProcessing {
		t.Fatalf("state changed to %s, want PROCESSING", updated.State)
	}
	if updated.PollAttempts != 3 {
		t.Fatalf("poll attempts = %d, want 3", updated.PollAttempts)
	}
	if updated.LastErrorCode != domain.CodeProviderRateLimited {
		t.Fatalf("last error = %s, want PROVIDER_RATE_LIMITED", updated.LastErrorCode)
	}
	if !updated.NextPollAt.Truncate(time.Microsecond).Equal(next.Truncate(time.Microsecond)) {
		t.Fatalf("next poll = %s, want %s", updated.NextPollAt, next)
	}
}

func TestMarkSwapStuckStopsPolling(t *testing.T) {
	store, ts := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedPollableSwap(t, store, ts, "swp_stuckmark", domain.StateProcessing, now.Add(-time.Minute))

	updated, err := store.MarkSwapStuck(ctx, "swp_stuckmark", now, domain.CodeProviderDelayed)
	if err != nil {
		t.Fatalf("mark stuck: %v", err)
	}
	if updated.StuckAt.IsZero() {
		t.Fatal("stuck_at was not set")
	}

	claimed, err := store.ClaimDueSwaps(ctx, now.Add(time.Hour), 10, 30*time.Second)
	if err != nil {
		t.Fatalf("claim after stuck: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("claimed %d swaps, want 0 after stuck", len(claimed))
	}
}

func TestUpsertAssetsSoftDisablesMissing(t *testing.T) {
	store, ts := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	assets := []domain.Asset{
		{ID: "ast_near", ProviderAssetID: "oneclick:near", Chain: "near", Symbol: "NEAR", Decimals: 24},
		{ID: "ast_usdc", ProviderAssetID: "oneclick:usdc", Chain: "ethereum", Symbol: "USDC", Decimals: 6},
	}
	count, err := store.UpsertAssets(ctx, assets, "oneclick", now)
	if err != nil {
		t.Fatalf("upsert assets: %v", err)
	}
	if count != 2 {
		t.Fatalf("upserted = %d, want 2", count)
	}

	listed, err := store.ListAssets(ctx, "")
	if err != nil {
		t.Fatalf("list assets: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed = %d, want 2", len(listed))
	}

	// The provider drops USDC on the next refresh.
	count, err = store.UpsertAssets(ctx, assets[:1], "oneclick", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if count != 1 {
		t.Fatalf("second upserted = %d, want 1", count)
	}

	listed, err = store.ListAssets(ctx, "")
	if err != nil {
		t.Fatalf("list after drop: %v", err)
	}
	if len(listed) != 1 || listed[0].ProviderAssetID != "oneclick:near" {
		t.Fatalf("listed = %+v, want only the still-returned asset", listed)
	}

	var disabled bool
	if err := ts.pool.QueryRow(ctx,
		`SELECT NOT is_enabled FROM assets WHERE provider_asset_id = 'oneclick:usdc'`,
	).Scan(&disabled); err != nil {
		t.Fatalf("read dropped asset: %v", err)
	}
	if !disabled {
		t.Fatal("dropped asset was not soft-disabled")
	}
}

func TestListAssetsFiltersByChain(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	assets := []domain.Asset{
		{ID: uuid.NewString(), ProviderAssetID: "oneclick:near", Chain: "near", Symbol: "NEAR", Decimals: 24},
		{ID: uuid.NewString(), ProviderAssetID: "oneclick:usdc", Chain: "ethereum", Symbol: "USDC", Decimals: 6},
	}
	if _, err := store.UpsertAssets(ctx, assets, "oneclick", now); err != nil {
		t.Fatalf("upsert assets: %v", err)
	}

	listed, err := store.ListAssets(ctx, "near")
	if err != nil {
		t.Fatalf("list assets: %v", err)
	}
	if len(listed) != 1 || listed[0].Chain != "near" {
		t.Fatalf("listed = %+v, want only near assets", listed)
	}
}
