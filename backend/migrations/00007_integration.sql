-- +goose Up
-- Simulation swaps share the swaps table with provider swaps so the activity
-- feed, ownership checks and audit trail stay uniform. They must never be
-- polled against a real provider and can never be a real settlement.
ALTER TABLE swaps ADD COLUMN simulated BOOLEAN NOT NULL DEFAULT FALSE;

-- The addresses validated at dry-quote time. An executable swap must use the
-- same recipient and refund address the user reviewed.
ALTER TABLE quotes ADD COLUMN recipient_address VARCHAR(255);
ALTER TABLE quotes ADD COLUMN refund_address VARCHAR(255);

DROP INDEX IF EXISTS idx_swaps_pollable;
CREATE INDEX idx_swaps_pollable
    ON swaps (next_poll_at)
    WHERE stuck_at IS NULL AND NOT simulated;

-- One on-chain deposit can fund exactly one swap. Without this a single
-- transaction hash could be reported against several swaps.
CREATE UNIQUE INDEX swap_transactions_deposit_once
    ON swap_transactions (chain_key, tx_hash)
    WHERE tx_role = 'deposit';

-- Wallet-authenticated sessions. Only the SHA-256 of the bearer token is
-- stored, so a database read never yields a usable credential.
CREATE TABLE auth_sessions (
    token_hash CHAR(64) PRIMARY KEY,
    session_id VARCHAR(64) NOT NULL UNIQUE,
    wallet_address VARCHAR(255) NOT NULL,
    chain_key VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_auth_sessions_expires_at ON auth_sessions (expires_at);

-- Single-use sign-in nonces. Consumption is DELETE ... RETURNING, so a nonce
-- can be redeemed exactly once even across several API instances.
CREATE TABLE auth_nonces (
    nonce VARCHAR(64) PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_auth_nonces_expires_at ON auth_nonces (expires_at);

-- +goose Down
DROP TABLE IF EXISTS auth_nonces;
DROP TABLE IF EXISTS auth_sessions;
DROP INDEX IF EXISTS swap_transactions_deposit_once;
DROP INDEX IF EXISTS idx_swaps_pollable;
CREATE INDEX idx_swaps_pollable
    ON swaps (next_poll_at)
    WHERE stuck_at IS NULL;
ALTER TABLE quotes DROP COLUMN IF EXISTS refund_address;
ALTER TABLE quotes DROP COLUMN IF EXISTS recipient_address;
ALTER TABLE swaps DROP COLUMN IF EXISTS simulated;
