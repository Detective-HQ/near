package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"universal-defi-intelligence/internal/storage/postgres"
	"universal-defi-intelligence/internal/swap/domain"
)

// PollableStates are the lifecycle states the worker keeps polling. Terminal
// states are excluded, and AWAITING_USER_DEPOSIT is included because a provider
// polled by deposit address is how a deposit is first observed.
var PollableStates = []domain.State{
	domain.StateAwaitingUserDeposit,
	domain.StateDepositSubmitted,
	domain.StateIncompleteDeposit,
	domain.StateProcessing,
}

// ClaimDueSwaps leases up to limit swaps that are due for polling.
//
// The lease is implemented by pushing next_poll_at forward inside the same
// statement, so a second worker (or a process restart) cannot claim the same
// swap until the lease expires. FOR UPDATE SKIP LOCKED means concurrent workers
// never block on each other, and the partial index keeps the scan cheap.
func (s *Store) ClaimDueSwaps(ctx context.Context, now time.Time, limit int, lease time.Duration) ([]domain.Swap, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if lease <= 0 {
		return nil, domain.New(domain.CodeValidationFailed, "Poll lease must be greater than zero.")
	}
	if limit <= 0 {
		limit = DefaultListLimit
	}
	if limit > MaxListLimit {
		limit = MaxListLimit
	}

	now = now.UTC()
	nextPollAt := now.Add(lease)

	rows, err := s.pool.Query(ctx, `
		UPDATE swaps
		SET last_polled_at = $1,
			next_poll_at = $2,
			updated_at = $1
		WHERE id IN (
			SELECT id
			FROM swaps
			WHERE state = ANY($3::text[])
				AND stuck_at IS NULL
				AND next_poll_at <= $1
			ORDER BY next_poll_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $4
		)
		RETURNING `+swapColumns,
		now,
		nextPollAt,
		stateStrings(PollableStates),
		limit,
	)
	if err != nil {
		return nil, postgres.MapError(err)
	}
	defer rows.Close()

	claimed := make([]domain.Swap, 0, limit)
	for rows.Next() {
		swap, scanErr := scanSwap(rows)
		if scanErr != nil {
			return nil, postgres.MapError(scanErr)
		}
		claimed = append(claimed, swap)
	}
	if err := rows.Err(); err != nil {
		return nil, postgres.MapError(err)
	}
	return claimed, nil
}

// PollResult is the persisted outcome of one provider status observation.
//
// NewState may equal OldState: a provider can report the same status repeatedly,
// which is recorded as an audit event without a lifecycle change.
type PollResult struct {
	SwapID         string
	OldState       domain.State
	NewState       domain.State
	ProviderStatus domain.ProviderStatus
	SourceTxHash   string
	Payload        map[string]any
	NextPollAt     time.Time
	PollAttempts   int
	ObservedAt     time.Time
	LastErrorCode  domain.Code
}

