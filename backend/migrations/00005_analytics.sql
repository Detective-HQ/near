-- +goose Up
CREATE TABLE metric_observations (
    id BIGSERIAL PRIMARY KEY,
    metric_key VARCHAR(64) NOT NULL,
    chain_key VARCHAR(64) NOT NULL,
    value NUMERIC(28, 4),
    unit VARCHAR(32) NOT NULL,
    source_name VARCHAR(64) NOT NULL,
    source_url VARCHAR(512),
    status VARCHAR(32) NOT NULL,
    methodology_version VARCHAR(32),
    observed_at TIMESTAMPTZ NOT NULL,
    retrieved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT metric_observations_status_valid CHECK (
        status IN ('available', 'stale', 'partial', 'unavailable')
    ),
    -- A source can only publish one observation for a metric/chain/timestamp.
    -- Re-ingestion is an upsert rather than a duplicate row.
    CONSTRAINT metric_observations_unique_observation
        UNIQUE (metric_key, chain_key, source_name, observed_at)
);

CREATE INDEX idx_metric_observations_key
    ON metric_observations (metric_key, chain_key);
CREATE INDEX idx_metric_observations_observed_at
    ON metric_observations (observed_at DESC);

CREATE TABLE data_snapshots (
    id VARCHAR(64) PRIMARY KEY,
    source_name VARCHAR(64) NOT NULL,
    dataset_key VARCHAR(128) NOT NULL,
    captured_at TIMESTAMPTZ NOT NULL,
    checksum VARCHAR(128) NOT NULL,
    payload_location VARCHAR(512) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_data_snapshots_source
    ON data_snapshots (source_name, dataset_key);
CREATE INDEX idx_data_snapshots_checksum ON data_snapshots (checksum);

CREATE TABLE research_models (
    id VARCHAR(64) PRIMARY KEY,
    model_name VARCHAR(64) NOT NULL,
    version VARCHAR(32) NOT NULL,
    assumptions_json JSONB NOT NULL,
    inputs_snapshot_id VARCHAR(64) REFERENCES data_snapshots (id),
    output_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT research_models_name_version_unique UNIQUE (model_name, version)
);

CREATE INDEX idx_research_models_snapshot
    ON research_models (inputs_snapshot_id);

-- +goose Down
DROP TABLE IF EXISTS research_models;
DROP TABLE IF EXISTS data_snapshots;
DROP TABLE IF EXISTS metric_observations;
