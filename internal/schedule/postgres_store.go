// internal/schedule/postgres_store.go

package schedule

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HammerMeetNail/nabu/internal/readlimit"
	"github.com/jackc/pgx/v5/pgconn"
	"time"
)

// PostgresStore is the Postgres-backed implementation of Store.
type PostgresStore struct {
	db *sql.DB
}

func (s *PostgresStore) ApplyLogEffects(ctx context.Context, effects LogEffects) (bool, error) {
	for attempt := 0; ; attempt++ {
		applied, err := s.applyLogEffects(ctx, effects)
		var pgerr *pgconn.PgError
		if attempt >= 2 || ctx.Err() != nil || !errors.As(err, &pgerr) || (pgerr.Code != "40P01" && pgerr.Code != "40001") {
			return applied, err
		}
	}
}

func (s *PostgresStore) applyLogEffects(ctx context.Context, effects LogEffects) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	// Acquire the chore before the effect ledger's log FK and follow-up rows,
	// matching account/member cleanup and avoiding cross-domain lock cycles.
	var choreID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM chores WHERE id=$1 AND household_id=$2 FOR NO KEY UPDATE`, effects.ChoreID, effects.HouseholdID).Scan(&choreID); err != nil {
		return false, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO chore_log_effects (log_id)
        SELECT id FROM chore_logs WHERE id=$1 AND household_id=$2 AND chore_id=$3
        ON CONFLICT DO NOTHING RETURNING log_id`, effects.LogID, effects.HouseholdID, effects.ChoreID).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if effects.UpdateFollowUp {
		// Serialize different new logs for the same chore, as well as retries
		// of one log, to preserve the single-follow-up invariant.
		if _, err := tx.ExecContext(ctx, `UPDATE chores SET last_follow_up_minutes=$1 WHERE id=$2 AND household_id=$3`, effects.LastFollowUpMinutes, effects.ChoreID, effects.HouseholdID); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM chore_schedules WHERE household_id=$1 AND chore_id=$2 AND is_follow_up`, effects.HouseholdID, effects.ChoreID); err != nil {
			return false, err
		}
		if effects.FollowUp != nil {
			f := *effects.FollowUp
			f.HouseholdID, f.ChoreID = effects.HouseholdID, effects.ChoreID
			f.FrequencyType, f.TimePeriod = "once", PeriodAnytime
			f.IsActive, f.IsFollowUp = true, true
			if _, err := s.create(ctx, tx, f); err != nil {
				return false, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// NewPostgresStore creates a new PostgresStore.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

const scheduleColumns = `
    id, household_id, chore_id, frequency_type,
    time_period, specific_time, times_of_day, days_of_week,
    interval_days, day_of_month, month_weekday, month_of_year,
    recurrence_end_date, start_date, target_count, is_active, is_follow_up, assigned_to_user_id,
    created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *PostgresStore) scan(row rowScanner) (ChoreSchedule, error) {
	var sch ChoreSchedule
	var timesRaw, daysRaw, mwRaw []byte
	var specificTime sql.NullString
	var endDate sql.NullTime
	var startDate sql.NullTime

	err := row.Scan(
		&sch.ID, &sch.HouseholdID, &sch.ChoreID, &sch.FrequencyType,
		&sch.TimePeriod, &specificTime, &timesRaw, &daysRaw,
		&sch.IntervalDays, &sch.DayOfMonth, &mwRaw, &sch.MonthOfYear,
		&endDate, &startDate, &sch.TargetCount, &sch.IsActive, &sch.IsFollowUp, &sch.AssignedUserID,
		&sch.CreatedAt, &sch.UpdatedAt,
	)
	if err != nil {
		return sch, err
	}
	if specificTime.Valid {
		sch.SpecificTime = specificTime.String
	}
	if endDate.Valid {
		t := endDate.Time
		sch.RecurrenceEnd = &t
	}
	if startDate.Valid {
		sch.StartDate = &DateOnly{Time: startDate.Time}
	}
	if len(timesRaw) > 0 {
		_ = json.Unmarshal(timesRaw, &sch.TimesOfDay)
	}
	if len(daysRaw) > 0 {
		_ = json.Unmarshal(daysRaw, &sch.DaysOfWeek)
	}
	if len(mwRaw) > 0 {
		_ = json.Unmarshal(mwRaw, &sch.MonthWeekday)
	}
	return sch, nil
}

func (s *PostgresStore) Create(ctx context.Context, sch ChoreSchedule) (ChoreSchedule, error) {
	return s.create(ctx, s.db, sch)
}

