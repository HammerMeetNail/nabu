// internal/schedule/postgres_store.go

package schedule

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// PostgresStore is the Postgres-backed implementation of Store.
type PostgresStore struct {
	db *sql.DB
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
	timesRaw := marshalJSONOrEmpty(sch.TimesOfDay)
	daysRaw := marshalJSONOrNull(sch.DaysOfWeek)
	mwRaw := marshalJSONOrNull(sch.MonthWeekday)
	now := time.Now().UTC()

	var startDateParam interface{}
	if sch.StartDate != nil && !sch.StartDate.IsZero() {
		startDateParam = sch.StartDate.Time.UTC().Format("2006-01-02")
	}

	row := s.db.QueryRowContext(ctx, `
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
		`SELECT `+scheduleColumns+` FROM chore_schedules WHERE household_id=$1 ORDER BY id`,
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
	return out, rows.Err()
}

func (s *PostgresStore) Update(ctx context.Context, sch ChoreSchedule) (ChoreSchedule, error) {
	daysRaw := marshalJSONOrNull(sch.DaysOfWeek)
	mwRaw := marshalJSONOrNull(sch.MonthWeekday)
	timesRaw := marshalJSONOrEmpty(sch.TimesOfDay)

	var startDateParam interface{}
	if sch.StartDate != nil && !sch.StartDate.IsZero() {
		startDateParam = sch.StartDate.Time.UTC().Format("2006-01-02")
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
