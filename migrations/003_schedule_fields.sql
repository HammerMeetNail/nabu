-- migrations/003_schedule_fields.sql
-- Add time-of-day scheduling columns to chore_schedules.
-- The table already exists from 001_initial.sql; this only adds columns.

ALTER TABLE chore_schedules
  ADD COLUMN IF NOT EXISTS time_period       TEXT    NOT NULL DEFAULT 'anytime',
  ADD COLUMN IF NOT EXISTS specific_time     TEXT,
  ADD COLUMN IF NOT EXISTS day_of_month      INT,
  ADD COLUMN IF NOT EXISTS month_weekday     JSONB,
  ADD COLUMN IF NOT EXISTS month_of_year     INT,
  ADD COLUMN IF NOT EXISTS recurrence_end_date DATE;

-- Ensure existing rows have a valid time_period.
UPDATE chore_schedules
  SET time_period = 'anytime'
  WHERE time_period IS NULL OR time_period = '';

COMMENT ON COLUMN chore_schedules.time_period IS
  'Named period: morning | afternoon | evening | night | anytime';
COMMENT ON COLUMN chore_schedules.specific_time IS
  'Optional exact time within the period, HH:MM 24-hour (e.g. 08:30)';
COMMENT ON COLUMN chore_schedules.month_weekday IS
  'For monthly_by_weekday: {"week":3,"day":1} = 3rd Monday (day 0=Sun..6=Sat)';
COMMENT ON COLUMN chore_schedules.recurrence_end_date IS
  'Optional date after which the schedule is considered inactive';
