package log

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPostgresLogStore_CreateLog(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO chore_logs (household_id, user_id, chore_id, completed_at, note, indicators, slot_hour, log_date, volume_ml, indicator_volumes) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`)).
		WithArgs(int64(1), int64(1), int64(1), now, "", "[]", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(1, now))

	entry, err := store.CreateLog(context.Background(), ChoreLog{
		HouseholdID: 1, UserID: 1, ChoreID: 1, CompletedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateLog: %v", err)
	}
	if entry.ID != 1 {
		t.Fatalf("ID = %d, want 1", entry.ID)
	}
}

func TestPostgresLogStore_GetLog(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT`).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "household_id", "user_id", "chore_id", "completed_at", "coalesce_note", "coalesce_indicators", "slot_hour", "created_at", "log_date", "volume_ml", "indicator_volumes"}).
			AddRow(1, 1, 1, 1, now, "", "[]", nil, now, nil, nil, nil))

	entry, err := store.GetLog(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetLog: %v", err)
	}
	if entry.ID != 1 {
		t.Fatalf("ID = %d, want 1", entry.ID)
	}
}

func TestPostgresLogStore_GetLogNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	mock.ExpectQuery(`SELECT`).WithArgs(int64(9)).WillReturnError(sql.ErrNoRows)

	_, err = store.GetLog(context.Background(), 9)
	if err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestPostgresLogStore_DeleteLog(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM chore_logs WHERE id = $1`)).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.DeleteLog(context.Background(), 1)
	if err != nil {
		t.Fatalf("DeleteLog: %v", err)
	}
}

func TestPostgresLogStore_ListLogs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, household_id, user_id, chore_id, completed_at, COALESCE(note,''), COALESCE(indicators,'[]'), slot_hour, created_at, log_date, volume_ml, indicator_volumes::text FROM chore_logs WHERE household_id = $1 AND COALESCE(log_date, completed_at::date) >= $2::date AND COALESCE(log_date, completed_at::date) < $3::date ORDER BY completed_at`)).
		WithArgs(int64(1), "2024-01-15", "2024-01-16").
		WillReturnRows(sqlmock.NewRows([]string{"id", "household_id", "user_id", "chore_id", "completed_at", "note", "indicators", "slot_hour", "created_at", "log_date", "volume_ml", "indicator_volumes"}).
			AddRow(1, 1, 1, 1, now, "", "[]", nil, now, nil, nil, nil))

	logs, err := store.ListLogs(context.Background(), 1, now)
	if err != nil {
		t.Fatalf("ListLogs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len = %d, want 1", len(logs))
	}
}

func TestPostgresLogStore_LatestPerChore_TiebreakerInOrderBy(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	// The ORDER BY must include id DESC as a tiebreaker so that when two
	// logs share the same completed_at, the higher-id (newer) row wins
	// after DISTINCT ON picks the first row in each group.
	mock.ExpectQuery("(?s).*ORDER BY chore_id, completed_at DESC, id DESC.*").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "household_id", "user_id", "chore_id", "completed_at", "coalesce_note", "coalesce_indicators", "slot_hour", "created_at", "log_date", "volume_ml", "indicator_volumes"}).
			AddRow(2, 1, 1, 5, now, "", "[]", nil, now, nil, nil, nil))

	result, err := store.LatestPerChore(context.Background(), 1)
	if err != nil {
		t.Fatalf("LatestPerChore: %v", err)
	}
	if result[5].ID != 2 {
		t.Errorf("LatestPerChore ID = %d, want 2", result[5].ID)
	}
}

func TestPostgresLogStore_ListLogsRange(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := NewPostgresStore(db)

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, household_id, user_id, chore_id, completed_at, COALESCE(note,''), COALESCE(indicators,'[]'), slot_hour, created_at, log_date, volume_ml, indicator_volumes::text FROM chore_logs WHERE household_id = $1 AND COALESCE(log_date, completed_at::date) >= $2::date AND COALESCE(log_date, completed_at::date) < $3::date ORDER BY completed_at`)).
		WithArgs(int64(1), "2024-01-01", "2024-01-08").
		WillReturnRows(sqlmock.NewRows([]string{"id", "household_id", "user_id", "chore_id", "completed_at", "note", "indicators", "slot_hour", "created_at", "log_date", "volume_ml", "indicator_volumes"}))

	logs, err := store.ListLogsRange(context.Background(), 1, start, end)
	if err != nil {
		t.Fatalf("ListLogsRange: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("len = %d, want 0", len(logs))
	}
}
