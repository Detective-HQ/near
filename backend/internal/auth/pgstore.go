package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore persists nonces and sessions so sign-in works across API
// instances and survives restarts.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a store on an initialized pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// Put stores a nonce and opportunistically deletes expired ones.
func (s *PostgresStore) Put(ctx context.Context, nonce string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		WITH swept AS (DELETE FROM auth_nonces WHERE expires_at < NOW())
		INSERT INTO auth_nonces (nonce, expires_at) VALUES ($1, $2)`, nonce, expiresAt.UTC())
	return err
}

// Consume atomically deletes the nonce and reports whether it was still valid.
func (s *PostgresStore) Consume(ctx context.Context, nonce string, now time.Time) (bool, error) {
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx,
		`DELETE FROM auth_nonces WHERE nonce = $1 RETURNING expires_at`, nonce).Scan(&expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return now.Before(expiresAt), nil
}

// Save stores a session keyed by its token hash.
func (s *PostgresStore) Save(ctx context.Context, tokenHash string, session Session) error {
	_, err := s.pool.Exec(ctx, `
		WITH swept AS (DELETE FROM auth_sessions WHERE expires_at < NOW())
		INSERT INTO auth_sessions (token_hash, session_id, wallet_address, chain_key, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		tokenHash, session.ID, session.WalletAddress, session.Chain,
		session.CreatedAt.UTC(), session.ExpiresAt.UTC())
	return err
}

// Get loads a session by token hash.
func (s *PostgresStore) Get(ctx context.Context, tokenHash string) (Session, bool, error) {
	var session Session
	err := s.pool.QueryRow(ctx, `
		SELECT session_id, wallet_address, chain_key, created_at, expires_at
		FROM auth_sessions WHERE token_hash = $1`, tokenHash).Scan(
		&session.ID, &session.WalletAddress, &session.Chain, &session.CreatedAt, &session.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	session.CreatedAt = session.CreatedAt.UTC()
	session.ExpiresAt = session.ExpiresAt.UTC()
	return session, true, nil
}

// Delete removes a session.
func (s *PostgresStore) Delete(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM auth_sessions WHERE token_hash = $1`, tokenHash)
	return err
}
