-- 028_stats_logdate_index: Expression index for stats range queries.
-- chore_logs range queries filter on COALESCE(log_date, completed_at::date),
-- which prevents the existing idx_chore_logs_household_completed from being
-- used as an index-only or index scan. This expression index matches the
-- COALESCE expression so the planner can use it for all ListLogsRange calls.
-- completed_at is timestamptz; completed_at::date is STABLE (depends on
-- session timezone), so we use AT TIME ZONE 'UTC' to make it IMMUTABLE.
CREATE INDEX IF NOT EXISTS idx_chore_logs_household_logdate ON chore_logs(household_id, COALESCE(log_date, (completed_at AT TIME ZONE 'UTC')::date));
