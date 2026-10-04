-- Platform operator credentials are separate from household roles. Only a
-- digest of each high-entropy key is persisted; the clear key is shown once.
CREATE TABLE IF NOT EXISTS operator_api_keys (
    id TEXT PRIMARY KEY,
    owner_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    scope TEXT NOT NULL CHECK (scope IN ('summary', 'full')),
    token_hash BYTEA NOT NULL UNIQUE,
    auth_version BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_operator_api_keys_owner
    ON operator_api_keys(owner_user_id, created_at DESC);

-- The operator views use creation time (when the app was used), rather than
-- the potentially backdated chore completion time.
CREATE INDEX IF NOT EXISTS idx_chore_logs_actor_created
    ON chore_logs(idempotency_actor_id, created_at DESC)
    WHERE idempotency_actor_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_chore_logs_household_created
    ON chore_logs(household_id, created_at DESC);
