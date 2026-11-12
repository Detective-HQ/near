-- +goose Up
-- Phase 3 (D13): the worker needs durable state to stop polling a swap that has
-- outlived SWAP_STATUS_MAX_AGE_MINUTES and to surface the last provider failure.
-- stuck_at is set once the poll ceiling is reached; the claim query ignores any
-- swap that has it set, so a restart cannot resume polling a stuck swap.
ALTER TABLE swaps ADD COLUMN stuck_at TIMESTAMPTZ;
ALTER TABLE swaps ADD COLUMN last_error_code VARCHAR(64);

-- A partial index keeps the stuck lookup and the claim query cheap: only
-- non-terminal, non-stuck swaps are ever scanned.
CREATE INDEX idx_swaps_pollable
    ON swaps (next_poll_at)
    WHERE stuck_at IS NULL;
CREATE INDEX idx_swaps_stuck_at ON swaps (stuck_at) WHERE stuck_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_swaps_stuck_at;
DROP INDEX IF EXISTS idx_swaps_pollable;
ALTER TABLE swaps DROP COLUMN IF EXISTS last_error_code;
ALTER TABLE swaps DROP COLUMN IF EXISTS stuck_at;
