-- Bind replay to the original authenticated actor and immutable request digest.
-- Legacy keys remain reserved but cannot safely return a mutable historic log.
ALTER TABLE chore_logs ADD COLUMN IF NOT EXISTS idempotency_actor_id BIGINT;
ALTER TABLE chore_logs ADD COLUMN IF NOT EXISTS idempotency_hash TEXT;

-- A log can commit before ancillary work runs. Claiming this row and applying
-- its follow-up in one transaction lets a retry finish interrupted work once.
CREATE TABLE IF NOT EXISTS chore_log_effects (
    log_id BIGINT PRIMARY KEY REFERENCES chore_logs(id) ON DELETE CASCADE,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
