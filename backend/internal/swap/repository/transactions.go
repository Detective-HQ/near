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

// RecordTransaction attaches a chain transaction to a swap. It is idempotent on
// (swap_id, tx_hash, tx_role): a duplicate report returns the existing row with
// created=false instead of failing.
//
// A deposit transaction also fills swaps.source_tx_hash when that column is
// still empty. Both writes happen in one transaction.
func (s *Store) RecordTransaction(
	ctx context.Context,
	swapID, chain, txHash string,
	role domain.TransactionRole,
) (domain.Deposit, bool, error) {
	if err := s.ready(); err != nil {
		return domain.Deposit{}, false, err
	}
	if strings.TrimSpace(swapID) == "" {
		return domain.Deposit{}, false, domain.New(domain.CodeValidationFailed, "Swap id is required.")
	}
	if strings.TrimSpace(chain) == "" {
		return domain.Deposit{}, false, domain.New(domain.CodeValidationFailed, "Chain key is required.")
	}
	if strings.TrimSpace(txHash) == "" {
		return domain.Deposit{}, false, domain.New(domain.CodeInvalidTxHash, "Transaction hash is required.")
	}
	switch role {
	case domain.TransactionRoleDeposit, domain.TransactionRoleDestination, domain.TransactionRoleRefund:
	default:
		return domain.Deposit{}, false, domain.Newf(domain.CodeValidationFailed, "Unknown transaction role %q.", role)
	}

	observedAt := time.Now().UTC()
	var deposit domain.Deposit
	created := false

	err := postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM swaps WHERE id = $1)`, swapID,
		).Scan(&exists); err != nil {
			return postgres.MapError(err)
		}
		if !exists {
			return domain.New(domain.CodeSwapNotFound, "Swap not found.")
		}

		scanErr := tx.QueryRow(ctx, `
			INSERT INTO swap_transactions (
				swap_id, chain_key, tx_hash, tx_role, observed_at
			) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (swap_id, tx_hash, tx_role) DO NOTHING
			RETURNING `+transactionColumns,
			swapID,
			chain,
			txHash,
			string(role),
			observedAt,
		).Scan(
			&deposit.ID,
			&deposit.SwapID,
			&deposit.ChainKey,
			&deposit.TxHash,
			&deposit.Role,
			&deposit.ObservedAt,
			&deposit.CreatedAt,
		)

		if scanErr == nil {
			created = true
			deposit.ObservedAt = deposit.ObservedAt.UTC()
			deposit.CreatedAt = deposit.CreatedAt.UTC()
			if role == domain.TransactionRoleDeposit {
				if _, err := tx.Exec(ctx, `
					UPDATE swaps
					SET source_tx_hash = COALESCE(NULLIF(source_tx_hash, ''), $2),
						updated_at = NOW()
					WHERE id = $1`, swapID, txHash); err != nil {
					return postgres.MapError(err)
				}
			}
			return nil
		}
		if !errors.Is(scanErr, pgx.ErrNoRows) {
			return postgres.MapError(scanErr)
		}

		existing, existingErr := scanDeposit(tx.QueryRow(ctx, `
			SELECT `+transactionColumns+`
			FROM swap_transactions
			WHERE swap_id = $1 AND tx_hash = $2 AND tx_role = $3`,
			swapID, txHash, string(role),
		))
		if existingErr != nil {
			return postgres.MapError(existingErr)
		}
		deposit = existing
		return nil
	})
	if err != nil {
		return domain.Deposit{}, false, err
	}
	return deposit, created, nil
}

// ListTransactions returns all chain transactions attached to a swap.
func (s *Store) ListTransactions(ctx context.Context, swapID string) ([]domain.Deposit, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(swapID) == "" {
		return nil, domain.New(domain.CodeValidationFailed, "Swap id is required.")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT `+transactionColumns+`
		FROM swap_transactions
		WHERE swap_id = $1
		ORDER BY observed_at ASC, id ASC`, swapID)
	if err != nil {
		return nil, postgres.MapError(err)
	}
	defer rows.Close()

	transactions := make([]domain.Deposit, 0, 4)
	for rows.Next() {
		deposit, scanErr := scanDeposit(rows)
		if scanErr != nil {
			return nil, postgres.MapError(scanErr)
		}
		transactions = append(transactions, deposit)
	}
	if err := rows.Err(); err != nil {
		return nil, postgres.MapError(err)
	}
	return transactions, nil
}
