-- 002_invites: Invite table for household joins
CREATE TABLE IF NOT EXISTS invites (
    id BIGSERIAL PRIMARY KEY,
    household_id BIGINT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    code TEXT NOT NULL UNIQUE,
    created_by BIGINT NOT NULL REFERENCES users(id),
    max_uses INT NOT NULL DEFAULT 0,
    used_count INT NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_invites_code ON invites(code);
CREATE INDEX IF NOT EXISTS idx_invites_household_id ON invites(household_id);
