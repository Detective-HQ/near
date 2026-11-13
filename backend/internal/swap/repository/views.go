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

// GetSwapByIdempotencyKey returns the swap created with the given key. It lets
// the service answer a replay before it spends a provider call.
func (s *Store) GetSwapByIdempotencyKey(ctx context.Context, key string) (domain.Swap, error) {
	if err := s.ready(); err != nil {
		return domain.Swap{}, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return domain.Swap{}, domain.New(domain.CodeIdempotencyKeyRequired, "An Idempotency-Key header is required.")
	}
	row := s.pool.QueryRow(ctx, `SELECT `+swapColumns+` FROM swaps WHERE idempotency_key = $1`, key)
	swap, err := scanSwap(row)
	if err != nil {
		return domain.Swap{}, notFound(err, domain.CodeSwapNotFound, "Swap not found.")
	}
	return swap, nil
}

// RecordDeposit attaches the user's source transaction and advances the swap
// from AWAITING_USER_DEPOSIT to DEPOSIT_SUBMITTED in a single transaction.
//
// Keeping both writes atomic means a crash can never leave a swap with a
// recorded source hash that is still waiting for a deposit. A replay of the
// same hash returns duplicate=true without writing anything.
func (s *Store) RecordDeposit(
	ctx context.Context,
	swapID, chain, txHash string,
	providerStatus domain.ProviderStatus,
	payload map[string]any,
) (domain.Swap, domain.Deposit, bool, error) {
	if err := s.ready(); err != nil {
		return domain.Swap{}, domain.Deposit{}, false, err
	}
	if strings.TrimSpace(swapID) == "" || strings.TrimSpace(chain) == "" || strings.TrimSpace(txHash) == "" {
		return domain.Swap{}, domain.Deposit{}, false, domain.New(domain.CodeValidationFailed,
			"Swap id, chain and transaction hash are required.")
	}

	var (
		swap      domain.Swap
		deposit   domain.Deposit
		duplicate bool
	)
	now := time.Now().UTC()

	err := postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := scanSwap(tx.QueryRow(ctx,
			`SELECT `+swapColumns+` FROM swaps WHERE id = $1 FOR UPDATE`, swapID))
		if err != nil {
			return notFound(err, domain.CodeSwapNotFound, "Swap not found.")
		}

		if current.SourceTxHash != "" {
			if !strings.EqualFold(current.SourceTxHash, txHash) {
				return domain.New(domain.CodeConflict,
					"A different deposit transaction hash is already attached to this swap.")
			}
			existing, scanErr := scanDeposit(tx.QueryRow(ctx, `
				SELECT `+transactionColumns+`
				FROM swap_transactions
				WHERE swap_id = $1 AND tx_hash = $2 AND tx_role = 'deposit'`,
				swapID, current.SourceTxHash))
			if scanErr != nil && !errors.Is(scanErr, pgx.ErrNoRows) {
				return postgres.MapError(scanErr)
			}
			swap, deposit, duplicate = current, existing, true
			return nil
		}

		advance := current.State == domain.StateAwaitingUserDeposit
		if !advance && current.State.Terminal() {
			return domain.Newf(domain.CodeInvalidSwapState,
				"Cannot record a deposit for a swap in terminal state %s.", current.State)
		}
		if !advance && current.State == domain.StateQuoted {
			return domain.CanTransition(current.State, domain.StateDepositSubmitted)
		}

		deposit, err = scanDeposit(tx.QueryRow(ctx, `
			INSERT INTO swap_transactions (swap_id, chain_key, tx_hash, tx_role, observed_at)
			VALUES ($1, $2, $3, 'deposit', $4)
			RETURNING `+transactionColumns,
			swapID, chain, txHash, now))
		if err != nil {
			mapped := postgres.MapError(err)
			if domain.CodeOf(mapped) == domain.CodeConflict {
				return domain.New(domain.CodeConflict,
					"This deposit transaction is already attached to another swap.")
			}
			return mapped
		}

		if !advance {
			// The poller already moved the swap forward from provider data;
			// attach the user's hash without rewinding or re-transitioning.
			swap, err = scanSwap(tx.QueryRow(ctx, `
				UPDATE swaps SET source_tx_hash = $2, updated_at = $3
				WHERE id = $1
				RETURNING `+swapColumns, swapID, txHash, now))
			if err != nil {
				return postgres.MapError(err)
			}
			return nil
		}

		swap, err = scanSwap(tx.QueryRow(ctx, `
			UPDATE swaps
			SET state = $2,
				provider_status = NULLIF($3, ''),
				source_tx_hash = $4,
				next_poll_at = $5,
				poll_attempts = 0,
				updated_at = $5
			WHERE id = $1
			RETURNING `+swapColumns,
			swapID,
			string(domain.StateDepositSubmitted),
			string(providerStatus.Normalize()),
			txHash,
			now))
		if err != nil {
			return postgres.MapError(err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO swap_events (swap_id, old_state, new_state, provider_status, event_payload, created_at)
			VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)`,
			swapID,
			string(current.State),
			string(domain.StateDepositSubmitted),
			string(providerStatus.Normalize()),
			nullableJSON(clonePayload(payload)),
			now,
		); err != nil {
			return postgres.MapError(err)
		}
		return nil
	})
	if err != nil {
		return domain.Swap{}, domain.Deposit{}, false, err
	}
	return swap, deposit, duplicate, nil
}

// GetQuotes loads several quotes in one round trip, keyed by id.
func (s *Store) GetQuotes(ctx context.Context, ids []string) (map[string]domain.Quote, error) {
	out := make(map[string]domain.Quote, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if err := s.ready(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+quoteColumns+` FROM quotes WHERE id = ANY($1::text[])`, ids)
	if err != nil {
		return nil, postgres.MapError(err)
	}
	defer rows.Close()
	for rows.Next() {
		quote, scanErr := scanQuote(rows)
		if scanErr != nil {
			return nil, postgres.MapError(scanErr)
		}
		out[quote.ID] = quote
	}
	if err := rows.Err(); err != nil {
		return nil, postgres.MapError(err)
	}
	return out, nil
}

// ListEventsBySwapIDs loads the audit trails of several swaps in one round trip.
func (s *Store) ListEventsBySwapIDs(ctx context.Context, ids []string) (map[string][]domain.Event, error) {
	out := make(map[string][]domain.Event, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if err := s.ready(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+eventColumns+`
		FROM swap_events
		WHERE swap_id = ANY($1::text[])
		ORDER BY swap_id, id ASC`, ids)
	if err != nil {
		return nil, postgres.MapError(err)
	}
	defer rows.Close()
	for rows.Next() {
		event, scanErr := scanEvent(rows)
		if scanErr != nil {
			return nil, postgres.MapError(scanErr)
		}
		out[event.SwapID] = append(out[event.SwapID], event)
	}
	if err := rows.Err(); err != nil {
		return nil, postgres.MapError(err)
	}
	return out, nil
}

// ListTransactionsBySwapIDs loads the chain transactions of several swaps in
// one round trip.
func (s *Store) ListTransactionsBySwapIDs(ctx context.Context, ids []string) (map[string][]domain.Deposit, error) {
	out := make(map[string][]domain.Deposit, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if err := s.ready(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+transactionColumns+`
		FROM swap_transactions
		WHERE swap_id = ANY($1::text[])
		ORDER BY swap_id, observed_at ASC, id ASC`, ids)
	if err != nil {
		return nil, postgres.MapError(err)
	}
	defer rows.Close()
	for rows.Next() {
		deposit, scanErr := scanDeposit(rows)
		if scanErr != nil {
			return nil, postgres.MapError(scanErr)
		}
		out[deposit.SwapID] = append(out[deposit.SwapID], deposit)
	}
	if err := rows.Err(); err != nil {
		return nil, postgres.MapError(err)
	}
	return out, nil
}
