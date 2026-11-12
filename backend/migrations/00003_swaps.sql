-- +goose Up
CREATE TABLE swaps (
    id VARCHAR(64) PRIMARY KEY,
    quote_id VARCHAR(64) NOT NULL REFERENCES quotes (id) ON DELETE RESTRICT,
    idempotency_key VARCHAR(128) NOT NULL,
    -- D7: a replay must compare the canonical request payload, otherwise the
    -- same key with different parameters silently returns the wrong swap.
    idempotency_payload_hash VARCHAR(64) NOT NULL,
    -- D5: ownership survives even after the originating quote is pruned.
    session_id VARCHAR(128),
    state VARCHAR(48) NOT NULL,
    provider_status VARCHAR(64),
    real_settlement BOOLEAN NOT NULL DEFAULT FALSE,
    origin_chain VARCHAR(64) NOT NULL,
    destination_chain VARCHAR(64) NOT NULL,
    deposit_address VARCHAR(255),
    deposit_memo VARCHAR(255),
    recipient_address VARCHAR(255) NOT NULL,
    refund_address VARCHAR(255) NOT NULL,
    source_tx_hash VARCHAR(255),
    -- D6: denormalized quote deadline so expiry works without a quote join.
    expires_at TIMESTAMPTZ NOT NULL,
    -- D13: persisted polling schedule for multi-worker claiming/backoff.
    next_poll_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    poll_attempts INT NOT NULL DEFAULT 0,
    last_polled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT swaps_state_valid CHECK (
        state IN (
            'CREATED',
            'QUOTING',
            'QUOTE_UNAVAILABLE',
            'QUOTED',
            'AWAITING_USER_DEPOSIT',
            'DEPOSIT_SUBMITTED',
            'INCOMPLETE_DEPOSIT',
            'PROCESSING',
            'SUCCEEDED',
            'REFUNDED',
            'FAILED',
            'CANCELLED',
            'EXPIRED'
        )
    ),
    CONSTRAINT swaps_poll_attempts_non_negative CHECK (poll_attempts >= 0)
);

-- One swap per idempotency key. The payload hash is checked by the repository
-- inside the create transaction, so a replay with a different request body
-- returns IDEMPOTENCY_CONFLICT instead of a duplicate row.
CREATE UNIQUE INDEX swaps_idempotency_key_key ON swaps (idempotency_key);
CREATE INDEX idx_swaps_quote_id ON swaps (quote_id);
CREATE INDEX idx_swaps_state ON swaps (state);
CREATE INDEX idx_swaps_state_next_poll_at ON swaps (state, next_poll_at);
CREATE INDEX idx_swaps_state_expires_at ON swaps (state, expires_at);
CREATE INDEX idx_swaps_session_id ON swaps (session_id);
CREATE INDEX idx_swaps_created_at_id ON swaps (created_at DESC, id DESC);
CREATE INDEX idx_swaps_source_tx_hash ON swaps (source_tx_hash);

-- +goose Down
DROP TABLE IF EXISTS swaps;
