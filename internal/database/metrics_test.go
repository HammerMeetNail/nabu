package database

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func TestQueryMetricsTrackConcurrentWorkWithoutPrivateValues(t *testing.T) {
	m := NewQueryMetrics()
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			ctx := m.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "SELECT PRIVATE-SQL", Args: []any{"PRIVATE-ARG"}})
			m.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: errors.New("PRIVATE-ERROR")})
		})
	}
	wg.Wait()
	snapshot := m.Snapshot()
	if snapshot.Count != 50 || snapshot.Failed != 50 {
		t.Fatalf("lost concurrent query observations: %+v", snapshot)
	}
	var bucketCount uint64
	for _, n := range snapshot.LatencyBuckets {
		bucketCount += n
	}
	if bucketCount != 50 || snapshot.Nanoseconds < snapshot.MaxNanoseconds {
		t.Fatal("inconsistent timing observations")
	}
	encoded, _ := json.Marshal(snapshot)
	if strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("retained private query input")
	}
}

func TestMeasuredPostgresQueriesAndPoolWaits(t *testing.T) {
	_ = testdb.New(t) // Require a reachable explicit test database, never DATABASE_URL.
	db, metrics, err := OpenMeasured(os.Getenv("TEST_DATABASE_URL"), PoolOptions{MaxOpen: 2, MaxIdle: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value int
	if err := db.QueryRow(`SELECT 1`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT 1/0`).Scan(&value); err == nil {
		t.Fatal("expected database error")
	}
	snapshot := metrics.Snapshot()
	if snapshot.Count != 2 || snapshot.Failed != 1 {
		t.Fatalf("real driver trace=%+v", snapshot)
	}
	db.SetMaxOpenConns(1)
	held, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- db.QueryRowContext(ctx, `SELECT 2`).Scan(&value) }()
	waitDeadline := time.NewTimer(3 * time.Second)
	defer waitDeadline.Stop()
	poll := time.NewTicker(time.Millisecond)
	defer poll.Stop()
	for db.Stats().WaitCount == 0 {
		select {
		case <-poll.C:
		case <-waitDeadline.C:
			t.Fatal("query never waited for the occupied pool")
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("pool cancellation=%v", err)
	}
	if metrics.Snapshot().Count != 2 {
		t.Fatal("pool wait counted as an executing query")
	}
	var output bytes.Buffer
	metrics.reportPool(db, log.New(&output, "", 0))
	var report map[string]any
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report["pool_waits_total"] != float64(1) || report["queries_total"] != float64(2) || report["query_failures_total"] != float64(1) {
		t.Fatalf("incomplete pool/query telemetry: %s", output.String())
	}
	metrics.MonitorPool(ctx, db, log.New(&output, "", 0)) // canceled context must terminate.
}

func TestOpenMeasuredRejectsInvalidOptionsAndSanitizesConnectionErrors(t *testing.T) {
	for _, options := range []PoolOptions{{MaxOpen: 1}, {MaxOpen: 101}, {MaxOpen: 2, MaxIdle: 3}, {MaxOpen: 2, MaxIdle: -1}} {
		if _, _, err := OpenMeasured("", options); err == nil {
			t.Fatal("invalid pool configuration accepted")
		}
	}
	_, _, err := OpenMeasured("postgres://SECRET%zz", PoolOptions{MaxOpen: 2})
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatal("connection parser leaked a value")
	}
}
