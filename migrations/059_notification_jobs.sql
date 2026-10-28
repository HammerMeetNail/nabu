-- Durable notification work (improvement plan, Phase 5.2). A log or a join
-- leaves a notification event; fanning it out writes each recipient's in-app
-- notification and one push job, and a push job delivers to each of its
-- recipient's devices. Any instance claims due rows with SKIP LOCKED under a
-- lease, so work survives a restart and is retried after a provider failure.
-- Every row expires: a late notification is noise. A new row is due at once,
-- whatever the database and application clocks say; a retry is due when its
-- worker says.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE TABLE IF NOT EXISTS notification_events (
    id BIGSERIAL PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('chore_logged', 'household_joined')),
    household_id BIGINT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    chore_id BIGINT REFERENCES chores(id) ON DELETE CASCADE,
    -- The member the event is about (who a log is for, or who joined). The
    -- actor did it. Neither is notified.
    subject_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    actor_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    -- A join's message is written when it happens; a log's when it fans out.
    title TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    attempts INT NOT NULL DEFAULT 0,
    next_attempt TIMESTAMPTZ NOT NULL DEFAULT '-infinity',
    lease_until TIMESTAMPTZ,
    lease_id TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_notification_events_due ON notification_events (next_attempt, id);
CREATE INDEX IF NOT EXISTS idx_notification_events_household ON notification_events (household_id);
CREATE INDEX IF NOT EXISTS idx_notification_events_chore ON notification_events (chore_id);
CREATE INDEX IF NOT EXISTS idx_notification_events_subject ON notification_events (subject_id);
CREATE INDEX IF NOT EXISTS idx_notification_events_actor ON notification_events (actor_id);

CREATE TABLE IF NOT EXISTS push_jobs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    household_id BIGINT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    chore_id BIGINT REFERENCES chores(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- A second job with the same key is dropped while the first is kept.
    dedupe_key TEXT UNIQUE,
    -- Devices already delivered to, or given up on.
    done JSONB NOT NULL DEFAULT '[]'::jsonb,
    attempts INT NOT NULL DEFAULT 0,
    -- 'infinity' once finished: the row stays until it expires, keeping its
    -- dedupe key.
    next_attempt TIMESTAMPTZ NOT NULL DEFAULT '-infinity',
    lease_until TIMESTAMPTZ,
    lease_id TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_push_jobs_due ON push_jobs (next_attempt, id);
CREATE INDEX IF NOT EXISTS idx_push_jobs_user ON push_jobs (user_id);
CREATE INDEX IF NOT EXISTS idx_push_jobs_household ON push_jobs (household_id);
CREATE INDEX IF NOT EXISTS idx_push_jobs_chore ON push_jobs (chore_id);

-- A retried fan-out finds the in-app notifications it already wrote.
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS dedupe_key TEXT;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
        WHERE NOT i.indisvalid AND i.indrelid = 'notifications'::regclass
          AND c.relname = 'idx_notifications_dedupe'
    ) THEN
        RAISE EXCEPTION 'invalid notifications dedupe index; drop and rebuild it before retrying';
    END IF;
END $$;
CREATE UNIQUE INDEX IF NOT EXISTS idx_notifications_dedupe ON notifications (dedupe_key)
    WHERE dedupe_key IS NOT NULL;
