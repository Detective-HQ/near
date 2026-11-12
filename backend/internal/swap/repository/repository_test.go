package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/storage/postgres"
	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/repository"
)

const testPayloadHashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const testPayloadHashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func testQuote(id string) domain.Quote {
	return domain.Quote{
		ID:                      id,
		SessionID:               "session_quote",
		ProviderQuoteID:         "provider_" + id,
		ProviderName:            "test-provider",
		Mode:                    domain.QuoteModeProviderDry,
		OriginAssetID:           "near:mainnet:near",
		DestinationAssetID:      "eth:1:0xa0b8",
		AmountInAtomic:          big.NewInt(1_250_000),
		AmountInDecimal:         "1.25",
		AmountOutEstimateAtomic: big.NewInt(3_700_000),
		MinAmountOutAtomic:      big.NewInt(3_600_000),
		FeeBPS:                  12,
		SlippageBPS:             50,
		FeeData:                 map[string]any{"providerFee": "0.01", "networkFee": "0.02"},
		DepositAddress:          "deposit.test",
		DepositMemo:             "memo-1",
		ExpiresAt:               time.Now().UTC().Add(time.Hour),
		RawResponse:             []byte(`{"quote":"ok","amountOut":"3.7"}`),
	}
}

func testSwap(quoteID, id, idempotencyKey, payloadHash string) domain.Swap {
	return domain.Swap{
		ID:                     id,
		QuoteID:                quoteID,
		IdempotencyPayloadHash: payloadHash,
		SessionID:              "session_swap",
		State:                  domain.StateCreated,
		OriginChain:            "near",
		DestinationChain:       "ethereum",
		RecipientAddress:       "0x1111111111111111111111111111111111111111",
		RefundAddress:          "0x2222222222222222222222222222222222222222",
		ExpiresAt:              time.Now().UTC().Add(45 * time.Minute),
	}
}

func seedQuote(t *testing.T, store *repository.Store, id string) domain.Quote {
	t.Helper()
	quote, err := store.SaveQuote(context.Background(), testQuote(id))
	if err != nil {
		t.Fatalf("save quote %s: %v", id, err)
	}
	return quote
}

func mustCreateSwap(t *testing.T, store *repository.Store, swap domain.Swap, key string) domain.Swap {
	t.Helper()
	created, replayed, err := store.CreateSwap(context.Background(), swap, key)
	if err != nil {
		t.Fatalf("create swap %s: %v", swap.ID, err)
	}
	if replayed {
		t.Fatalf("create swap %s unexpectedly replayed", swap.ID)
	}
	return created
}

func mustCode(t *testing.T, err error, code domain.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected domain code %s, got nil error", code)
	}
	if !domain.IsCode(err, code) {
		t.Fatalf("expected domain code %s, got %s (%v)", code, domain.CodeOf(err), err)
	}
}

