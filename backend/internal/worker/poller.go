package worker

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/repository"
)

// SwapStore is the persistence subset the poller needs. *repository.Store
// implements it.
type SwapStore interface {
	ClaimDueSwaps(ctx context.Context, now time.Time, limit int, lease time.Duration) ([]domain.Swap, error)
	RecordPollResult(ctx context.Context, result repository.PollResult) (domain.Swap, domain.Event, error)
	RescheduleSwap(ctx context.Context, swapID string, nextPollAt time.Time, pollAttempts int, lastError domain.Code, lastPolledAt time.Time) (domain.Swap, error)
	MarkSwapStuck(ctx context.Context, swapID string, at time.Time, reason domain.Code) (domain.Swap, error)
}

// PollerConfig tunes the swap status poller. Zero values fall back to defaults
// in NewPoller.
type PollerConfig struct {
	// Interval is the delay between poll cycles.
	Interval time.Duration
	// Concurrency bounds how many swaps are polled in parallel per cycle.
	Concurrency int
	// StatusMaxAge is the ceiling after which a swap stops being polled.
	StatusMaxAge time.Duration
	// Lease is how long a claimed swap is hidden from other workers.
	Lease time.Duration
	// MinInterval is the shortest delay before re-polling a changed swap.
	MinInterval time.Duration
	// MaxBackoff caps the per-swap backoff between unchanged polls.
	MaxBackoff time.Duration
	// ProviderPause is how long polling pauses when no provider is configured.
	ProviderPause time.Duration
	// Backoff computes the delay between unchanged polls.
	Backoff RetryPolicy
}

func (c PollerConfig) withDefaults() PollerConfig {
	if c.Interval <= 0 {
		c.Interval = 5 * time.Second
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 10
	}
	if c.StatusMaxAge <= 0 {
		c.StatusMaxAge = time.Hour
	}
	if c.Lease <= 0 {
		// The lease must outlive one poll including its retries, otherwise a
		// second worker could claim the swap mid-poll.
		c.Lease = 2 * time.Minute
	}
	if c.MinInterval <= 0 {
		c.MinInterval = 5 * time.Second
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 5 * time.Minute
	}
	if c.ProviderPause <= 0 {
		c.ProviderPause = 5 * time.Minute
	}
	if c.Backoff.MaxAttempts <= 0 {
		c.Backoff = DefaultRetryPolicy()
	}
	return c
}

// Poller polls active swaps and advances their lifecycle from provider status.
type Poller struct {
	provider Provider
	store    SwapStore
	health   *Health
	logger   *slog.Logger
	clock    Clock
	cfg      PollerConfig
}

// NewPoller creates a poller. Nil health and logger are replaced with safe
// defaults so callers cannot accidentally panic the worker.
func NewPoller(provider Provider, store SwapStore, health *Health, logger *slog.Logger, clock Clock, cfg PollerConfig) *Poller {
	if health == nil {
		health = NewHealth()
	}
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Poller{
		provider: provider,
		store:    store,
		health:   health,
		logger:   logger,
		clock:    clock,
		cfg:      cfg.withDefaults(),
	}
}

