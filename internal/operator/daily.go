package operator

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

// DailyRollup fills daily_totals with one row per finished UTC day. It counts
// existing rows once a day, so the totals add no write per event:
//
//   - signups: accounts created that day
//   - activated: of those, the ones that logged within ten minutes of signing up
//   - logs, logging_households: logs recorded that day, and the households they came from
//   - shared_households_7d: households where two or more people logged in the
//     seven days to the end of that day
//   - cohort_d7, returned_d7 (and d30): households created seven (thirty) days
//     earlier, and how many of them logged that day
type DailyRollup struct {
	db *sql.DB
}

func NewDailyRollup(db *sql.DB) *DailyRollup { return &DailyRollup{db: db} }

const (
	dailyDelay    = 2 * time.Minute
	dailyInterval = time.Hour
	// The first rollup counts back to the first sign-up, but no further than this.
	dailyBackfillDays = 400
	// The transaction-scoped advisory lock that lets one instance roll up at a
	// time ("nabu_dty").
	dailyLockKey int64 = 0x6e6162755f647479
)

// Run rolls up two minutes after start and then every hour, until ctx ends.
func (d *DailyRollup) Run(ctx context.Context) {
	timer := time.NewTimer(dailyDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if days, err := d.RollUp(ctx); err != nil {
			if ctx.Err() == nil {
				slog.Warn("daily totals failed", "error", err)
			}
		} else if days > 0 {
			slog.Info("daily totals rolled up", "days", days)
		}
		timer.Reset(dailyInterval)
	}
}

// RollUp computes every finished day not yet stored, and the latest stored
// day again in case a log changed after it was counted. Days follow the
// database's clock. It reports how many days it wrote; an instance that finds
// another rolling up writes none.
func (d *DailyRollup) RollUp(ctx context.Context) (int, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var leader bool
	if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock($1)`, dailyLockKey).Scan(&leader); err != nil || !leader {
		return 0, err
	}
	var from, through time.Time
	if err := tx.QueryRowContext(ctx, `WITH clock AS (SELECT (now() AT TIME ZONE 'UTC')::date AS today)
 SELECT COALESCE((SELECT MAX(day) FROM daily_totals),
   GREATEST((SELECT MIN((created_at AT TIME ZONE 'UTC')::date) FROM users), today - $1::int),
   today), today - 1
 FROM clock`, dailyBackfillDays).Scan(&from, &through); err != nil {
		return 0, err
	}
	days := 0
	for day := from; !day.After(through); day = day.AddDate(0, 0, 1) {
		if err := rollUpDay(ctx, tx, day); err != nil {
			return 0, err
		}
		days++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return days, nil
}

// rollUpDay counts the UTC day that day falls on and stores its row. The
// day itself is passed as a date string, so no session time zone can shift it.
func rollUpDay(ctx context.Context, tx *sql.Tx, day time.Time) error {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	_, err := tx.ExecContext(ctx, `INSERT INTO daily_totals (day, signups, activated, logs, logging_households,
   shared_households_7d, cohort_d7, returned_d7, cohort_d30, returned_d30, computed_at)
 SELECT $3::date,
  (SELECT COUNT(*) FROM users WHERE created_at >= $1 AND created_at < $2),
  (SELECT COUNT(*) FROM users u WHERE u.created_at >= $1 AND u.created_at < $2 AND EXISTS (
     SELECT 1 FROM chore_logs l WHERE l.idempotency_actor_id = u.id
     AND l.created_at >= u.created_at AND l.created_at < u.created_at + INTERVAL '10 minutes')),
  (SELECT COUNT(*) FROM chore_logs WHERE created_at >= $1 AND created_at < $2),
  (SELECT COUNT(DISTINCT household_id) FROM chore_logs WHERE created_at >= $1 AND created_at < $2),
  (SELECT COUNT(*) FROM (SELECT household_id FROM chore_logs
     WHERE created_at >= $2 - INTERVAL '7 days' AND created_at < $2 AND idempotency_actor_id IS NOT NULL
     GROUP BY household_id HAVING COUNT(DISTINCT idempotency_actor_id) >= 2) shared),
  (SELECT COUNT(*) FROM households WHERE created_at >= $1 - INTERVAL '7 days' AND created_at < $2 - INTERVAL '7 days'),
  (SELECT COUNT(*) FROM households h WHERE h.created_at >= $1 - INTERVAL '7 days' AND h.created_at < $2 - INTERVAL '7 days'
     AND EXISTS (SELECT 1 FROM chore_logs l WHERE l.household_id = h.id AND l.created_at >= $1 AND l.created_at < $2)),
  (SELECT COUNT(*) FROM households WHERE created_at >= $1 - INTERVAL '30 days' AND created_at < $2 - INTERVAL '30 days'),
  (SELECT COUNT(*) FROM households h WHERE h.created_at >= $1 - INTERVAL '30 days' AND h.created_at < $2 - INTERVAL '30 days'
     AND EXISTS (SELECT 1 FROM chore_logs l WHERE l.household_id = h.id AND l.created_at >= $1 AND l.created_at < $2)),
  now()
 ON CONFLICT (day) DO UPDATE SET signups = EXCLUDED.signups, activated = EXCLUDED.activated, logs = EXCLUDED.logs,
  logging_households = EXCLUDED.logging_households, shared_households_7d = EXCLUDED.shared_households_7d,
  cohort_d7 = EXCLUDED.cohort_d7, returned_d7 = EXCLUDED.returned_d7,
  cohort_d30 = EXCLUDED.cohort_d30, returned_d30 = EXCLUDED.returned_d30, computed_at = EXCLUDED.computed_at`,
		start, end, start.Format("2006-01-02"))
	return err
}

// DailyTotal is one day's stored counts, as Activity returns them.
type DailyTotal struct {
	Signups               int64 `json:"signups"`
	Activated             int64 `json:"activated"`
	Logs                  int64 `json:"logs"`
	LoggingHouseholds     int64 `json:"loggingHouseholds"`
	SharedHouseholds7Days int64 `json:"sharedHouseholds7Days"`
	CohortD7              int64 `json:"cohortD7"`
	ReturnedD7            int64 `json:"returnedD7"`
	CohortD30             int64 `json:"cohortD30"`
	ReturnedD30           int64 `json:"returnedD30"`
}
