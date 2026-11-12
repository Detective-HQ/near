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

// CreateSwap inserts a swap and its initial CREATED event in one transaction.
//
// The insert uses ON CONFLICT (idempotency_key) DO NOTHING. A replay with the
// same payload hash returns the existing record with replayed=true and performs
// no second insert. A replay with a different payload hash returns
// IDEMPOTENCY_CONFLICT.
func (s *Store) CreateSwap(ctx context.Context, swap domain.Swap, idempotencyKey string) (domain.Swap, bool, error) {
	if err := s.ready(); err != nil {
		return domain.Swap{}, false, err
	}

	swap.IdempotencyKey = strings.TrimSpace(idempotencyKey)
	payloadHash, err := normalizePayloadHash(swap.IdempotencyPayloadHash)
	if err != nil {
		return domain.Swap{}, false, err
	}
	swap.IdempotencyPayloadHash = payloadHash

	now := time.Now().UTC()
	if swap.State == "" {
		swap.State = domain.StateCreated
	}
	if swap.CreatedAt.IsZero() {
		swap.CreatedAt = now
	} else {
		swap.CreatedAt = swap.CreatedAt.UTC()
	}
	if swap.UpdatedAt.IsZero() {
		swap.UpdatedAt = swap.CreatedAt
	} else {
		swap.UpdatedAt = swap.UpdatedAt.UTC()
	}
	if swap.NextPollAt.IsZero() {
		swap.NextPollAt = swap.CreatedAt
	} else {
		swap.NextPollAt = swap.NextPollAt.UTC()
	}
	if !swap.ExpiresAt.IsZero() {
		swap.ExpiresAt = swap.ExpiresAt.UTC()
	}
	if err := validateSwapForCreate(swap); err != nil {
		return domain.Swap{}, false, err
	}

	var created domain.Swap
	replayed := false

	err = postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO swaps (
				id, quote_id, idempotency_key, idempotency_payload_hash, session_id,
				state, provider_status, real_settlement, origin_chain, destination_chain,
				deposit_address, deposit_memo, recipient_address, refund_address,
				source_tx_hash, expires_at, next_poll_at, poll_attempts, last_polled_at,
				created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, NULLIF($7, ''), $8, $9, $10,
				$11, $12, $13, $14,
				NULLIF($15, ''), $16, $17, $18, $19,
				$20, $21
			)
			ON CONFLICT (idempotency_key) DO NOTHING
			RETURNING `+swapColumns, swapInsertArgs(swap)...)

		inserted, scanErr := scanSwap(row)
		if scanErr == nil {
			created = inserted
			event := domain.InitialEvent(inserted)
			_, eventErr := tx.Exec(ctx, `
				INSERT INTO swap_events (
					swap_id, old_state, new_state, provider_status, event_payload, created_at
				) VALUES ($1, NULL, $2, NULLIF($3, ''), $4, $5)`,
				event.SwapID,
				string(event.NewState),
				string(event.ProviderStatus),
				nullableJSON(event.Payload),
				event.CreatedAt,
			)
			if eventErr != nil {
				return postgres.MapError(eventErr)
			}
			return nil
		}
		if !errors.Is(scanErr, pgx.ErrNoRows) {
			return postgres.MapError(scanErr)
		}

		// The unique key already exists. Read it and compare the canonical
		// request hash so a different payload cannot silently reuse the swap.
		existing, existingErr := scanSwap(tx.QueryRow(ctx,
			`SELECT `+swapColumns+` FROM swaps WHERE idempotency_key = $1`,
			swap.IdempotencyKey,
		))
		if existingErr != nil {
			return domain.Wrap(
				domain.CodeInternal,
				"The existing swap could not be read after an idempotency conflict.",
				existingErr,
			)
		}
		if !strings.EqualFold(existing.IdempotencyPayloadHash, swap.IdempotencyPayloadHash) {
			return domain.New(
				domain.CodeIdempotencyConflict,
				"The idempotency key was already used with a different request payload.",
			)
		}
		created = existing
		replayed = true
		return nil
	})
	if err != nil {
		return domain.Swap{}, false, err
	}
	return created, replayed, nil
}

// GetSwapByID returns a swap by its internal id.
func (s *Store) GetSwapByID(ctx context.Context, id string) (domain.Swap, error) {
	if err := s.ready(); err != nil {
		return domain.Swap{}, err
	}
	if strings.TrimSpace(id) == "" {
		return domain.Swap{}, domain.New(domain.CodeValidationFailed, "Swap id is required.")
	}

	row := s.pool.QueryRow(ctx, `SELECT `+swapColumns+` FROM swaps WHERE id = $1`, id)
	swap, err := scanSwap(row)
	if err != nil {
		return domain.Swap{}, notFound(err, domain.CodeSwapNotFound, "Swap not found.")
	}
	return swap, nil
}

// UpdateSwapState atomically applies a state transition and appends its audit
// event. The UPDATE is optimistic: it matches both id and the expected old
// state. If no row matches, a STATE_CONFLICT is returned so the worker can
// re-read and retry instead of silently losing an event.
func (s *Store) UpdateSwapState(
	ctx context.Context,
	swapID string,
	oldState, newState domain.State,
	providerStatus domain.ProviderStatus,
	payload map[string]any,
) (domain.Swap, domain.Event, error) {
	if err := s.ready(); err != nil {
		return domain.Swap{}, domain.Event{}, err
	}
	if strings.TrimSpace(swapID) == "" {
		return domain.Swap{}, domain.Event{}, domain.New(domain.CodeValidationFailed, "Swap id is required.")
	}
	if err := domain.CanTransition(oldState, newState); err != nil {
		return domain.Swap{}, domain.Event{}, err
	}
	providerStatus = providerStatus.Normalize()
	if newState == domain.StateSucceeded && providerStatus != domain.ProviderStatusSuccess {
		return domain.Swap{}, domain.Event{}, domain.New(
			domain.CodeInvalidSwapState,
			"SUCCEEDED requires a verified SUCCESS provider status.",
		)
	}
	if newState == domain.StateRefunded && providerStatus != domain.ProviderStatusRefunded {
		return domain.Swap{}, domain.Event{}, domain.New(
			domain.CodeInvalidSwapState,
			"REFUNDED requires a REFUNDED provider status.",
		)
	}

	now := time.Now().UTC()
	var updated domain.Swap
	event := domain.Event{
		SwapID:         swapID,
		OldState:       oldState,
		NewState:       newState,
		ProviderStatus: providerStatus,
		Payload:        clonePayload(payload),
		CreatedAt:      now,
	}

	err := postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE swaps
			SET state = $3,
				provider_status = NULLIF($4, ''),
				real_settlement = $5,
				updated_at = $6
			WHERE id = $1 AND state = $2
			RETURNING `+swapColumns,
			swapID,
			string(oldState),
			string(newState),
			string(providerStatus),
			newState == domain.StateSucceeded,
			now,
		)

		next, scanErr := scanSwap(row)
		if scanErr != nil {
			if !errors.Is(scanErr, pgx.ErrNoRows) {
				return postgres.MapError(scanErr)
			}
			var exists bool
			if existsErr := tx.QueryRow(ctx,
				`SELECT EXISTS (SELECT 1 FROM swaps WHERE id = $1)`, swapID,
			).Scan(&exists); existsErr != nil {
				return postgres.MapError(existsErr)
			}
			if !exists {
				return domain.New(domain.CodeSwapNotFound, "Swap not found.")
			}
			return domain.Newf(
				domain.CodeStateConflict,
				"Swap %s is not in state %s; re-read the swap and retry.",
				swapID,
				oldState,
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

// ListSwaps returns one keyset page ordered by (created_at, id) DESC.
func (s *Store) ListSwaps(ctx context.Context, limit int, cursor string) ([]domain.Swap, string, error) {
	return s.listSwaps(ctx, "", limit, cursor)
}

// ListSwapsBySession returns one keyset page scoped to a session for ownership
// checks in the activity endpoint.
func (s *Store) ListSwapsBySession(ctx context.Context, sessionID string, limit int, cursor string) ([]domain.Swap, string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, "", domain.New(domain.CodeValidationFailed, "Session id is required.")
	}
	return s.listSwaps(ctx, sessionID, limit, cursor)
}

func (s *Store) listSwaps(ctx context.Context, sessionID string, limit int, cursor string) ([]domain.Swap, string, error) {
	if err := s.ready(); err != nil {
		return nil, "", err
	}
	pageSize, err := normalizeLimit(limit)
	if err != nil {
		return nil, "", err
	}

	args := []any{pageSize + 1, sessionID}
	where := `WHERE ($2 = '' OR session_id = $2)`
	if strings.TrimSpace(cursor) != "" {
		position, decodeErr := DecodeCursor(cursor)
		if decodeErr != nil {
			return nil, "", domain.Wrap(domain.CodeInvalidRequest, "Invalid pagination cursor.", decodeErr)
		}
		where += ` AND (created_at, id) < ($3::timestamptz, $4::text)`
		args = append(args, position.CreatedAt, position.ID)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT `+swapColumns+`
		FROM swaps
		`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $1`, args...)
	if err != nil {
		return nil, "", postgres.MapError(err)
	}
	defer rows.Close()

	swaps := make([]domain.Swap, 0, pageSize+1)
	for rows.Next() {
		swap, scanErr := scanSwap(rows)
		if scanErr != nil {
			return nil, "", postgres.MapError(scanErr)
		}
		swaps = append(swaps, swap)
	}
	if err := rows.Err(); err != nil {
		return nil, "", postgres.MapError(err)
	}

	nextCursor := ""
	if len(swaps) > pageSize {
		swaps = swaps[:pageSize]
		last := swaps[len(swaps)-1]
		nextCursor = EncodeCursor(Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	return swaps, nextCursor, nil
}

// ListEvents returns a swap's immutable audit trail in insertion order.
func (s *Store) ListEvents(ctx context.Context, swapID string) ([]domain.Event, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(swapID) == "" {
		return nil, domain.New(domain.CodeValidationFailed, "Swap id is required.")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT `+eventColumns+`
		FROM swap_events
		WHERE swap_id = $1
		ORDER BY id ASC`, swapID)
	if err != nil {
		return nil, postgres.MapError(err)
	}
	defer rows.Close()

	events := make([]domain.Event, 0, 8)
	for rows.Next() {
		event, scanErr := scanEvent(rows)
		if scanErr != nil {
			return nil, postgres.MapError(scanErr)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, postgres.MapError(err)
	}
	return events, nil
}

func validateSwapForCreate(swap domain.Swap) error {
	if strings.TrimSpace(swap.ID) == "" {
		return domain.New(domain.CodeValidationFailed, "Swap id is required.")
	}
	if strings.TrimSpace(swap.QuoteID) == "" {
		return domain.New(domain.CodeValidationFailed, "Swap quote id is required.")
	}
	if swap.IdempotencyKey == "" {
		return domain.New(domain.CodeIdempotencyKeyRequired, "An Idempotency-Key header is required.")
	}
	if len(swap.IdempotencyKey) > 128 {
		return domain.New(domain.CodeValidationFailed, "Idempotency key must be at most 128 characters.")
	}
	if swap.State == "" {
		return domain.New(domain.CodeValidationFailed, "Swap state is required.")
	}
	if !swap.State.Valid() {
		return domain.Newf(domain.CodeInvalidSwapState, "Unknown swap state %q.", swap.State)
	}
	if strings.TrimSpace(swap.OriginChain) == "" || strings.TrimSpace(swap.DestinationChain) == "" {
		return domain.New(domain.CodeValidationFailed, "Swap origin and destination chains are required.")
	}
	if strings.TrimSpace(swap.RecipientAddress) == "" {
		return domain.New(domain.CodeValidationFailed, "Swap recipient address is required.")
	}
	if strings.TrimSpace(swap.RefundAddress) == "" {
		return domain.New(domain.CodeValidationFailed, "Swap refund address is required.")
	}
	if swap.ExpiresAt.IsZero() {
		return domain.New(domain.CodeValidationFailed, "Swap expiry is required.")
	}
	return nil
}

func swapInsertArgs(swap domain.Swap) []any {
	return []any{
		swap.ID,
		swap.QuoteID,
		swap.IdempotencyKey,
		swap.IdempotencyPayloadHash,
		nullableString(swap.SessionID),
		string(swap.State),
		string(swap.ProviderStatus),
		swap.RealSettlement,
		swap.OriginChain,
		swap.DestinationChain,
		nullableString(swap.DepositAddress),
		nullableString(swap.DepositMemo),
		swap.RecipientAddress,
		swap.RefundAddress,
		swap.SourceTxHash,
		swap.ExpiresAt,
		swap.NextPollAt,
		swap.PollAttempts,
		nullableTime(swap.LastPolledAt),
		swap.CreatedAt,
		swap.UpdatedAt,
	}
}

func clonePayload(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
}