// Run polls until ctx is cancelled. A cancelled context is reported as
// ctx.Err() so the caller can treat it as a clean shutdown.
func (p *Poller) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.cfg.Interval)
	defer ticker.Stop()

	for {
		if _, err := p.PollOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			p.logger.Error("poll cycle failed", "error", err, "provider", p.provider.Name())
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// PollOnce claims one batch of due swaps and polls each of them. It returns the
// number of swaps processed. Errors from individual swaps are logged and never
// cancel the rest of the batch.
func (p *Poller) PollOnce(ctx context.Context) (int, error) {
	if !p.provider.Available() {
		// No provider: leave swaps scheduled so a later Phase 4 deployment can
		// pick them up, but do not touch them this cycle.
		return 0, nil
	}

	now := p.clock.Now()
	claimed, err := p.store.ClaimDueSwaps(ctx, now, p.cfg.Concurrency, p.cfg.Lease)
	if err != nil {
		return 0, err
	}
	if len(claimed) == 0 {
		return 0, nil
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(p.cfg.Concurrency)

	var processed atomic.Int64
	for _, swap := range claimed {
		swap := swap
		group.Go(func() error {
			p.pollSwap(groupCtx, swap)
			processed.Add(1)
			return nil
		})
	}
	_ = group.Wait()
	return int(processed.Load()), nil
}

// pollSwap handles one claimed swap. It never returns an error: a failure is
// recorded in health, logged, and turned into a backoff so the swap is retried
// later instead of being lost.
func (p *Poller) pollSwap(ctx context.Context, swap domain.Swap) {
	now := p.clock.Now()
	if ctx.Err() != nil {
		return
	}

	// Stuck ceiling: stop polling and alert rather than guessing an outcome.
	if !swap.CreatedAt.IsZero() && now.Sub(swap.CreatedAt) > p.cfg.StatusMaxAge {
		if _, err := p.store.MarkSwapStuck(ctx, swap.ID, now, domain.CodeProviderDelayed); err != nil {
			p.logger.Error("mark swap stuck", "swap_id", swap.ID, "error", err)
			return
		}
		p.logger.Error("swap exceeded max poll age; polling stopped for manual review",
			"swap_id", swap.ID,
			"state", string(swap.State),
			"age", now.Sub(swap.CreatedAt).String(),
		)
		return
	}

	// An unfunded swap past its deadline is a candidate for EXPIRED, but the
	// provider is asked first: a user may have paid the deposit address
	// without reporting the transaction, and expiring that swap would orphan
	// real funds.
	expiryCandidate := swap.State == domain.StateAwaitingUserDeposit &&
		strings.TrimSpace(swap.SourceTxHash) == "" &&
		!swap.ExpiresAt.IsZero() && !now.Before(swap.ExpiresAt)

	// Provider status lookup (idempotent GET, safe to retry).
	request := StatusRequest{
		SwapID:         swap.ID,
		OriginChain:    swap.OriginChain,
		DepositAddress: swap.DepositAddress,
		DepositMemo:    swap.DepositMemo,
	}
	var result StatusResult
	err := p.cfg.Backoff.Do(ctx, func(callCtx context.Context) error {
		var callErr error
		result, callErr = p.provider.GetStatus(callCtx, request)
		return callErr
	})
	if err != nil {
		code := domain.CodeOf(err)
		p.health.RecordError(p.provider.Name(), code, now)
		if code == domain.CodeProviderNotConfigured {
			if expiryCandidate {
				// No provider can ever observe a deposit for this swap.
				p.applyExpiry(ctx, swap, now)
				return
			}
			p.pause(ctx, swap, code, now)
			return
		}
		p.logger.Warn("provider status poll failed",
			"swap_id", swap.ID,
			"provider", p.provider.Name(),
			"code", string(code),
			"error", err,
		)
		p.scheduleBackoff(ctx, swap, code, now)
		return
	}
	p.health.RecordSuccess(p.provider.Name(), now)

	hasSource := strings.TrimSpace(swap.SourceTxHash) != "" || strings.TrimSpace(result.SourceTxHash) != ""
	mapped, mapErr := domain.MapProviderStatus(result.Status, hasSource)
	if mapErr != nil {
		p.health.RecordError(p.provider.Name(), domain.CodeOf(mapErr), now)
		p.logger.Warn("unrecognized provider status",
			"swap_id", swap.ID,
			"provider", p.provider.Name(),
			"status", string(result.Status.Normalize()),
		)
		p.scheduleBackoff(ctx, swap, domain.CodeOf(mapErr), now)
		return
	}

	if mapped == swap.State {
		if expiryCandidate {
			// The provider confirms nothing arrived and the deadline passed.
			p.applyExpiry(ctx, swap, now)
			return
		}
		p.scheduleBackoff(ctx, swap, "", now)
		return
	}

	// The provider may have moved through states the backend never saw (a
	// fast swap can settle between two polls). Walk the legal path and write
	// one audit event per hop so the lifecycle never skips a state.
	hops, ok := domain.ProviderPath(swap.State, result.Status, hasSource)
	if !ok {
		// A stale or regressive provider response; keep polling.
		p.scheduleBackoff(ctx, swap, "", now)
		return
	}

	current := swap.State
	for index, hop := range hops {
		final := index == len(hops)-1
		payload := providerPayload(result)
		if !final {
			payload["inferred"] = true
			payload["observedProviderStatus"] = string(result.Status.Normalize())
		}
		nextPollAt := now.Add(p.nextDelay(0))
		if hop.To.Terminal() {
			nextPollAt = now
		}
		updated, _, recordErr := p.store.RecordPollResult(ctx, repository.PollResult{
			SwapID:         swap.ID,
			OldState:       current,
			NewState:       hop.To,
			ProviderStatus: hop.ProviderStatus,
			SourceTxHash:   result.SourceTxHash,
			Payload:        payload,
			NextPollAt:     nextPollAt,
			PollAttempts:   0,
			ObservedAt:     now,
		})
		if recordErr != nil {
			p.logger.Error("persist provider status",
				"swap_id", swap.ID,
				"from", string(current),
				"to", string(hop.To),
				"error", recordErr,
			)
			return
		}
		current = updated.State
	}

	p.logger.Info("swap state advanced",
		"swap_id", swap.ID,
		"provider", p.provider.Name(),
		"from", string(swap.State),
		"to", string(current),
		"hops", len(hops),
		"provider_status", string(result.Status.Normalize()),
	)
}

func (p *Poller) applyExpiry(ctx context.Context, swap domain.Swap, now time.Time) {
	if _, _, recordErr := p.store.RecordPollResult(ctx, repository.PollResult{
		SwapID:         swap.ID,
		OldState:       swap.State,
		NewState:       domain.StateExpired,
		ProviderStatus: swap.ProviderStatus,
		Payload:        map[string]any{"reason": "quote deadline passed with no deposit"},
		NextPollAt:     now,
		ObservedAt:     now,
	}); recordErr != nil {
		p.logger.Error("expire swap", "swap_id", swap.ID, "error", recordErr)
		return
	}
	p.logger.Info("swap expired", "swap_id", swap.ID)
}

// scheduleBackoff pushes a swap's next poll out with exponential backoff and
// records the last error code. It is used whenever the lifecycle state does not
// change.
func (p *Poller) scheduleBackoff(ctx context.Context, swap domain.Swap, code domain.Code, now time.Time) {
	attempts := swap.PollAttempts + 1
	nextPollAt := now.Add(p.nextDelay(attempts))
	if _, err := p.store.RescheduleSwap(ctx, swap.ID, nextPollAt, attempts, code, now); err != nil {
		p.logger.Error("reschedule swap", "swap_id", swap.ID, "error", err)
	}
}

// pause keeps a swap eligible for a future provider without counting toward the
// stuck ceiling. It is used when no provider is configured.
func (p *Poller) pause(ctx context.Context, swap domain.Swap, code domain.Code, now time.Time) {
	nextPollAt := now.Add(p.cfg.ProviderPause)
	if _, err := p.store.RescheduleSwap(ctx, swap.ID, nextPollAt, swap.PollAttempts, code, now); err != nil {
		p.logger.Error("pause swap", "swap_id", swap.ID, "error", err)
	}
}

// nextDelay returns the backoff before the next poll for the given number of
// consecutive unchanged observations.
func (p *Poller) nextDelay(attempts int) time.Duration {
	if attempts <= 0 {
		return p.cfg.MinInterval
	}
	delay := p.cfg.Backoff.Delay(attempts)
	if delay < p.cfg.MinInterval {
		delay = p.cfg.MinInterval
	}
	if delay > p.cfg.MaxBackoff {
		delay = p.cfg.MaxBackoff
	}
	return delay
}

func providerPayload(result StatusResult) map[string]any {
	payload := map[string]any{
		"providerStatus": string(result.Status.Normalize()),
	}
	if strings.TrimSpace(result.SourceTxHash) != "" {
		payload["sourceTxHash"] = result.SourceTxHash
	}
	if strings.TrimSpace(result.DestinationTxHash) != "" {
		payload["destinationTxHash"] = result.DestinationTxHash
	}
	if strings.TrimSpace(result.RefundTxHash) != "" {
		payload["refundTxHash"] = result.RefundTxHash
	}
	return payload
}
