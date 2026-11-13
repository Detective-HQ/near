// Package repository implements persistence for analytics observations and snapshots.
package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"universal-defi-intelligence/internal/analytics/domain"
	"universal-defi-intelligence/internal/storage/postgres"
)

// Store handles database persistence for analytics observations and snapshots.
type Store struct {
	pool *pgxpool.Pool
}

// New creates an analytics repository.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// SaveMetricObservations performs a batch upsert into metric_observations.
func (s *Store) SaveMetricObservations(ctx context.Context, obs []domain.MetricObservation) error {
	if s == nil || s.pool == nil {
		return nil // Non-blocking if running in memory/local mode
	}
	if len(obs) == 0 {
		return nil
	}

	return postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		batch := &pgx.Batch{}
		for _, o := range obs {
			observedAt := o.ObservedAt
			if observedAt.IsZero() {
				observedAt = time.Now().UTC()
			}
			retrievedAt := o.RetrievedAt
			if retrievedAt.IsZero() {
				retrievedAt = time.Now().UTC()
			}

			batch.Queue(`
				INSERT INTO metric_observations (
					metric_key, chain_key, value, unit, source_name, source_url,
					status, methodology_version, observed_at, retrieved_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (metric_key, chain_key, source_name, observed_at)
				DO UPDATE SET
					value = EXCLUDED.value,
					status = EXCLUDED.status,
					source_url = EXCLUDED.source_url,
					retrieved_at = EXCLUDED.retrieved_at`,
				o.MetricKey,
				o.ChainKey,
				o.Value,
				o.Unit,
				o.SourceName,
				o.SourceURL,
				string(o.Status),
				o.MethodologyVersion,
				observedAt,
				retrievedAt,
			)
		}

		results := tx.SendBatch(ctx, batch)
		for range obs {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return postgres.MapError(err)
			}
		}
		return results.Close()
	})
}

// GetLatestObservations returns the most recent observation for each metric for a chain.
func (s *Store) GetLatestObservations(ctx context.Context, chain string) ([]domain.MetricObservation, error) {
	if s == nil || s.pool == nil {
		return nil, nil
	}
	chain = strings.ToLower(strings.TrimSpace(chain))

	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (metric_key)
			id, metric_key, chain_key, value, unit, source_name, COALESCE(source_url, ''),
			status, COALESCE(methodology_version, ''), observed_at, retrieved_at
		FROM metric_observations
		WHERE chain_key = $1
		ORDER BY metric_key, observed_at DESC`, chain)
	if err != nil {
		return nil, postgres.MapError(err)
	}
	defer rows.Close()

	var observations []domain.MetricObservation
	for rows.Next() {
		var o domain.MetricObservation
		var statusStr string
		err := rows.Scan(
			&o.ID,
			&o.MetricKey,
			&o.ChainKey,
			&o.Value,
			&o.Unit,
			&o.SourceName,
			&o.SourceURL,
			&statusStr,
			&o.MethodologyVersion,
			&o.ObservedAt,
			&o.RetrievedAt,
		)
		if err != nil {
			return nil, postgres.MapError(err)
		}
		o.Status = domain.Status(statusStr)
		observations = append(observations, o)
	}
	return observations, nil
}

// SaveDataSnapshot records a data_snapshots entry.
func (s *Store) SaveDataSnapshot(ctx context.Context, snap domain.DataSnapshot) error {
	if s == nil || s.pool == nil {
		return nil
	}

	createdAt := snap.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO data_snapshots (
			id, source_name, dataset_key, captured_at, checksum, payload_location, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO NOTHING`,
		snap.ID,
		snap.SourceName,
		snap.DatasetKey,
		snap.CapturedAt,
		snap.Checksum,
		snap.PayloadLocation,
		createdAt,
	)
	if err != nil {
		return postgres.MapError(err)
	}
	return nil
}

// GetDataSnapshot returns a snapshot row by ID.
func (s *Store) GetDataSnapshot(ctx context.Context, id string) (domain.DataSnapshot, error) {
	if s == nil || s.pool == nil {
		return domain.DataSnapshot{}, fmt.Errorf("database not configured")
	}

	row := s.pool.QueryRow(ctx, `
		SELECT id, source_name, dataset_key, captured_at, checksum, payload_location, created_at
		FROM data_snapshots
		WHERE id = $1`, id)

	var snap domain.DataSnapshot
	err := row.Scan(
		&snap.ID,
		&snap.SourceName,
		&snap.DatasetKey,
		&snap.CapturedAt,
		&snap.Checksum,
		&snap.PayloadLocation,
		&snap.CreatedAt,
	)
	if err != nil {
		return domain.DataSnapshot{}, postgres.MapError(err)
	}
	return snap, nil
}
