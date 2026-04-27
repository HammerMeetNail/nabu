package log

import (
	"context"
	"database/sql"
	"time"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) CreateLog(ctx context.Context, log ChoreLog) (ChoreLog, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO chore_logs (household_id, user_id, chore_id, completed_at, note)
		VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at
	`, log.HouseholdID, log.UserID, log.ChoreID, log.CompletedAt, log.Note).Scan(&log.ID, &log.CreatedAt)
	return log, err
}

func (s *PostgresStore) GetLog(ctx context.Context, id int64) (ChoreLog, error) {
	var l ChoreLog
	err := s.db.QueryRowContext(ctx, `SELECT id, household_id, user_id, chore_id, completed_at, COALESCE(note,''), created_at FROM chore_logs WHERE id = $1`, id).Scan(&l.ID, &l.HouseholdID, &l.UserID, &l.ChoreID, &l.CompletedAt, &l.Note, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return ChoreLog{}, ErrNotFound
	}
	return l, err
}

func (s *PostgresStore) DeleteLog(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chore_logs WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) FindLog(ctx context.Context, householdID, choreID int64, date time.Time) (*ChoreLog, error) {
	var l ChoreLog
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	err := s.db.QueryRowContext(ctx, `SELECT id, household_id, user_id, chore_id, completed_at, COALESCE(note,''), created_at FROM chore_logs WHERE household_id = $1 AND chore_id = $2 AND completed_at >= $3 AND completed_at < $4 LIMIT 1`, householdID, choreID, start, end).Scan(&l.ID, &l.HouseholdID, &l.UserID, &l.ChoreID, &l.CompletedAt, &l.Note, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
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

func (s *PostgresStore) queryLogs(ctx context.Context, householdID int64, start, end time.Time) ([]ChoreLog, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, household_id, user_id, chore_id, completed_at, COALESCE(note,''), created_at FROM chore_logs WHERE household_id = $1 AND completed_at >= $2 AND completed_at < $3 ORDER BY completed_at`, householdID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []ChoreLog
	for rows.Next() {
		var l ChoreLog
		if err := rows.Scan(&l.ID, &l.HouseholdID, &l.UserID, &l.ChoreID, &l.CompletedAt, &l.Note, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}
