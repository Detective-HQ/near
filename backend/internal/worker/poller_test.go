package worker_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/repository"
	"universal-defi-intelligence/internal/worker"
)

// ---- fakes ----

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(now time.Time) *fakeClock { return &fakeClock{now: now.UTC()} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type rescheduleCall struct {
	swapID       string
	nextPollAt   time.Time
	pollAttempts int
	lastError    domain.Code
}

type stuckCall struct {
	swapID string
	reason domain.Code
}

type fakeStore struct {
	mu          sync.Mutex
	swaps       map[string]domain.Swap
	results     []repository.PollResult
	reschedules []rescheduleCall
	stuck       []stuckCall
}

func newFakeStore(swaps ...domain.Swap) *fakeStore {
	store := &fakeStore{swaps: make(map[string]domain.Swap, len(swaps))}
	for _, swap := range swaps {
		store.swaps[swap.ID] = swap
	}
	return store
}

func (s *fakeStore) get(id string) domain.Swap {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.swaps[id]
}

func (s *fakeStore) ClaimDueSwaps(_ context.Context, now time.Time, limit int, lease time.Duration) ([]domain.Swap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Swap, 0, limit)
	for id, swap := range s.swaps {
		if len(out) >= limit {
			break
		}
		if !swap.StuckAt.IsZero() || swap.State.Terminal() || !pollable(swap.State) {
			continue
		}
		if swap.NextPollAt.After(now) {
			continue
		}
		swap.NextPollAt = now.Add(lease)
		swap.LastPolledAt = now
		s.swaps[id] = swap
		out = append(out, swap)
	}
	return out, nil
}

func (s *fakeStore) RecordPollResult(_ context.Context, result repository.PollResult) (domain.Swap, domain.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	swap, ok := s.swaps[result.SwapID]
	if !ok {
		return domain.Swap{}, domain.Event{}, domain.New(domain.CodeSwapNotFound, "")
	}
	if swap.State != result.OldState {
		return domain.Swap{}, domain.Event{}, domain.New(domain.CodeStateConflict, "")
	}
	if result.OldState != result.NewState {
		// Mirror the repository: every persisted hop must be a legal edge.
		if err := domain.CanTransition(result.OldState, result.NewState); err != nil {
			return domain.Swap{}, domain.Event{}, err
		}
	}
	swap.State = result.NewState
	swap.ProviderStatus = result.ProviderStatus
	if swap.SourceTxHash == "" {
		swap.SourceTxHash = result.SourceTxHash
	}
	swap.NextPollAt = result.NextPollAt
	swap.PollAttempts = result.PollAttempts
	swap.LastPolledAt = result.ObservedAt
	s.swaps[result.SwapID] = swap
	s.results = append(s.results, result)
	return swap, domain.Event{
		SwapID:         result.SwapID,
		OldState:       result.OldState,
		NewState:       result.NewState,
		ProviderStatus: result.ProviderStatus,
		Payload:        result.Payload,
		CreatedAt:      result.ObservedAt,
	}, nil
}

func (s *fakeStore) RescheduleSwap(_ context.Context, swapID string, nextPollAt time.Time, pollAttempts int, lastError domain.Code, lastPolledAt time.Time) (domain.Swap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	swap, ok := s.swaps[swapID]
	if !ok {
		return domain.Swap{}, domain.New(domain.CodeSwapNotFound, "")
	}
	swap.NextPollAt = nextPollAt
	swap.PollAttempts = pollAttempts
	swap.LastErrorCode = lastError
	swap.LastPolledAt = lastPolledAt
	s.swaps[swapID] = swap
	s.reschedules = append(s.reschedules, rescheduleCall{swapID: swapID, nextPollAt: nextPollAt, pollAttempts: pollAttempts, lastError: lastError})
	return swap, nil
}

func (s *fakeStore) MarkSwapStuck(_ context.Context, swapID string, at time.Time, reason domain.Code) (domain.Swap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	swap, ok := s.swaps[swapID]
	if !ok {
		return domain.Swap{}, domain.New(domain.CodeSwapNotFound, "")
	}
	swap.StuckAt = at
	swap.LastErrorCode = reason
	swap.LastPolledAt = at
	s.swaps[swapID] = swap
	s.stuck = append(s.stuck, stuckCall{swapID: swapID, reason: reason})
	return swap, nil
}

func (s *fakeStore) snapshot() (int, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.results), len(s.reschedules), len(s.stuck)
}