type scheduleInserter interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *PostgresStore) create(ctx context.Context, q scheduleInserter, sch ChoreSchedule) (ChoreSchedule, error) {
	timesRaw := marshalJSONOrEmpty(sch.TimesOfDay)
	daysRaw := marshalJSONOrNull(sch.DaysOfWeek)
	mwRaw := marshalJSONOrNull(sch.MonthWeekday)
	now := time.Now().UTC()

	var startDateParam interface{}
	if sch.StartDate != nil && !sch.StartDate.IsZero() {
		y, m, d := sch.StartDate.Date()
		startDateParam = fmt.Sprintf("%04d-%02d-%02d", y, m, d)
	}

	row := q.QueryRowContext(ctx, `
		INSERT INTO chore_schedules
		    (household_id, chore_id, frequency_type,
		     time_period, specific_time, times_of_day, days_of_week,
		     interval_days, day_of_month, month_weekday, month_of_year,
		     recurrence_end_date, start_date, target_count, is_active, is_follow_up, assigned_to_user_id,
		     created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		RETURNING `+scheduleColumns,
		sch.HouseholdID, sch.ChoreID, sch.FrequencyType,
		sch.TimePeriod, nullString(sch.SpecificTime), timesRaw, daysRaw,
		sch.IntervalDays, sch.DayOfMonth, mwRaw, sch.MonthOfYear,
		sch.RecurrenceEnd, startDateParam, sch.TargetCount, sch.IsActive, sch.IsFollowUp, sch.AssignedUserID,
		now, now,
	)
	return s.scan(row)
}

func (s *PostgresStore) Get(ctx context.Context, id int64) (ChoreSchedule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+scheduleColumns+` FROM chore_schedules WHERE id=$1`, id)
	return s.scan(row)
}

func (s *PostgresStore) ListByHousehold(ctx context.Context, householdID int64) ([]ChoreSchedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+scheduleColumns+` FROM chore_schedules WHERE household_id=$1 ORDER BY id`+readlimit.SQL(ctx),
		householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChoreSchedule
	for rows.Next() {
		sch, err := s.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sch)
	}
	if err := readlimit.Check(ctx, len(out)); err != nil {
		return nil, err
	}
	return out, rows.Err()
}

func (s *PostgresStore) Update(ctx context.Context, sch ChoreSchedule) (ChoreSchedule, error) {
	daysRaw := marshalJSONOrNull(sch.DaysOfWeek)
	mwRaw := marshalJSONOrNull(sch.MonthWeekday)
	timesRaw := marshalJSONOrEmpty(sch.TimesOfDay)

	var startDateParam interface{}
	if sch.StartDate != nil && !sch.StartDate.IsZero() {
		y, m, d := sch.StartDate.Date()
		startDateParam = fmt.Sprintf("%04d-%02d-%02d", y, m, d)
	}

	row := s.db.QueryRowContext(ctx, `
		UPDATE chore_schedules SET
		    frequency_type=$1, time_period=$2, specific_time=$3,
		    times_of_day=$4, days_of_week=$5, interval_days=$6,
		    day_of_month=$7, month_weekday=$8, month_of_year=$9,
		    recurrence_end_date=$10, start_date=$11, target_count=$12, is_active=$13, is_follow_up=$14,
		    assigned_to_user_id=$15, updated_at=$16
		WHERE id=$17
		RETURNING `+scheduleColumns,
		sch.FrequencyType, sch.TimePeriod, nullString(sch.SpecificTime),
		timesRaw, daysRaw, sch.IntervalDays,
		sch.DayOfMonth, mwRaw, sch.MonthOfYear,
		sch.RecurrenceEnd, startDateParam, sch.TargetCount, sch.IsActive, sch.IsFollowUp, sch.AssignedUserID,
		time.Now().UTC(), sch.ID,
	)
	return s.scan(row)
}

func (s *PostgresStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chore_schedules WHERE id=$1`, id)
	return err
}

func (s *PostgresStore) ListActiveWithTime(ctx context.Context) ([]ChoreSchedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+scheduleColumns+`
		 FROM chore_schedules
		 WHERE is_active = TRUE
		   AND specific_time IS NOT NULL AND specific_time != ''
		 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChoreSchedule
	for rows.Next() {
		sch, err := s.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sch)
	}
	return out, rows.Err()
}

func (s *PostgresStore) DeleteFollowUpSchedulesByChore(ctx context.Context, choreID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chore_schedules WHERE chore_id=$1 AND is_follow_up=TRUE`, choreID)
	return err
}

// nullString converts an empty string to a SQL NULL.
func nullString(str string) sql.NullString {
	return sql.NullString{String: str, Valid: str != ""}
}

// marshalJSONOrEmpty marshals v to JSON, returning [] for nil/empty slices.
func marshalJSONOrEmpty(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return []byte("[]")
	}
	return b
}

// marshalJSONOrNull marshals v to JSON, returning nil (SQL NULL) for nil values.
func marshalJSONOrNull(v any) []byte {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return nil
	}
	return b
}
