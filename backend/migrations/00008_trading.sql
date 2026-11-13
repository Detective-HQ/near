-- +goose Up
-- Published trading signals. Rows are immutable: the outcome of each signal is
-- computed on read from price data, so a result can never be edited after
-- the fact. The id is "<asset>|<timeframe>|<direction>|<bar unix time>", which
-- makes recording idempotent across API instances and the worker.
CREATE TABLE trading_signals (
    id VARCHAR(160) PRIMARY KEY,
    generated_at TIMESTAMPTZ NOT NULL,
    bar_time TIMESTAMPTZ NOT NULL,
    asset_id VARCHAR(64) NOT NULL,
    symbol VARCHAR(32) NOT NULL,
    chain_key VARCHAR(64) NOT NULL,
    direction VARCHAR(16) NOT NULL,
    timeframe VARCHAR(8) NOT NULL,
    model_version VARCHAR(64) NOT NULL,
    score DOUBLE PRECISION NOT NULL,
    entry_price DOUBLE PRECISION NOT NULL,
    stop_price DOUBLE PRECISION NOT NULL,
    target1_price DOUBLE PRECISION NOT NULL,
    target2_price DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT trading_signals_direction_valid CHECK (direction IN ('bullish', 'bearish'))
);

CREATE INDEX idx_trading_signals_generated_at ON trading_signals (generated_at DESC, id DESC);
CREATE INDEX idx_trading_signals_asset ON trading_signals (asset_id, timeframe);

-- +goose Down
DROP TABLE IF EXISTS trading_signals;
