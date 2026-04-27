-- 001_initial: Core schema
CREATE TABLE IF NOT EXISTS households (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    invite_code TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    household_id BIGINT REFERENCES households(id) ON DELETE CASCADE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL,
    avatar_color TEXT NOT NULL DEFAULT '#19323C',
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    role TEXT NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS auth_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS chores (
    id BIGSERIAL PRIMARY KEY,
    household_id BIGINT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    icon TEXT NOT NULL DEFAULT '📋',
    color TEXT NOT NULL DEFAULT '#2E86AB',
    sort_order INT NOT NULL DEFAULT 0,
    category TEXT NOT NULL DEFAULT 'custom',
    is_predefined BOOLEAN NOT NULL DEFAULT FALSE,
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(household_id, name)
);

CREATE TABLE IF NOT EXISTS chore_logs (
    id BIGSERIAL PRIMARY KEY,
    household_id BIGINT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chore_id BIGINT NOT NULL REFERENCES chores(id) ON DELETE CASCADE,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS chore_schedules (
    id BIGSERIAL PRIMARY KEY,
    household_id BIGINT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    chore_id BIGINT NOT NULL REFERENCES chores(id) ON DELETE CASCADE,
    frequency_type TEXT NOT NULL DEFAULT 'daily',
    times_of_day JSONB NOT NULL DEFAULT '["08:00"]',
    days_of_week JSONB,
    interval_days INT,
    target_count INT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    assigned_to_user_id BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS reminder_preferences (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    push_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    email_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    quiet_hours_start TEXT,
    quiet_hours_end TEXT,
    timezone TEXT NOT NULL DEFAULT 'UTC'
);

CREATE TABLE IF NOT EXISTS notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_household_id ON users(household_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_token_hash ON auth_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_chores_household_id ON chores(household_id);
CREATE INDEX IF NOT EXISTS idx_chore_logs_household_id ON chore_logs(household_id);
CREATE INDEX IF NOT EXISTS idx_chore_logs_user_id ON chore_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_chore_logs_chore_id ON chore_logs(chore_id);
CREATE INDEX IF NOT EXISTS idx_chore_logs_completed_at ON chore_logs(completed_at);
CREATE INDEX IF NOT EXISTS idx_chore_schedules_household_id ON chore_schedules(household_id);
CREATE INDEX IF NOT EXISTS idx_chore_schedules_chore_id ON chore_schedules(chore_id);
CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications(user_id);
