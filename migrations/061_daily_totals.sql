-- One row per finished UTC day of aggregate counts for the operator
-- dashboard. A row names no person and no household: the rollup
-- (operator.DailyRollup) counts existing rows once a day, and nothing is
-- written per event.
CREATE TABLE IF NOT EXISTS daily_totals (
    day DATE PRIMARY KEY,
    signups INTEGER NOT NULL,
    activated INTEGER NOT NULL,
    logs INTEGER NOT NULL,
    logging_households INTEGER NOT NULL,
    shared_households_7d INTEGER NOT NULL,
    cohort_d7 INTEGER NOT NULL,
    returned_d7 INTEGER NOT NULL,
    cohort_d30 INTEGER NOT NULL,
    returned_d30 INTEGER NOT NULL,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The rollup reads logs by the day they were recorded.
CREATE INDEX IF NOT EXISTS idx_chore_logs_created_at ON chore_logs (created_at);
