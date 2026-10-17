-- Personal API keys let a user script their own activity logging. A key acts
-- as its owner with the same household permissions as the web UI, limited to
-- an allowlist of routes. Only a digest is stored; the clear key is shown once.
-- Bumping users.auth_version (password change/reset) invalidates every key.
CREATE TABLE IF NOT EXISTS user_api_keys (
    id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    auth_version BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_user_api_keys_user
    ON user_api_keys(user_id, created_at DESC);
