-- +goose Up
CREATE TABLE assets (
    id VARCHAR(255) PRIMARY KEY,
    provider_asset_id VARCHAR(512) NOT NULL,
    chain_key VARCHAR(64) NOT NULL,
    symbol VARCHAR(40) NOT NULL,
    name VARCHAR(128) NOT NULL,
    decimals INT NOT NULL,
    contract_address VARCHAR(255),
    price_usd NUMERIC(24, 8),
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    source VARCHAR(64) NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT assets_decimals_range CHECK (decimals >= 0 AND decimals <= 255)
);

-- The provider identifies an asset by (provider_asset_id, chain_key). The
-- internal id remains the primary key, but this constraint prevents the
-- catalogue refresh from inserting the same provider asset twice.
CREATE UNIQUE INDEX assets_provider_asset_chain_key
    ON assets (provider_asset_id, chain_key);

CREATE INDEX idx_assets_chain ON assets (chain_key);
CREATE INDEX idx_assets_enabled ON assets (is_enabled) WHERE is_enabled;

-- +goose Down
DROP TABLE IF EXISTS assets;
