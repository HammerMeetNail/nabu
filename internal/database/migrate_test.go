package database

import (
	"context"
	"errors"
	"github.com/HammerMeetNail/nabu/internal/testdb"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	migrationassets "github.com/HammerMeetNail/nabu/migrations"
)

// expectLock/expectUnlock set expectations for the session advisory lock that
// Migrate acquires (and releases) around the migration run.
func expectLock(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_lock($1)`)).
		WithArgs(migrateLockKey).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

func expectUnlock(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_unlock($1)`)).
		WithArgs(migrateLockKey).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

func TestMigrateAppliesPendingMigrations(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	names, err := migrationassets.Names()
	if err != nil {
		t.Fatalf("Names returned error: %v", err)
	}
	expectLock(mock)
	mock.ExpectExec(regexp.QuoteMeta(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT name FROM schema_migrations`)).
		WillReturnRows(sqlmock.NewRows([]string{"name"}))
	for _, name := range names {
		body, err := migrationassets.Assets.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile returned error: %v", err)
		}
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(string(body))).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations(name) VALUES ($1)`)).
			WithArgs(name).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
	}

	expectUnlock(mock)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestMigrateSkipsAppliedMigrations(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	names, err := migrationassets.Names()
	if err != nil {
		t.Fatalf("Names returned error: %v", err)
	}

	expectLock(mock)
	mock.ExpectExec(regexp.QuoteMeta(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT name FROM schema_migrations`)).
		WillReturnRows(func() *sqlmock.Rows {
			rows := sqlmock.NewRows([]string{"name"})
			for _, name := range names {
				rows.AddRow(name)
			}
			return rows
		}())

	expectUnlock(mock)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestMigrateReturnsCreateTableErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	expectLock(mock)
	mock.ExpectExec(regexp.QuoteMeta(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)).
		WillReturnError(errors.New("create failed"))

	expectUnlock(mock)
	err = Migrate(context.Background(), db)
	if err == nil || err.Error() != "create schema_migrations: create failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMigrateReturnsApplyErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	names, err := migrationassets.Names()
	if err != nil {
		t.Fatalf("Names returned error: %v", err)
	}
	body, err := migrationassets.Assets.ReadFile(names[0])
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	expectLock(mock)
	mock.ExpectExec(regexp.QuoteMeta(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT name FROM schema_migrations`)).
		WillReturnRows(sqlmock.NewRows([]string{"name"}))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(string(body))).
		WillReturnError(errors.New("syntax error"))
	mock.ExpectRollback()

	expectUnlock(mock)
	err = Migrate(context.Background(), db)
	if err == nil || err.Error() != "apply migration "+names[0]+": syntax error" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMigrateReturnsRecordErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	names, err := migrationassets.Names()
	if err != nil {
		t.Fatalf("Names returned error: %v", err)
	}
	body, err := migrationassets.Assets.ReadFile(names[0])
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	expectLock(mock)
	mock.ExpectExec(regexp.QuoteMeta(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT name FROM schema_migrations`)).
		WillReturnRows(sqlmock.NewRows([]string{"name"}))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(string(body))).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations(name) VALUES ($1)`)).
		WithArgs(names[0]).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	expectUnlock(mock)
	err = Migrate(context.Background(), db)
	if err == nil || err.Error() != "record migration "+names[0]+": insert failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenReturnsDatabaseHandle(t *testing.T) {
	db, err := Open("postgres://nabu:nabu@localhost:5432/nabu?sslmode=disable")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	if db.Driver() == nil {
		t.Fatal("expected database driver")
	}
}

func TestMigrateAgainstRealPostgres(t *testing.T) {
	db := testdb.New(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate against real Postgres failed: %v", err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("second migration run failed: %v", err)
	}
}
