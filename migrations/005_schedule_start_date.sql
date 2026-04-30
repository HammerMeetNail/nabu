-- migrations/005_schedule_start_date.sql
-- Add start_date for "once" (does not repeat) schedules.
-- For a "once" schedule the chore card is only shown on this specific date.

ALTER TABLE chore_schedules
  ADD COLUMN IF NOT EXISTS start_date DATE;

COMMENT ON COLUMN chore_schedules.start_date IS
  'For once frequency: the specific date on which this schedule is active (YYYY-MM-DD).';