type fakeProvider struct {
	name      string
	available bool
	status    func(worker.StatusRequest) (worker.StatusResult, error)
	tokens    []domain.Asset
	tokenErr  error

	calls      atomic.Int32
	tokenCalls atomic.Int32
}

func (p *fakeProvider) Name() string    { return p.name }
func (p *fakeProvider) Available() bool { return p.available }

func (p *fakeProvider) GetStatus(_ context.Context, req worker.StatusRequest) (worker.StatusResult, error) {
	p.calls.Add(1)
	return p.status(req)
}

func (p *fakeProvider) GetTokens(context.Context) ([]domain.Asset, error) {
	p.tokenCalls.Add(1)
	return p.tokens, p.tokenErr
}

func pollable(state domain.State) bool {
	for _, candidate := range repository.PollableStates {
		if candidate == state {
			return true
		}
	}
	return false
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func statusProvider(status domain.ProviderStatus) *fakeProvider {
	return &fakeProvider{
		name:      "fake",
		available: true,
		status: func(worker.StatusRequest) (worker.StatusResult, error) {
			return worker.StatusResult{Status: status}, nil
		},
	}
}

func newPoller(provider worker.Provider, store worker.SwapStore, clock worker.Clock, cfg worker.PollerConfig) *worker.Poller {
	return worker.NewPoller(provider, store, worker.NewHealth(), discardLogger(), clock, cfg)
}

func dueSwap(now time.Time, state domain.State) domain.Swap {
	return domain.Swap{
		ID:               "swp_1",
		State:            state,
		OriginChain:      "near",
		DepositAddress:   "deposit.test",
		ExpiresAt:        now.Add(time.Hour),
		NextPollAt:       now.Add(-time.Minute),
		CreatedAt:        now.Add(-time.Minute),
		RecipientAddress: "0x1111111111111111111111111111111111111111",
		RefundAddress:    "0x2222222222222222222222222222222222222222",
	}
}

// ---- tests ----

func TestPollerAdvancesStateFromProvider(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore(dueSwap(now, domain.StateAwaitingUserDeposit))
	provider := statusProvider(domain.ProviderStatusKnownDepositTx)
	poller := newPoller(provider, store, newFakeClock(now), worker.PollerConfig{})

	processed, err := poller.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if got := store.get("swp_1").State; got != domain.StateDepositSubmitted {
		t.Fatalf("state = %s, want DEPOSIT_SUBMITTED", got)
	}
	if results, _, _ := store.snapshot(); results != 1 {
		t.Fatalf("recorded results = %d, want 1", results)
	}
}

func TestPollerBacksOffOnUnchangedStatus(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore(dueSwap(now, domain.StateDepositSubmitted))
	provider := statusProvider(domain.ProviderStatusKnownDepositTx)
	poller := newPoller(provider, store, newFakeClock(now), worker.PollerConfig{MinInterval: 30 * time.Second})

	if _, err := poller.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	results, reschedules, _ := store.snapshot()
	if results != 0 {
		t.Fatalf("recorded results = %d, want 0 for an unchanged status", results)
	}
	if reschedules != 1 {
		t.Fatalf("reschedules = %d, want 1", reschedules)
	}
	swap := store.get("swp_1")
	if swap.PollAttempts != 1 {
		t.Fatalf("poll attempts = %d, want 1", swap.PollAttempts)
	}
	if !swap.NextPollAt.After(now) {
		t.Fatalf("next poll %s is not after now %s", swap.NextPollAt, now)
	}
}

func TestPollerStopsStuckSwaps(t *testing.T) {
	now := time.Now().UTC()
	swap := dueSwap(now, domain.StateProcessing)
	swap.CreatedAt = now.Add(-2 * time.Hour)
	store := newFakeStore(swap)
	provider := statusProvider(domain.ProviderStatusProcessing)
	poller := newPoller(provider, store, newFakeClock(now), worker.PollerConfig{StatusMaxAge: time.Hour})

	if _, err := poller.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	if got := provider.calls.Load(); got != 0 {
		t.Fatalf("provider calls = %d, want 0 for a stuck swap", got)
	}
	if _, _, stuck := store.snapshot(); stuck != 1 {
		t.Fatalf("stuck markers = %d, want 1", stuck)
	}
	if store.get("swp_1").StuckAt.IsZero() {
		t.Fatal("stuck_at was not set")
	}
}

func TestPollerExpiresUnfundedQuote(t *testing.T) {
	now := time.Now().UTC()
	swap := dueSwap(now, domain.StateAwaitingUserDeposit)
	swap.ExpiresAt = now.Add(-time.Minute)
	store := newFakeStore(swap)
	provider := statusProvider(domain.ProviderStatusPendingDeposit)
	poller := newPoller(provider, store, newFakeClock(now), worker.PollerConfig{})

	if _, err := poller.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if got := store.get("swp_1").State; got != domain.StateExpired {
		t.Fatalf("state = %s, want EXPIRED", got)
	}
	if got := provider.calls.Load(); got != 1 {
		t.Fatalf("provider calls = %d, want 1: the provider must confirm no deposit before expiry", got)
	}
}

func TestPollerDoesNotExpireAFundedButUnreportedSwap(t *testing.T) {
	now := time.Now().UTC()
	swap := dueSwap(now, domain.StateAwaitingUserDeposit)
	swap.ExpiresAt = now.Add(-time.Minute)
	store := newFakeStore(swap)
	provider := statusProvider(domain.ProviderStatusProcessing)
	poller := newPoller(provider, store, newFakeClock(now), worker.PollerConfig{})

	if _, err := poller.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if got := store.get("swp_1").State; got != domain.StateProcessing {
		t.Fatalf("state = %s, want PROCESSING (user paid but never reported the hash)", got)
	}
}

func TestPollerWalksSkippedStatesToSettlement(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore(dueSwap(now, domain.StateAwaitingUserDeposit))
	provider := statusProvider(domain.ProviderStatusSuccess)
	poller := newPoller(provider, store, newFakeClock(now), worker.PollerConfig{})

	if _, err := poller.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	final := store.get("swp_1")
	if final.State != domain.StateSucceeded || final.ProviderStatus != domain.ProviderStatusSuccess {
		t.Fatalf("final = %s/%s, want SUCCEEDED/SUCCESS", final.State, final.ProviderStatus)
	}
	if events, _, _ := store.snapshot(); events != 3 {
		t.Fatalf("events = %d, want 3 audited hops (DEPOSIT_SUBMITTED, PROCESSING, SUCCEEDED)", events)
	}
}

func TestPollerBacksOffOnUnknownProviderStatus(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore(dueSwap(now, domain.StateDepositSubmitted))
	provider := statusProvider(domain.ProviderStatus("BOGUS"))
	poller := newPoller(provider, store, newFakeClock(now), worker.PollerConfig{MinInterval: time.Second})

	if _, err := poller.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	results, reschedules, _ := store.snapshot()
	if results != 0 {
		t.Fatalf("recorded results = %d, want 0", results)
	}
	if reschedules != 1 {
		t.Fatalf("reschedules = %d, want 1", reschedules)
	}
	if code := store.get("swp_1").LastErrorCode; code != domain.CodeUnknownProviderStatus {
		t.Fatalf("last error = %s, want UNKNOWN_PROVIDER_STATUS", code)
	}
}

func TestPollerRetriesTransientProviderFailure(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore(dueSwap(now, domain.StateDepositSubmitted))
	var calls atomic.Int32
	provider := &fakeProvider{
		name:      "fake",
		available: true,
		status: func(worker.StatusRequest) (worker.StatusResult, error) {
			if calls.Add(1) == 1 {
				return worker.StatusResult{}, domain.New(domain.CodeProviderRateLimited, "slow down").WithRetryAfter(time.Millisecond)
			}
			return worker.StatusResult{Status: domain.ProviderStatusProcessing}, nil
		},
	}
	poller := newPoller(provider, store, newFakeClock(now), worker.PollerConfig{
		Backoff: worker.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond},
	})

	if _, err := poller.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if got := store.get("swp_1").State; got != domain.StateProcessing {
		t.Fatalf("state = %s, want PROCESSING after a retry", got)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("provider calls = %d, want 2", got)
	}
}

