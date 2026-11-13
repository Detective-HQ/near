-- +goose Up
CREATE TABLE swap_transactions (
    id BIGSERIAL PRIMARY KEY,
    swap_id VARCHAR(64) NOT NULL REFERENCES swaps (id) ON DELETE CASCADE,
    chain_key VARCHAR(64) NOT NULL,
    tx_hash VARCHAR(255) NOT NULL,
    tx_role VARCHAR(32) NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT swap_transactions_role_valid CHECK (
        tx_role IN ('deposit', 'destination', 'refund')
    ),
    CONSTRAINT swap_transactions_unique_reference UNIQUE (swap_id, tx_hash, tx_role)
);

CREATE INDEX idx_swap_transactions_swap_id ON swap_transactions (swap_id);
CREATE INDEX idx_swap_transactions_tx_hash ON swap_transactions (tx_hash);

CREATE TABLE swap_events (
    id BIGSERIAL PRIMARY KEY,
    swap_id VARCHAR(64) NOT NULL REFERENCES swaps (id) ON DELETE CASCADE,
    -- D8: the synthetic CREATED event has no previous state, so old_state is
    -- nullable. The repository writes NULL for the first event.
    old_state VARCHAR(48),
    new_state VARCHAR(48) NOT NULL,
    provider_status VARCHAR(64),
    event_payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_swap_events_swap_id ON swap_events (swap_id);
CREATE INDEX idx_swap_events_swap_id_id ON swap_events (swap_id, id);
CREATE INDEX idx_swap_events_created_at ON swap_events (created_at);
CREATE INDEX idx_swap_events_new_state ON swap_events (new_state);

-- +goose Down
DROP TABLE IF EXISTS swap_events;
DROP TABLE IF EXISTS swap_transactions;