func countRows(t *testing.T, ts *testSchema, table string) int {
	t.Helper()
	var count int
	if err := ts.pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func TestSaveAndGetQuote(t *testing.T) {
	store, ts := newTestStore(t)
	ctx := context.Background()

	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	quote := testQuote("qte_roundtrip")
	quote.AmountInAtomic = maxUint256
	quote.AmountInDecimal = maxUint256.String()
	quote.MinAmountOutAtomic = nil

	saved, err := store.SaveQuote(ctx, quote)
	if err != nil {
		t.Fatalf("save quote: %v", err)
	}
	if saved.ID != quote.ID {
		t.Fatalf("saved id = %q, want %q", saved.ID, quote.ID)
	}
	if saved.AmountInAtomic.Cmp(maxUint256) != 0 {
		t.Fatalf("amount_in_atomic round-trip = %s, want %s", saved.AmountInAtomic, maxUint256)
	}
	if saved.AmountOutEstimateAtomic.Cmp(quote.AmountOutEstimateAtomic) != 0 {
		t.Fatalf("amount_out round-trip = %s, want %s", saved.AmountOutEstimateAtomic, quote.AmountOutEstimateAtomic)
	}
	if saved.MinAmountOutAtomic != nil {
		t.Fatalf("min_amount_out_atomic = %s, want nil", saved.MinAmountOutAtomic)
	}
	if saved.FeeData["providerFee"] != "0.01" {
		t.Fatalf("fee data = %#v", saved.FeeData)
	}
	if !json.Valid(saved.RawResponse) {
		t.Fatalf("raw response is not valid JSON: %s", saved.RawResponse)
	}
	if _, err := store.SaveQuote(ctx, quote); err == nil {
		t.Fatal("duplicate quote insert succeeded")
	} else {
		mustCode(t, err, domain.CodeConflict)
	}

	loaded, err := store.GetQuote(ctx, quote.ID)
	if err != nil {
		t.Fatalf("get quote: %v", err)
	}
	if loaded.AmountInAtomic.Cmp(maxUint256) != 0 {
		t.Fatalf("loaded amount_in = %s, want %s", loaded.AmountInAtomic, maxUint256)
	}
	if loaded.Mode != quote.Mode {
		t.Fatalf("loaded mode = %q, want %q", loaded.Mode, quote.Mode)
	}

	if _, err := store.GetQuote(ctx, "qte_missing"); err == nil {
		t.Fatal("missing quote lookup succeeded")
	} else {
		mustCode(t, err, domain.CodeQuoteNotFound)
	}

	if count := countRows(t, ts, "quotes"); count != 1 {
		t.Fatalf("quotes count = %d, want 1", count)
	}
}

func TestCreateSwapIdempotentReplay(t *testing.T) {
	store, ts := newTestStore(t)
	ctx := context.Background()
	seedQuote(t, store, "qte_idem")

	swap := testSwap("qte_idem", "swp_idem", "key-idem", testPayloadHashA)
	first := mustCreateSwap(t, store, swap, "key-idem")

	replay := testSwap("qte_idem", "swp_idem_different", "key-idem", testPayloadHashA)
	second, replayed, err := store.CreateSwap(ctx, replay, "key-idem")
	if err != nil {
		t.Fatalf("replay create: %v", err)
	}
	if !replayed {
		t.Fatal("expected replay to be reported")
	}
	if second.ID != first.ID {
		t.Fatalf("replay id = %q, want %q", second.ID, first.ID)
	}
	if count := countRows(t, ts, "swaps"); count != 1 {
		t.Fatalf("swaps count = %d, want 1", count)
	}
	if count := countRows(t, ts, "swap_events"); count != 1 {
		t.Fatalf("swap_events count = %d, want 1", count)
	}

	events, err := store.ListEvents(ctx, first.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].OldState != "" || events[0].NewState != domain.StateCreated {
		t.Fatalf("initial event = %s -> %s, want '' -> CREATED", events[0].OldState, events[0].NewState)
	}
}

func TestCreateSwapIdempotencyConflict(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	seedQuote(t, store, "qte_conflict")

	mustCreateSwap(t, store, testSwap("qte_conflict", "swp_conflict_a", "key-conflict", testPayloadHashA), "key-conflict")

	different := testSwap("qte_conflict", "swp_conflict_b", "key-conflict", testPayloadHashB)
	_, _, err := store.CreateSwap(ctx, different, "key-conflict")
	mustCode(t, err, domain.CodeIdempotencyConflict)
}

func TestCreateSwapConcurrentIdempotency(t *testing.T) {
	store, ts := newTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	seedQuote(t, store, "qte_concurrent")

	const workers = 50
	type result struct {
		swap     domain.Swap
		replayed bool
		err      error
	}
	results := make(chan result, workers)
	var waitGroup sync.WaitGroup
	for index := 0; index < workers; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			swap := testSwap("qte_concurrent", fmt.Sprintf("swp_concurrent_%02d", index), "key-concurrent", testPayloadHashA)
			created, replayed, err := store.CreateSwap(ctx, swap, "key-concurrent")
			results <- result{swap: created, replayed: replayed, err: err}
		}(index)
	}
	waitGroup.Wait()
	close(results)

	winner := ""
	createdCount := 0
	replayedCount := 0
	for outcome := range results {
		if outcome.err != nil {
			t.Fatalf("concurrent create: %v", outcome.err)
		}
		if winner == "" {
			winner = outcome.swap.ID
		} else if outcome.swap.ID != winner {
			t.Fatalf("idempotent create returned %q and %q", winner, outcome.swap.ID)
		}
		if outcome.replayed {
			replayedCount++
		} else {
			createdCount++
		}
	}

	if createdCount != 1 {
		t.Fatalf("created count = %d, want 1", createdCount)
	}
	if replayedCount != workers-1 {
		t.Fatalf("replayed count = %d, want %d", replayedCount, workers-1)
	}
	if count := countRows(t, ts, "swaps"); count != 1 {
		t.Fatalf("swaps count = %d, want 1", count)
	}
	if count := countRows(t, ts, "swap_events"); count != 1 {
		t.Fatalf("swap_events count = %d, want 1", count)
	}
}

