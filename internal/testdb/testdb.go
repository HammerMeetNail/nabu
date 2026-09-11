// Package testdb gives integration tests an isolated PostgreSQL database. It
// never uses DATABASE_URL or touches an existing application's schema. The test
// role needs CREATEDB; each test owns and drops only its random database.
package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func New(t *testing.T) *sql.DB {
	t.Helper()
	return NewWithTracer(t, nil)
}

// NewWithTracer attaches instrumentation to this test's isolated pool.
func NewWithTracer(t *testing.T, tracer pgx.QueryTracer) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run isolated PostgreSQL integration tests")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid TEST_DATABASE_URL")
	}
	admin := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = admin.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := admin.PingContext(ctx); err != nil {
		t.Fatal("TEST_DATABASE_URL is configured but PostgreSQL is unavailable")
	}
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	databaseName := "nabu_test_" + hex.EncodeToString(id[:])
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+databaseName+" TEMPLATE template0"); err != nil {
		t.Fatal(err)
	}
	config.Database = databaseName
	config.RuntimeParams["application_name"] = databaseName
	config.Tracer = tracer
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(12)
	t.Cleanup(func() {
		_ = db.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP DATABASE "+databaseName); err != nil {
			t.Errorf("remove isolated test database: %v", err)
		}
	})
	return db
}

// WaitBlocked observes a real PostgreSQL wait. Polling waits for a database
// condition, never guesses when a concurrent goroutine has reached a barrier.
func WaitBlocked(t *testing.T, db *sql.DB, blockerPID int, queryContains string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		var pid int
		err := db.QueryRowContext(ctx, `WITH RECURSIVE waiting(pid) AS (
 SELECT pid FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid))
 UNION SELECT a.pid FROM pg_stat_activity a JOIN waiting w ON w.pid=ANY(pg_blocking_pids(a.pid))
 ) SELECT a.pid FROM pg_stat_activity a JOIN waiting w ON a.pid=w.pid
 WHERE a.application_name=current_setting('application_name') AND a.query LIKE '%' || $2 || '%' LIMIT 1`, blockerPID, queryContains).Scan(&pid)
		if err == nil {
			return pid
		}
		if err != sql.ErrNoRows {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
			t.Fatal("operation never reached expected database wait")
		case <-tick.C:
		}
	}
}
