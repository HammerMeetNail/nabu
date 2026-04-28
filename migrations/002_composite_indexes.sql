-- 002_composite_indexes: Add composite indexes for dominant range queries
CREATE INDEX IF NOT EXISTS idx_chore_logs_household_completed ON chore_logs(household_id, completed_at);
CREATE INDEX IF NOT EXISTS idx_chores_household_sort ON chores(household_id, sort_order);
