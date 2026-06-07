CREATE TABLE IF NOT EXISTS chore_reminder_prefs (
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chore_id       BIGINT NOT NULL REFERENCES chores(id) ON DELETE CASCADE,
    enabled        BOOLEAN NOT NULL DEFAULT false,
    lead_minutes   INTEGER NOT NULL DEFAULT 10,
    UNIQUE (user_id, chore_id)
);

CREATE TABLE IF NOT EXISTS schedule_reminders (
    schedule_id    BIGINT NOT NULL REFERENCES chore_schedules(id) ON DELETE CASCADE,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scheduled_date DATE NOT NULL,
    reminded_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (schedule_id, user_id, scheduled_date)
);

ALTER TABLE reminder_preferences
    ADD COLUMN IF NOT EXISTS default_reminder_lead_minutes INTEGER NOT NULL DEFAULT 10;
