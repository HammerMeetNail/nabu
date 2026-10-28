package database

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/testdb"
	"github.com/jackc/pgx/v5/pgconn"
)

// openBounded opens the app's pool on this test's isolated database, as
// OpenMeasured opens it for the server. Its connections carry the database
// name as application_name, as testdb's do, so testdb.WaitBlocked sees them.
func openBounded(t *testing.T, isolated *sql.DB, query url.Values, options PoolOptions) *sql.DB {
	t.Helper()
	var name string
	if err := isolated.QueryRow(`SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	dsn, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal("invalid TEST_DATABASE_URL")
	}
	dsn.Path = "/" + name
	values := dsn.Query()
	values.Set("application_name", name)
	for key := range query {
		values.Set(key, query.Get(key))
	}
	dsn.RawQuery = values.Encode()
	if options.MaxOpen == 0 {
		options.MaxOpen, options.MaxIdle = 4, 2
	}
	db, _, err := OpenMeasured(dsn.String(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func setting(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var value string
	if err := db.QueryRow(`SELECT current_setting($1)`, name).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func TestEveryStatementAndLockWaitIsBounded(t *testing.T) {
	isolated := testdb.New(t)
	db := openBounded(t, isolated, nil, PoolOptions{StatementTimeout: 200 * time.Millisecond, LockTimeout: 100 * time.Millisecond})
	if got := setting(t, db, "statement_timeout"); got != "200ms" {
		t.Fatalf("statement_timeout = %q", got)
	}
	if got := setting(t, db, "lock_timeout"); got != "100ms" {
		t.Fatalf("lock_timeout = %q", got)
	}

	// A slow statement fails its own request.
	if _, err := db.Exec(`SELECT pg_sleep(1)`); sqlState(err) != "57014" {
		t.Fatalf("slow statement: err = %v, want query_canceled (57014)", err)
	}

	// So does waiting on another session's lock.
	holder, err := isolated.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if _, err := holder.ExecContext(context.Background(), `SELECT pg_advisory_lock(4242)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT pg_advisory_lock(4242)`); sqlState(err) != "55P03" {
		t.Fatalf("lock wait: err = %v, want lock_not_available (55P03)", err)
	}
}

func TestDefaultsApplyAndTheDatabaseURLWins(t *testing.T) {
	isolated := testdb.New(t)
	defaults := openBounded(t, isolated, nil, PoolOptions{})
	if got := setting(t, defaults, "statement_timeout"); got != "30s" {
		t.Fatalf("default statement_timeout = %q", got)
	}
	if got := setting(t, defaults, "lock_timeout"); got != "15s" {
		t.Fatalf("default lock_timeout = %q", got)
	}

	operator := openBounded(t, isolated, url.Values{"statement_timeout": {"60000"}, "lock_timeout": {"2000"}}, PoolOptions{})
	if got := setting(t, operator, "statement_timeout"); got != "1min" {
		t.Fatalf("operator statement_timeout = %q", got)
	}
	if got := setting(t, operator, "lock_timeout"); got != "2s" {
		t.Fatalf("operator lock_timeout = %q", got)
	}
}

func TestAMigrationRunsPastTheStatementTimeout(t *testing.T) {
	isolated := testdb.New(t)
	db := openBounded(t, isolated, nil, PoolOptions{StatementTimeout: 100 * time.Millisecond, LockTimeout: 100 * time.Millisecond})
	if _, err := db.Exec(`CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		t.Fatal(err)
	}
	if err := applyMigration(context.Background(), db, "slow.sql", `SELECT pg_sleep(0.3)`); err != nil {
		t.Fatalf("slow migration: %v", err)
	}
	// The lifted timeouts ended with its transaction.
	if _, err := db.Exec(`SELECT pg_sleep(0.3)`); sqlState(err) != "57014" {
		t.Fatalf("after the migration: err = %v, want query_canceled (57014)", err)
	}
}

func TestMigrateWaitsForAnotherInstancePastTheLockTimeout(t *testing.T) {
	isolated := testdb.New(t)
	db := openBounded(t, isolated, nil, PoolOptions{StatementTimeout: 100 * time.Millisecond, LockTimeout: 100 * time.Millisecond})

	// Another instance is migrating.
	other, err := isolated.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.ExecContext(context.Background(), `SELECT pg_advisory_lock($1)`, migrateLockKey); err != nil {
		t.Fatal(err)
	}
	var otherPID int
	if err := other.QueryRowContext(context.Background(), `SELECT pg_backend_pid()`).Scan(&otherPID); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- Migrate(context.Background(), db) }()
	testdb.WaitBlocked(t, isolated, otherPID, "pg_advisory_lock")
	time.Sleep(300 * time.Millisecond) // past both timeouts while it waits
	select {
	case err := <-done:
		t.Fatalf("Migrate gave up while another instance held the lock: %v", err)
	default:
	}
	if _, err := other.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrateLockKey); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Migrate: %v", err)
	}
}
