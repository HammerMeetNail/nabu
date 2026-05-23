package log

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// nullIntToPtr converts a sql.NullInt64 to *int (nil when not valid).
func nullIntToPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// ptrToNullInt64 converts a *int to sql.NullInt64.
func ptrToNullInt64(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) CreateLog(ctx context.Context, log ChoreLog) (ChoreLog, error) {
	indJSON, _ := json.Marshal(nilToEmptyLog(log.Indicators))
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO chore_logs (household_id, user_id, chore_id, completed_at, note, indicators, slot_hour)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, created_at
	`, log.HouseholdID, log.UserID, log.ChoreID, log.CompletedAt, log.Note, string(indJSON), ptrToNullInt64(log.SlotHour)).Scan(&log.ID, &log.CreatedAt)
	return log, err
}

func (s *PostgresStore) GetLog(ctx context.Context, id int64) (ChoreLog, error) {
	var l ChoreLog
	var indJSON string
	var slotHour sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT id, household_id, user_id, chore_id, completed_at, COALESCE(note,''), COALESCE(indicators,'[]'), slot_hour, created_at FROM chore_logs WHERE id = $1`, id).Scan(&l.ID, &l.HouseholdID, &l.UserID, &l.ChoreID, &l.CompletedAt, &l.Note, &indJSON, &slotHour, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return ChoreLog{}, ErrNotFound
	}
	if err == nil {
		_ = json.Unmarshal([]byte(indJSON), &l.Indicators)
		if l.Indicators == nil {
			l.Indicators = []string{}
		}
		l.SlotHour = nullIntToPtr(slotHour)
	}
	return l, err
}

func (s *PostgresStore) UpdateLog(ctx context.Context, log ChoreLog) error {
	indJSON, _ := json.Marshal(nilToEmptyLog(log.Indicators))
	_, err := s.db.ExecContext(ctx,
		`UPDATE chore_logs SET note=$1, indicators=$2 WHERE id=$3`,
		log.Note, string(indJSON), log.ID)
	return err
}

func (s *PostgresStore) DeleteLog(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chore_logs WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) FindLog(ctx context.Context, householdID, choreID int64, date time.Time) (*ChoreLog, error) {
	var l ChoreLog
	var indJSON string
	var slotHour sql.NullInt64
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	err := s.db.QueryRowContext(ctx, `SELECT id, household_id, user_id, chore_id, completed_at, COALESCE(note,''), COALESCE(indicators,'[]'), slot_hour, created_at FROM chore_logs WHERE household_id = $1 AND chore_id = $2 AND completed_at >= $3 AND completed_at < $4 LIMIT 1`, householdID, choreID, start, end).Scan(&l.ID, &l.HouseholdID, &l.UserID, &l.ChoreID, &l.CompletedAt, &l.Note, &indJSON, &slotHour, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err == nil {
		_ = json.Unmarshal([]byte(indJSON), &l.Indicators)
		if l.Indicators == nil {
			l.Indicators = []string{}
		}
		l.SlotHour = nullIntToPtr(slotHour)
	}
	return &l, err
}

func (s *PostgresStore) ListLogs(ctx context.Context, householdID int64, date time.Time) ([]ChoreLog, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	return s.queryLogs(ctx, householdID, start, end)
}

func (s *PostgresStore) ListLogsRange(ctx context.Context, householdID int64, start, end time.Time) ([]ChoreLog, error) {
	return s.queryLogs(ctx, householdID, start, end)
}

func (s *PostgresStore) LatestPerChore(ctx context.Context, householdID int64) (map[int64]ChoreLog, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT ON (chore_id)
			id, household_id, user_id, chore_id, completed_at,
			COALESCE(note,''), COALESCE(indicators,'[]'), slot_hour, created_at
		FROM chore_logs
		WHERE household_id = $1
		ORDER BY chore_id, completed_at DESC
	`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[int64]ChoreLog{}
	for rows.Next() {
		var l ChoreLog
		var indJSON string
		var slotHour sql.NullInt64
		if err := rows.Scan(&l.ID, &l.HouseholdID, &l.UserID, &l.ChoreID, &l.CompletedAt, &l.Note, &indJSON, &slotHour, &l.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(indJSON), &l.Indicators)
		if l.Indicators == nil {
			l.Indicators = []string{}
		}
		l.SlotHour = nullIntToPtr(slotHour)
		result[l.ChoreID] = l
	}
	return result, rows.Err()
}

func (s *PostgresStore) queryLogs(ctx context.Context, householdID int64, start, end time.Time) ([]ChoreLog, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, household_id, user_id, chore_id, completed_at, COALESCE(note,''), COALESCE(indicators,'[]'), slot_hour, created_at FROM chore_logs WHERE household_id = $1 AND completed_at >= $2 AND completed_at < $3 ORDER BY completed_at`, householdID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []ChoreLog
	for rows.Next() {
		var l ChoreLog
		var indJSON string
		var slotHour sql.NullInt64
		if err := rows.Scan(&l.ID, &l.HouseholdID, &l.UserID, &l.ChoreID, &l.CompletedAt, &l.Note, &indJSON, &slotHour, &l.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(indJSON), &l.Indicators)
		if l.Indicators == nil {
			l.Indicators = []string{}
		}
		l.SlotHour = nullIntToPtr(slotHour)
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func nilToEmptyLog(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