// RecordPollResult persists a provider observation, its audit event, and the
// next polling schedule in one transaction. The state UPDATE is optimistic
// against old_state, so a swap changed underneath the poller returns
// STATE_CONFLICT instead of writing a stale event.
func (s *Store) RecordPollResult(ctx context.Context, result PollResult) (domain.Swap, domain.Event, error) {
	if err := s.ready(); err != nil {
		return domain.Swap{}, domain.Event{}, err
	}
	if strings.TrimSpace(result.SwapID) == "" {
		return domain.Swap{}, domain.Event{}, domain.New(domain.CodeValidationFailed, "Swap id is required.")
	}
	if !result.OldState.Valid() || !result.NewState.Valid() {
		return domain.Swap{}, domain.Event{}, domain.New(domain.CodeInvalidSwapState, "Poll result carries an unknown swap state.")
	}
	if result.OldState != result.NewState {
		if err := domain.CanTransition(result.OldState, result.NewState); err != nil {
			return domain.Swap{}, domain.Event{}, err
		}
	}

	providerStatus := result.ProviderStatus.Normalize()
	if result.NewState == domain.StateSucceeded && providerStatus != domain.ProviderStatusSuccess {
		return domain.Swap{}, domain.Event{}, domain.New(
			domain.CodeInvalidSwapState,
			"SUCCEEDED requires a verified SUCCESS provider status.",
		)
	}
	if result.NewState == domain.StateRefunded && providerStatus != domain.ProviderStatusRefunded {
		return domain.Swap{}, domain.Event{}, domain.New(
			domain.CodeInvalidSwapState,
			"REFUNDED requires a REFUNDED provider status.",
		)
	}

	observedAt := result.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}
	nextPollAt := result.NextPollAt
	if nextPollAt.IsZero() {
		nextPollAt = observedAt
	} else {
		nextPollAt = nextPollAt.UTC()
	}
	pollAttempts := result.PollAttempts
	if pollAttempts < 0 {
		pollAttempts = 0
	}

	settled := result.NewState == domain.StateSucceeded && providerStatus == domain.ProviderStatusSuccess

	var updated domain.Swap
	event := domain.Event{
		SwapID:         result.SwapID,
		OldState:       result.OldState,
		NewState:       result.NewState,
		ProviderStatus: providerStatus,
		Payload:        clonePayload(result.Payload),
		CreatedAt:      observedAt,
	}

	err := postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE swaps
			SET state = $3,
				provider_status = NULLIF($4, ''),
				source_tx_hash = COALESCE(NULLIF(source_tx_hash, ''), NULLIF($5, '')),
				real_settlement = real_settlement OR $6,
				next_poll_at = $7,
				poll_attempts = $8,
				last_polled_at = $9,
				last_error_code = NULLIF($10, ''),
				updated_at = $9
			WHERE id = $1 AND state = $2
			RETURNING `+swapColumns,
			result.SwapID,
			string(result.OldState),
			string(result.NewState),
			string(providerStatus),
			result.SourceTxHash,
			settled,
			nextPollAt,
			pollAttempts,
			observedAt,
			string(result.LastErrorCode),
		)

		next, scanErr := scanSwap(row)
		if scanErr != nil {
			if !errors.Is(scanErr, pgx.ErrNoRows) {
				return postgres.MapError(scanErr)
			}
			var exists bool
			if existsErr := tx.QueryRow(ctx,
				`SELECT EXISTS (SELECT 1 FROM swaps WHERE id = $1)`, result.SwapID,
			).Scan(&exists); existsErr != nil {
				return postgres.MapError(existsErr)
			}
			if !exists {
				return domain.New(domain.CodeSwapNotFound, "Swap not found.")
			}
			return domain.Newf(
				domain.CodeStateConflict,
				"Swap %s is not in state %s; re-read the swap and retry.",
				result.SwapID,
				result.OldState,
			)
		}
		updated = next

		if err := tx.QueryRow(ctx, `
			INSERT INTO swap_events (
				swap_id, old_state, new_state, provider_status, event_payload, created_at
			) VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)
			RETURNING id`,
			event.SwapID,
			string(event.OldState),
			string(event.NewState),
			string(event.ProviderStatus),
			nullableJSON(event.Payload),
			event.CreatedAt,
		).Scan(&event.ID); err != nil {
			return postgres.MapError(err)
		}
		return nil
	})
	if err != nil {
		return domain.Swap{}, domain.Event{}, err
	}
	return updated, event, nil
}

// RescheduleSwap updates only the polling schedule for a swap whose lifecycle
// state did not change (provider error, backoff, or a repeated status). It
// leaves state, provider status, and settlement untouched.
func (s *Store) RescheduleSwap(
	ctx context.Context,
	swapID string,
	nextPollAt time.Time,
	pollAttempts int,
	lastError domain.Code,
	lastPolledAt time.Time,
) (domain.Swap, error) {
	if err := s.ready(); err != nil {
		return domain.Swap{}, err
	}
	if strings.TrimSpace(swapID) == "" {
		return domain.Swap{}, domain.New(domain.CodeValidationFailed, "Swap id is required.")
	}
	if nextPollAt.IsZero() {
		return domain.Swap{}, domain.New(domain.CodeValidationFailed, "Next poll time is required.")
	}
	if pollAttempts < 0 {
		pollAttempts = 0
	}
	if lastPolledAt.IsZero() {
		lastPolledAt = time.Now().UTC()
	}

	row := s.pool.QueryRow(ctx, `
		UPDATE swaps
		SET next_poll_at = $2,
			poll_attempts = $3,
			last_error_code = NULLIF($4, ''),
			last_polled_at = $5,
			updated_at = $5
		WHERE id = $1
		RETURNING `+swapColumns,
		swapID,
		nextPollAt.UTC(),
		pollAttempts,
		string(lastError),
		lastPolledAt.UTC(),
	)
	swap, err := scanSwap(row)
	if err != nil {
		return domain.Swap{}, notFound(err, domain.CodeSwapNotFound, "Swap not found.")
	}
	return swap, nil
}

// MarkSwapStuck stops polling a swap by stamping stuck_at. The claim query skips
// any swap that has it set, so the decision survives a worker restart. The
// caller is responsible for emitting the alert.
func (s *Store) MarkSwapStuck(ctx context.Context, swapID string, at time.Time, reason domain.Code) (domain.Swap, error) {
	if err := s.ready(); err != nil {
		return domain.Swap{}, err
	}
	if strings.TrimSpace(swapID) == "" {
		return domain.Swap{}, domain.New(domain.CodeValidationFailed, "Swap id is required.")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}

	row := s.pool.QueryRow(ctx, `
		UPDATE swaps
		SET stuck_at = COALESCE(stuck_at, $2),
			last_error_code = NULLIF($3, ''),
			last_polled_at = $2,
			updated_at = $2
		WHERE id = $1
		RETURNING `+swapColumns,
		swapID,
		at.UTC(),
		string(reason),
	)
	swap, err := scanSwap(row)
	if err != nil {
		return domain.Swap{}, notFound(err, domain.CodeSwapNotFound, "Swap not found.")
	}
	return swap, nil
}

func stateStrings(states []domain.State) []string {
	out := make([]string, len(states))
	for index, state := range states {
		out[index] = string(state)
	}
	return out
}