func TestPollerRecordsRateLimitBackoffAfterRetries(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore(dueSwap(now, domain.StateProcessing))
	provider := &fakeProvider{
		name:      "fake",
		available: true,
		status: func(worker.StatusRequest) (worker.StatusResult, error) {
			return worker.StatusResult{}, domain.New(domain.CodeProviderRateLimited, "slow down")
		},
	}
	poller := newPoller(provider, store, newFakeClock(now), worker.PollerConfig{
		MinInterval: time.Second,
		Backoff:     worker.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond},
	})

	if _, err := poller.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	results, reschedules, _ := store.snapshot()
	if results != 0 || reschedules != 1 {
		t.Fatalf("results=%d reschedules=%d, want 0 and 1", results, reschedules)
	}
	if code := store.get("swp_1").LastErrorCode; code != domain.CodeProviderRateLimited {
		t.Fatalf("last error = %s, want PROVIDER_RATE_LIMITED", code)
	}
}

func TestTwoPollersClaimEachSwapOnce(t *testing.T) {
	now := time.Now().UTC()
	swaps := make([]domain.Swap, 0, 20)
	for index := 0; index < 20; index++ {
		swap := dueSwap(now, domain.StateDepositSubmitted)
		swap.ID = "swp_" + string(rune('a'+index))
		swaps = append(swaps, swap)
	}
	store := newFakeStore(swaps...)

	var calls atomic.Int32
	provider := &fakeProvider{
		name:      "fake",
		available: true,
		status: func(worker.StatusRequest) (worker.StatusResult, error) {
			calls.Add(1)
			return worker.StatusResult{Status: domain.ProviderStatusProcessing}, nil
		},
	}

	clock := newFakeClock(now)
	cfg := worker.PollerConfig{Concurrency: 20}
	first := newPoller(provider, store, clock, cfg)
	second := newPoller(provider, store, clock, cfg)

	var waitGroup sync.WaitGroup
	for _, poller := range []*worker.Poller{first, second} {
		waitGroup.Add(1)
		go func(p *worker.Poller) {
			defer waitGroup.Done()
			if _, err := p.PollOnce(context.Background()); err != nil {
				t.Errorf("PollOnce: %v", err)
			}
		}(poller)
	}
	waitGroup.Wait()

	if got := calls.Load(); got != int32(len(swaps)) {
		t.Fatalf("provider calls = %d, want %d (each swap polled exactly once)", got, len(swaps))
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for id, swap := range store.swaps {
		if swap.State != domain.StateProcessing {
			t.Fatalf("swap %s state = %s, want PROCESSING", id, swap.State)
		}
	}
}

func TestPollerRestartResumesSchedule(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore(dueSwap(now, domain.StateDepositSubmitted))
	provider := statusProvider(domain.ProviderStatusKnownDepositTx)
	clock := newFakeClock(now)
	cfg := worker.PollerConfig{MinInterval: 30 * time.Second}

	first := newPoller(provider, store, clock, cfg)
	if _, err := first.PollOnce(context.Background()); err != nil {
		t.Fatalf("first poll: %v", err)
	}
	if got := provider.calls.Load(); got != 1 {
		t.Fatalf("provider calls = %d, want 1", got)
	}

	// A restarted worker must not immediately re-poll a swap whose next_poll_at
	// is still in the future.
	restarted := newPoller(provider, store, clock, cfg)
	if _, err := restarted.PollOnce(context.Background()); err != nil {
		t.Fatalf("restart poll: %v", err)
	}
	if got := provider.calls.Load(); got != 1 {
		t.Fatalf("provider calls after restart = %d, want 1", got)
	}

	clock.Advance(31 * time.Second)
	if _, err := restarted.PollOnce(context.Background()); err != nil {
		t.Fatalf("poll after advance: %v", err)
	}
	if got := provider.calls.Load(); got != 2 {
		t.Fatalf("provider calls after due = %d, want 2", got)
	}
}

func TestPollerPausesWithoutProvider(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore(dueSwap(now, domain.StateDepositSubmitted))
	provider := &fakeProvider{name: "unconfigured", available: false}
	poller := newPoller(provider, store, newFakeClock(now), worker.PollerConfig{})

	processed, err := poller.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if processed != 0 {
		t.Fatalf("processed = %d, want 0", processed)
	}
	if got := provider.calls.Load(); got != 0 {
		t.Fatalf("provider calls = %d, want 0", got)
	}
	if results, reschedules, _ := store.snapshot(); results != 0 || reschedules != 0 {
		t.Fatalf("results=%d reschedules=%d, want 0 and 0", results, reschedules)
	}
}

func TestPollerRecordsProviderHealth(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore(dueSwap(now, domain.StateDepositSubmitted))
	provider := statusProvider(domain.ProviderStatusProcessing)
	health := worker.NewHealth()
	poller := worker.NewPoller(provider, store, health, discardLogger(), newFakeClock(now), worker.PollerConfig{})

	if _, err := poller.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	snapshot, ok := health.Snapshot("fake")
	if !ok {
		t.Fatal("provider health was not recorded")
	}
	if snapshot.Total != 1 || snapshot.Failures != 0 {
		t.Fatalf("health = %+v, want one success and no failures", snapshot)
	}
}
