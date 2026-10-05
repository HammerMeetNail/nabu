-- Per-chore (household-level) quiet hours: schedule reminders for this chore
-- are suppressed for every household member whose local time falls inside the
-- window. NULL/empty means no household-level quiet hours.
ALTER TABLE chores
    ADD COLUMN IF NOT EXISTS quiet_hours_start TEXT,
    ADD COLUMN IF NOT EXISTS quiet_hours_end   TEXT;

-- Per-chore, per-user quiet hours: quiet window that applies to one member's
-- reminders about this chore (in addition to the household-level window and
-- the user's global quiet hours).
ALTER TABLE chore_reminder_prefs
    ADD COLUMN IF NOT EXISTS quiet_hours_start TEXT,
    ADD COLUMN IF NOT EXISTS quiet_hours_end   TEXT;
