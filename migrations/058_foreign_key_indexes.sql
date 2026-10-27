-- Index every foreign key that had no index on its columns, and drop three
-- indexes that duplicated their UNIQUE constraint's own (improvement plan,
-- Phase 5.4). user_households serves every member list, which reads by
-- household while the primary key leads with user_id. The others serve a
-- user's or household's deletion, whose cascades and foreign key checks
-- otherwise scan these tables, and revoking a user's sessions.
--
-- These tables are small. A larger installation can prebuild the exact
-- indexes CONCURRENTLY first (deploy runbook); startup skips existing names
-- but refuses an invalid one left by an interrupted build.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
        WHERE NOT i.indisvalid AND (i.indrelid, c.relname) IN (
            ('user_households'::regclass, 'idx_user_households_household'),
            ('sessions'::regclass, 'idx_sessions_user'),
            ('auth_tokens'::regclass, 'idx_auth_tokens_user'),
            ('auth_mail_outbox'::regclass, 'idx_auth_mail_outbox_user'),
            ('schedule_reminders'::regclass, 'idx_schedule_reminders_user'),
            ('users'::regclass, 'idx_users_active_household'),
            ('chores'::regclass, 'idx_chores_created_by'),
            ('chore_schedules'::regclass, 'idx_chore_schedules_assignee'),
            ('invites'::regclass, 'idx_invites_created_by'),
            ('chore_reminder_prefs'::regclass, 'idx_chore_reminder_prefs_chore'))
    ) THEN
        RAISE EXCEPTION 'invalid foreign key index; drop and rebuild it before retrying';
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_user_households_household ON user_households (household_id);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_user ON auth_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_auth_mail_outbox_user ON auth_mail_outbox (user_id);
CREATE INDEX IF NOT EXISTS idx_schedule_reminders_user ON schedule_reminders (user_id);
CREATE INDEX IF NOT EXISTS idx_users_active_household ON users (active_household_id);
CREATE INDEX IF NOT EXISTS idx_chores_created_by ON chores (created_by);
CREATE INDEX IF NOT EXISTS idx_chore_schedules_assignee ON chore_schedules (assigned_to_user_id);
CREATE INDEX IF NOT EXISTS idx_invites_created_by ON invites (created_by);
CREATE INDEX IF NOT EXISTS idx_chore_reminder_prefs_chore ON chore_reminder_prefs (chore_id);

-- Each duplicated the index behind a UNIQUE constraint on the same column.
DROP INDEX IF EXISTS idx_invites_code;
DROP INDEX IF EXISTS idx_sessions_token_hash;
DROP INDEX IF EXISTS idx_auth_tokens_token_hash;
