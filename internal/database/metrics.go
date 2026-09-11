package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// QuerySnapshot is cumulative since this process opened its pool. Durations
// include receiving rows from PostgreSQL; pool acquisition is reported by
// sql.DB.Stats separately. SQL and arguments are deliberately never retained.
type QuerySnapshot struct {
	Count          uint64 `json:"queries_total"`
	Failed         uint64 `json:"query_failures_total"`
	Nanoseconds    int64  `json:"query_nanoseconds_total"`
	MaxNanoseconds int64  `json:"query_max_nanoseconds"`
	// Non-cumulative bins: <=1,5,10,25,50,100,500,1000ms and >1000ms.
	LatencyBuckets [9]uint64 `json:"query_latency_buckets"`
}

type QueryMetrics struct {
	mu       sync.Mutex
	snapshot QuerySnapshot
	now      func() time.Time
}

func NewQueryMetrics() *QueryMetrics { return &QueryMetrics{now: time.Now} }

type queryStartKey struct{}

func (m *QueryMetrics) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryStartKey{}, m.now())
}

func (m *QueryMetrics) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	started, ok := ctx.Value(queryStartKey{}).(time.Time)
	if !ok {
		return
	}
	elapsed := m.now().Sub(started)
	if elapsed < 0 {
		elapsed = 0
	}
	index := 8
	for i, upper := range [...]time.Duration{time.Millisecond, 5 * time.Millisecond, 10 * time.Millisecond, 25 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond, 500 * time.Millisecond, time.Second} {
		if elapsed <= upper {
			index = i
			break
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshot.Count++
	if data.Err != nil {
		m.snapshot.Failed++
	}
	m.snapshot.Nanoseconds += elapsed.Nanoseconds()
	m.snapshot.MaxNanoseconds = max(m.snapshot.MaxNanoseconds, elapsed.Nanoseconds())
	m.snapshot.LatencyBuckets[index]++
}

func (m *QueryMetrics) Snapshot() QuerySnapshot {
	if m == nil {
		return QuerySnapshot{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshot
}

func (m *QueryMetrics) reportPool(db *sql.DB, logger *log.Logger) {
	m.reportPoolNamed(db, logger, "http")
}

func (m *QueryMetrics) reportPoolNamed(db *sql.DB, logger *log.Logger, name string) {
	pool := db.Stats()
	payload := struct {
		Event string `json:"event"`
		Pool  string `json:"pool"`
		QuerySnapshot
		MaxOpen          int   `json:"pool_max_open"`
		InUse            int   `json:"pool_in_use"`
		Idle             int   `json:"pool_idle"`
		WaitCount        int64 `json:"pool_waits_total"`
		WaitMilliseconds int64 `json:"pool_wait_ms_total"`
	}{"database.pool", name, m.Snapshot(), pool.MaxOpenConnections, pool.InUse, pool.Idle, pool.WaitCount, pool.WaitDuration.Milliseconds()}
	encoded, _ := json.Marshal(payload)
	logger.Print(string(encoded))
}

// MonitorPool belongs to the server shutdown context and shares its wait group.
// It runs no database query and exposes no HTTP diagnostics endpoint.
func (m *QueryMetrics) MonitorPool(ctx context.Context, db *sql.DB, logger *log.Logger) {
	m.MonitorPoolNamed(ctx, db, logger, "http")
}

func (m *QueryMetrics) MonitorPoolNamed(ctx context.Context, db *sql.DB, logger *log.Logger, name string) {
	if logger == nil {
		logger = log.Default()
	}
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			m.reportPoolNamed(db, logger, name)
		}
	}
}
