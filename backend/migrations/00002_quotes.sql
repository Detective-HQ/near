-- +goose Up
CREATE TABLE quotes (
    id VARCHAR(64) PRIMARY KEY,
    session_id VARCHAR(128),
    provider_quote_id VARCHAR(255),
    provider_name VARCHAR(64) NOT NULL,
    mode VARCHAR(32) NOT NULL,
    origin_asset_id VARCHAR(255) NOT NULL,
    destination_asset_id VARCHAR(255) NOT NULL,
    amount_in_atomic NUMERIC(78, 0) NOT NULL,
    amount_in_decimal VARCHAR(80) NOT NULL,
    amount_out_estimate_atomic NUMERIC(78, 0) NOT NULL,
    min_amount_out_atomic NUMERIC(78, 0),
    fee_bps INT NOT NULL DEFAULT 0,
    slippage_bps INT NOT NULL DEFAULT 50,
    fee_data JSONB,
    deposit_address VARCHAR(255),
    deposit_memo VARCHAR(255),
    expires_at TIMESTAMPTZ NOT NULL,
    raw_response_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT quotes_mode_valid CHECK (
        mode IN ('simulation', 'provider_dry_quote', 'provider_executable_quote')
    ),
    CONSTRAINT quotes_amount_in_positive CHECK (amount_in_atomic >= 0),
    CONSTRAINT quotes_amount_out_non_negative CHECK (amount_out_estimate_atomic >= 0),
    CONSTRAINT quotes_min_out_non_negative CHECK (
        min_amount_out_atomic IS NULL OR min_amount_out_atomic >= 0
    ),
    CONSTRAINT quotes_fee_bps_range CHECK (fee_bps >= 0 AND fee_bps <= 10000),
    CONSTRAINT quotes_slippage_bps_range CHECK (slippage_bps >= 0 AND slippage_bps <= 10000)
);

CREATE INDEX idx_quotes_expires_at ON quotes (expires_at);
CREATE INDEX idx_quotes_session_id ON quotes (session_id);
CREATE INDEX idx_quotes_created_at ON quotes (created_at DESC, id DESC);

-- +goose Down
DROP TABLE IF EXISTS quotes;