func TestUpdateSwapStateAndEvent(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	seedQuote(t, store, "qte_state")

	swap := mustCreateSwap(t, store, testSwap("qte_state", "swp_state", "key-state", testPayloadHashA), "key-state")

	steps := []struct {
		from, to domain.State
		status   domain.ProviderStatus
	}{
		{domain.StateCreated, domain.StateQuoting, ""},
		{domain.StateQuoting, domain.StateQuoted, ""},
		{domain.StateQuoted, domain.StateAwaitingUserDeposit, domain.ProviderStatusPendingDeposit},
		{domain.StateAwaitingUserDeposit, domain.StateDepositSubmitted, domain.ProviderStatusKnownDepositTx},
		{domain.StateDepositSubmitted, domain.StateProcessing, domain.ProviderStatusProcessing},
		{domain.StateProcessing, domain.StateSucceeded, domain.ProviderStatusSuccess},
	}

	for index, step := range steps {
		updated, event, err := store.UpdateSwapState(ctx, swap.ID, step.from, step.to, step.status, map[string]any{"step": index})
		if err != nil {
			t.Fatalf("transition %s -> %s: %v", step.from, step.to, err)
		}
		if updated.State != step.to {
			t.Fatalf("state after transition = %s, want %s", updated.State, step.to)
		}
		if event.OldState != step.from || event.NewState != step.to {
			t.Fatalf("event = %s -> %s, want %s -> %s", event.OldState, event.NewState, step.from, step.to)
		}
		if event.ID == 0 {
			t.Fatal("event id was not assigned")
		}
	}

	final, err := store.GetSwapByID(ctx, swap.ID)
	if err != nil {
		t.Fatalf("get final swap: %v", err)
	}
	if final.State != domain.StateSucceeded {
		t.Fatalf("final state = %s, want SUCCEEDED", final.State)
	}
	if !final.RealSettlement {
		t.Fatal("SUCCEEDED swap did not set real_settlement")
	}

	events, err := store.ListEvents(ctx, swap.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != len(steps)+1 {
		t.Fatalf("events = %d, want %d", len(events), len(steps)+1)
	}
	if events[0].NewState != domain.StateCreated {
		t.Fatalf("first event = %s, want CREATED", events[0].NewState)
	}
	if events[len(events)-1].NewState != domain.StateSucceeded {
		t.Fatalf("last event = %s, want SUCCEEDED", events[len(events)-1].NewState)
	}
}

func TestUpdateSwapStateRequiresVerifiedSettlement(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	seedQuote(t, store, "qte_settle")
	swap := mustCreateSwap(t, store, testSwap("qte_settle", "swp_settle", "key-settle", testPayloadHashA), "key-settle")

	steps := []struct {
		from, to domain.State
		status   domain.ProviderStatus
	}{
		{domain.StateCreated, domain.StateQuoting, ""},
		{domain.StateQuoting, domain.StateQuoted, ""},
		{domain.StateQuoted, domain.StateAwaitingUserDeposit, domain.ProviderStatusPendingDeposit},
		{domain.StateAwaitingUserDeposit, domain.StateDepositSubmitted, domain.ProviderStatusKnownDepositTx},
		{domain.StateDepositSubmitted, domain.StateProcessing, domain.ProviderStatusProcessing},
	}
	for _, step := range steps {
		if _, _, err := store.UpdateSwapState(ctx, swap.ID, step.from, step.to, step.status, nil); err != nil {
			t.Fatalf("transition %s -> %s: %v", step.from, step.to, err)
		}
	}

	_, _, err := store.UpdateSwapState(ctx, swap.ID, domain.StateProcessing, domain.StateSucceeded, domain.ProviderStatusProcessing, nil)
	mustCode(t, err, domain.CodeInvalidSwapState)

	current, err := store.GetSwapByID(ctx, swap.ID)
	if err != nil {
		t.Fatalf("get swap: %v", err)
	}
	if current.State != domain.StateProcessing {
		t.Fatalf("state = %s, want PROCESSING", current.State)
	}
}

func TestUpdateSwapStateConflict(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	seedQuote(t, store, "qte_conflict_state")
	swap := mustCreateSwap(t, store, testSwap("qte_conflict_state", "swp_conflict_state", "key-conflict-state", testPayloadHashA), "key-conflict-state")

	// QUOTING -> QUOTED is a valid edge, but the row is still CREATED, so the
	// optimistic WHERE clause must match zero rows.
	_, _, err := store.UpdateSwapState(ctx, swap.ID, domain.StateQuoting, domain.StateQuoted, "", nil)
	mustCode(t, err, domain.CodeStateConflict)

	current, err := store.GetSwapByID(ctx, swap.ID)
	if err != nil {
		t.Fatalf("get swap: %v", err)
	}
	if current.State != domain.StateCreated {
		t.Fatalf("state = %s, want CREATED", current.State)
	}
	events, err := store.ListEvents(ctx, swap.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want only the initial event", len(events))
	}
}

func TestUpdateSwapStateRollsBackOnEventFailure(t *testing.T) {
	store, ts := newTestStore(t)
	ctx := context.Background()
	seedQuote(t, store, "qte_rollback")
	swap := mustCreateSwap(t, store, testSwap("qte_rollback", "swp_rollback", "key-rollback", testPayloadHashA), "key-rollback")

	if _, err := ts.pool.Exec(ctx, `
		CREATE FUNCTION reject_swap_event() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'swap event rejected by test trigger';
		END;
		$$ LANGUAGE plpgsql`); err != nil {
		t.Fatalf("create trigger function: %v", err)
	}
	if _, err := ts.pool.Exec(ctx, `
		CREATE TRIGGER reject_swap_event_trigger
		BEFORE INSERT ON swap_events
		FOR EACH ROW EXECUTE FUNCTION reject_swap_event()`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	_, _, err := store.UpdateSwapState(ctx, swap.ID, domain.StateCreated, domain.StateQuoting, "", nil)
	if err == nil {
		t.Fatal("state update succeeded despite failing event trigger")
	}

	if _, err := ts.pool.Exec(ctx, `DROP TRIGGER reject_swap_event_trigger ON swap_events`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	if _, err := ts.pool.Exec(ctx, `DROP FUNCTION reject_swap_event()`); err != nil {
		t.Fatalf("drop trigger function: %v", err)
	}

	current, err := store.GetSwapByID(ctx, swap.ID)
	if err != nil {
		t.Fatalf("get swap after rollback: %v", err)
	}
	if current.State != domain.StateCreated {
		t.Fatalf("state after rollback = %s, want CREATED", current.State)
	}
	events, err := store.ListEvents(ctx, swap.ID)
	if err != nil {
		t.Fatalf("list events after rollback: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events after rollback = %d, want 1", len(events))
	}
}

func TestRecordTransactionIdempotent(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	seedQuote(t, store, "qte_tx")
	swap := mustCreateSwap(t, store, testSwap("qte_tx", "swp_tx", "key-tx", testPayloadHashA), "key-tx")

	depositHash := "0xdeposit"
	first, created, err := store.RecordTransaction(ctx, swap.ID, "near", depositHash, domain.TransactionRoleDeposit)
	if err != nil {
		t.Fatalf("record deposit: %v", err)
	}
	if !created {
		t.Fatal("first deposit was not reported as created")
	}
	if first.ID == 0 {
		t.Fatal("deposit id was not assigned")
	}

	second, created, err := store.RecordTransaction(ctx, swap.ID, "near", depositHash, domain.TransactionRoleDeposit)
	if err != nil {
		t.Fatalf("record duplicate deposit: %v", err)
	}
	if created {
		t.Fatal("duplicate deposit was reported as created")
	}
	if second.ID != first.ID {
		t.Fatalf("duplicate deposit id = %d, want %d", second.ID, first.ID)
	}

	if _, created, err := store.RecordTransaction(ctx, swap.ID, "ethereum", depositHash, domain.TransactionRoleDestination); err != nil {
		t.Fatalf("record destination: %v", err)
	} else if !created {
		t.Fatal("destination transaction was not created")
	}

	loaded, err := store.GetSwapByID(ctx, swap.ID)
	if err != nil {
		t.Fatalf("get swap: %v", err)
	}
	if loaded.SourceTxHash != depositHash {
		t.Fatalf("source_tx_hash = %q, want %q", loaded.SourceTxHash, depositHash)
	}

	transactions, err := store.ListTransactions(ctx, swap.ID)
	if err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	if len(transactions) != 2 {
		t.Fatalf("transactions = %d, want 2", len(transactions))
	}

	if _, _, err := store.RecordTransaction(ctx, "swp_missing", "near", "0xmissing", domain.TransactionRoleDeposit); err == nil {
		t.Fatal("transaction for a missing swap succeeded")
	} else {
		mustCode(t, err, domain.CodeSwapNotFound)
	}
}

func TestListSwapsKeysetAndOwnership(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	seedQuote(t, store, "qte_list")

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	const total = 5
	for index := 0; index < total; index++ {
		swap := testSwap("qte_list", fmt.Sprintf("swp_list_%d", index), fmt.Sprintf("key-list-%d", index), testPayloadHashA)
		swap.CreatedAt = base.Add(time.Duration(index) * time.Second)
		swap.UpdatedAt = swap.CreatedAt
		swap.NextPollAt = swap.CreatedAt
		if index < 3 {
			swap.SessionID = "session_a"
		} else {
			swap.SessionID = "session_b"
		}
		mustCreateSwap(t, store, swap, fmt.Sprintf("key-list-%d", index))
	}

	var listed []domain.Swap
	cursor := ""
	for page := 0; page < 10; page++ {
		swaps, next, err := store.ListSwaps(ctx, 2, cursor)
		if err != nil {
			t.Fatalf("list swaps page %d: %v", page, err)
		}
		listed = append(listed, swaps...)
		if next == "" {
			break
		}
		cursor = next
	}
	if len(listed) != total {
		t.Fatalf("listed %d swaps, want %d", len(listed), total)
	}
	for index := 1; index < len(listed); index++ {
		previous := listed[index-1]
		current := listed[index]
		if current.CreatedAt.After(previous.CreatedAt) {
			t.Fatalf("created_at is not descending: %s then %s", previous.CreatedAt, current.CreatedAt)
		}
		if current.CreatedAt.Equal(previous.CreatedAt) && current.ID > previous.ID {
			t.Fatalf("id tie-breaker is not descending: %s then %s", previous.ID, current.ID)
		}
	}
	seen := make(map[string]struct{}, len(listed))
	for _, swap := range listed {
		if _, ok := seen[swap.ID]; ok {
			t.Fatalf("duplicate swap in pages: %s", swap.ID)
		}
		seen[swap.ID] = struct{}{}
	}

	owned, next, err := store.ListSwapsBySession(ctx, "session_a", 10, "")
	if err != nil {
		t.Fatalf("list session swaps: %v", err)
	}
	if len(owned) != 3 || next != "" {
		t.Fatalf("session_a page = %d swaps, next = %q; want 3 and empty", len(owned), next)
	}
	if _, _, err := store.ListSwapsBySession(ctx, "session_missing", 10, ""); err != nil {
		t.Fatalf("list missing session: %v", err)
	}
	if _, _, err := store.ListSwaps(ctx, 10, "not-a-cursor"); err == nil {
		t.Fatal("invalid cursor was accepted")
	} else {
		mustCode(t, err, domain.CodeInvalidRequest)
	}
	if _, _, err := store.ListSwaps(ctx, repository.MaxListLimit+1, ""); err == nil {
		t.Fatal("oversized page limit was accepted")
	} else {
		mustCode(t, err, domain.CodeValidationFailed)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	position := repository.Cursor{
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		ID:        "swp_cursor",
	}
	decoded, err := repository.DecodeCursor(repository.EncodeCursor(position))
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if !decoded.CreatedAt.Equal(position.CreatedAt) || decoded.ID != position.ID {
		t.Fatalf("cursor round-trip = %#v, want %#v", decoded, position)
	}

	for _, invalid := range []string{"", "%%%", "bm90LWEtY3Vyc29y"} {
		if _, err := repository.DecodeCursor(invalid); err == nil {
			t.Fatalf("DecodeCursor(%q) succeeded", invalid)
		}
	}
}

func TestPostgresPoolOpenAndPing(t *testing.T) {
	url := testDatabaseURL(t)
	pool, err := postgres.Open(context.Background(), config.DatabaseConfig{
		URL:            url,
		MaxConns:       4,
		MinConns:       0,
		ConnectTimeout: 5 * time.Second,
		PingTimeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	defer pool.Close()
	if pool.DB() == nil {
		t.Fatal("pool DB handle is nil")
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping pool: %v", err)
	}

	if _, err := postgres.Open(context.Background(), config.DatabaseConfig{}); err == nil {
		t.Fatal("open pool with an empty URL succeeded")
	} else {
		mustCode(t, err, domain.CodeInternal)
	}
}

func TestWithTransactionRollbackAndCommit(t *testing.T) {
	_, ts := newTestStore(t)
	ctx := context.Background()
	if _, err := ts.pool.Exec(ctx, `CREATE TABLE tx_probe (id INT PRIMARY KEY)`); err != nil {
		t.Fatalf("create tx_probe: %v", err)
	}

	sentinel := errors.New("transaction failed")
	err := postgres.WithTransaction(ctx, ts.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO tx_probe (id) VALUES (1)`); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WithTransaction error = %v, want sentinel", err)
	}

	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("panic did not propagate")
			}
		}()
		_ = postgres.WithTransaction(ctx, ts.pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO tx_probe (id) VALUES (2)`); err != nil {
				return err
			}
			panic("boom")
		})
	}()

	if err := postgres.WithTransaction(ctx, ts.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO tx_probe (id) VALUES (3)`)
		return err
	}); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}

	rows, err := ts.pool.Query(ctx, `SELECT id FROM tx_probe ORDER BY id`)
	if err != nil {
		t.Fatalf("query tx_probe: %v", err)
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan tx_probe: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tx_probe: %v", err)
	}
	sort.Ints(ids)
	if len(ids) != 1 || ids[0] != 3 {
		t.Fatalf("tx_probe ids = %v, want [3]", ids)
	}
}

func TestMigrationUpDownStatus(t *testing.T) {
	ts := newTestSchema(t, false)
	ctx := context.Background()

	version, err := ts.migrator.Version(ctx)
	if err != nil {
		t.Fatalf("initial version: %v", err)
	}
	if version != 0 {
		t.Fatalf("initial version = %d, want 0", version)
	}

	applied, err := ts.migrator.Up(ctx)
	if err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	wantVersions := []int64{1, 2, 3, 4, 5, 6}
	if len(applied) != len(wantVersions) {
		t.Fatalf("applied versions = %v, want %v", applied, wantVersions)
	}
	for index, want := range wantVersions {
		if applied[index] != want {
			t.Fatalf("applied versions = %v, want %v", applied, wantVersions)
		}
	}

	if version, err = ts.migrator.Version(ctx); err != nil {
		t.Fatalf("version after up: %v", err)
	}
	if version != 6 {
		t.Fatalf("version after up = %d, want 6", version)
	}
	if err := ts.migrator.Check(ctx); err != nil {
		t.Fatalf("check after up: %v", err)
	}
	statuses, err := ts.migrator.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(statuses) != 6 {
		t.Fatalf("status rows = %d, want 6", len(statuses))
	}
	for _, status := range statuses {
		if status.State != "applied" {
			t.Fatalf("migration %d state = %q, want applied", status.Version, status.State)
		}
	}

	rolledBack, err := ts.migrator.Down(ctx)
	if err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if rolledBack != 6 {
		t.Fatalf("rolled back version = %d, want 6", rolledBack)
	}
	if version, err = ts.migrator.Version(ctx); err != nil {
		t.Fatalf("version after down: %v", err)
	}
	if version != 5 {
		t.Fatalf("version after down = %d, want 5", version)
	}
	if columnExists(t, ts, "swaps", "stuck_at") {
		t.Fatal("swaps.stuck_at still exists after rolling back migration 6")
	}
	if !relationExists(t, ts, "research_models") {
		t.Fatal("research_models was removed by rolling back migration 6")
	}
	if !relationExists(t, ts, "quotes") {
		t.Fatal("quotes was removed by rolling back migration 6")
	}
	if err := ts.migrator.Check(ctx); err == nil {
		t.Fatal("check succeeded while migration 6 was pending")
	}

	applied, err = ts.migrator.Up(ctx)
	if err != nil {
		t.Fatalf("migrate up again: %v", err)
	}
	if len(applied) != 1 || applied[0] != 6 {
		t.Fatalf("re-applied versions = %v, want [6]", applied)
	}

	rolledBackVersions, err := ts.migrator.DownTo(ctx, 0)
	if err != nil {
		t.Fatalf("migrate down to zero: %v", err)
	}
	if len(rolledBackVersions) != 6 {
		t.Fatalf("down-to-zero versions = %v, want all six", rolledBackVersions)
	}
	for _, table := range []string{"assets", "quotes", "swaps", "swap_transactions", "swap_events", "metric_observations", "data_snapshots", "research_models"} {
		if relationExists(t, ts, table) {
			t.Fatalf("%s still exists after down-to-zero", table)
		}
	}
	if version, err = ts.migrator.Version(ctx); err != nil {
		t.Fatalf("version after down-to-zero: %v", err)
	}
	if version != 0 {
		t.Fatalf("version after down-to-zero = %d, want 0", version)
	}

	if _, err := ts.migrator.Up(ctx); err != nil {
		t.Fatalf("final migrate up: %v", err)
	}
	if err := ts.migrator.Check(ctx); err != nil {
		t.Fatalf("final check: %v", err)
	}
}

func relationExists(t *testing.T, ts *testSchema, table string) bool {
	t.Helper()
	var name string
	if err := ts.pool.QueryRow(context.Background(),
		`SELECT COALESCE(to_regclass($1)::text, '')`, table,
	).Scan(&name); err != nil {
		t.Fatalf("look up relation %s: %v", table, err)
	}
	return strings.TrimSpace(name) != ""
}

func columnExists(t *testing.T, ts *testSchema, table, column string) bool {
	t.Helper()
	var exists bool
	if err := ts.pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_name = $1 AND column_name = $2
		)`, table, column).Scan(&exists); err != nil {
		t.Fatalf("look up column %s.%s: %v", table, column, err)
	}
	return exists
}
