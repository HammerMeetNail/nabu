package log

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
	"github.com/jackc/pgx/v5"
)

type capturedQuery struct {
	SQL  string
	Args []any
}
type workloadTrace struct {
	*database.QueryMetrics
	mu     sync.Mutex
	latest capturedQuery
}

func (w *workloadTrace) TraceQueryStart(ctx context.Context, c *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	w.mu.Lock()
	w.latest = capturedQuery{data.SQL, append([]any{}, data.Args...)}
	w.mu.Unlock()
	return w.QueryMetrics.TraceQueryStart(ctx, c, data)
}
func (w *workloadTrace) last() capturedQuery { w.mu.Lock(); defer w.mu.Unlock(); return w.latest }

type workloadSample struct {
	Samples                  int
	DatasetRows              int64
	MedianMS, P95MS          float64
	AllocBytes, AllocObjects uint64
	Queries                  float64
	PoolWaits                int64
	PoolWaitMS               float64
	Plan                     json.RawMessage
}

// TestLocalHistoryWorkload is an opt-in, destructive-to-its-own-random-database
// measurement. It never reads an application's database or chooses production
// credentials. All measurements and plans are written outside the repository.
func TestLocalHistoryWorkload(t *testing.T) {
	if os.Getenv("NABU_PERF") != "1" {
		t.Skip("set NABU_PERF=1 for the isolated 500k-row workload")
	}
	trace := &workloadTrace{QueryMetrics: database.NewQueryMetrics()}
	db := testdb.NewWithTracer(t, trace)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`DROP INDEX IF EXISTS idx_chore_logs_household_chore_latest`)
	exec(`DROP INDEX IF EXISTS idx_chore_logs_search_trgm`)
	exec(`INSERT INTO households(id,name,invite_code) SELECT n,'Synthetic '||n,'synthetic-'||n FROM generate_series(1,10)n`)
	exec(`INSERT INTO users(id,email,password_hash,display_name) SELECT n,'synthetic-'||n||'@example.invalid','','Synthetic' FROM generate_series(1,10)n`)
	exec(`INSERT INTO user_households(user_id,household_id,role) SELECT n,n,'member' FROM generate_series(1,10)n`)
	exec(`INSERT INTO chores(id,household_id,name,category,visibility) SELECT h*100+c,h,'Synthetic '||c,'care',CASE WHEN c=25 THEN 'admins' ELSE 'household' END FROM generate_series(1,10)h CROSS JOIN generate_series(1,25)c`)
	insert := `INSERT INTO chore_logs(household_id,user_id,chore_id,completed_at,log_date,note,title,indicators,volume_ml,indicator_volumes,duration_seconds)
 SELECT h,h,h*100+1+(n%25),'2026-09-10'::timestamptz-n*interval '30 minutes',('2026-09-10'::timestamptz-n*interval '30 minutes')::date,
 'Synthetic regular entry '||n||CASE WHEN n%1000=0 THEN ' raretarget' ELSE '' END,'Fixture','["a","b"]'::jsonb,100,'{"a":40,"b":60}'::jsonb,60
 FROM generate_series(1,10)h CROSS JOIN generate_series($1::int,$2::int)n`
	exec(insert, 1, 50000)
	exec(`ANALYZE chore_logs`)
	exec(`ANALYZE chores`)
	exec(`ANALYZE user_households`)
	store := NewPostgresStore(db)
	scope := func(h int64) context.Context {
		ids := map[int64]struct{}{}
		for c := int64(1); c <= 24; c++ {
			ids[h*100+c] = struct{}{}
		}
		return WithReadAccess(ctx, h, h, ids)
	}
	var report = map[string]any{"scope": "10 isolated households, 50k logs each, 25 chores each, 2.85 years, growing to 530k logs in write phases; local development host only", "go": runtime.Version(), "cpus": runtime.NumCPU(), "samples": map[string]workloadSample{}, "writeMS": map[string][]float64{}}
	samples := report["samples"].(map[string]workloadSample)
	measure := func(name string, call func() error) {
		t.Helper()
		if err := call(); err != nil {
			t.Fatal(err)
		}
		query := trace.last()
		times := []float64{}
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		queries := trace.Snapshot()
		pool := db.Stats()
		for range 20 {
			began := time.Now()
			if err := call(); err != nil {
				t.Fatal(err)
			}
			times = append(times, float64(time.Since(began))/float64(time.Millisecond))
		}
		runtime.ReadMemStats(&after)
		endPool := db.Stats()
		endQueries := trace.Snapshot()
		sort.Float64s(times)
		var plan []byte
		if err := db.QueryRowContext(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+query.SQL, query.Args...).Scan(&plan); err != nil {
			t.Fatal(err)
		}
		var datasetRows int64
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM chore_logs`).Scan(&datasetRows); err != nil {
			t.Fatal(err)
		}
		samples[name] = workloadSample{Samples: len(times), DatasetRows: datasetRows, MedianMS: times[len(times)/2], P95MS: times[(len(times)*95+99)/100-1], AllocBytes: (after.TotalAlloc - before.TotalAlloc) / 20, AllocObjects: (after.Mallocs - before.Mallocs) / 20, Queries: float64(endQueries.Count-queries.Count) / 20, PoolWaits: endPool.WaitCount - pool.WaitCount, PoolWaitMS: float64(endPool.WaitDuration-pool.WaitDuration) / float64(time.Millisecond), Plan: plan}
	}
	latest := func() error {
		logs, err := store.LatestPerChore(scope(1), 1)
		if err == nil && len(logs) != 24 {
			return fmt.Errorf("latest lookup returned %d logs", len(logs))
		}
		return err
	}
	search := func() error { _, err := store.SearchHistoryLogs(scope(1), 1, "absentneedle", 100); return err }
	rare := func() error { _, err := store.SearchHistoryLogs(scope(1), 1, "raretarget", 100); return err }
	oldLatest := func() error {
		scoped := scope(1)
		access, args := readAccessSQL(scoped, 1, "chore_logs.chore_id", "chore_logs.household_id", 2)
		rows, err := db.QueryContext(scoped, `SELECT DISTINCT ON(chore_id) `+logColumns+` FROM chore_logs WHERE household_id=$1`+access+` ORDER BY chore_id,completed_at DESC,id DESC`, append([]any{int64(1)}, args...)...)
		if err != nil {
			return err
		}
		logs, err := scanLogRows(rows)
		if err == nil && len(logs) != 24 {
			return fmt.Errorf("latest baseline returned %d logs", len(logs))
		}
		return err
	}

	allRows := func() error {
		_, err := store.ListLogsRange(scope(1), 1, time.Unix(0, 0), time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
		return err
	}
	aggregate := func() error {
		_, err := store.Aggregate(scope(1), 1, AggregateQuery{CandidateStart: "1970-01-01", CandidateEnd: "9999-01-01", Group: GroupChore})
		return err
	}
	write := func(name string, first int) {
		values := []float64{}
		for batch := range 5 {
			start := time.Now()
			exec(insert, first+batch*200, first+batch*200+199)
			values = append(values, float64(time.Since(start))/float64(time.Millisecond))
		}
		report["writeMS"].(map[string][]float64)[name] = values
	}

	measure("old_latest", oldLatest)
	measure("lateral_before_index", latest)
	measure("search_miss_before_index", search)
	measure("search_rare_before_index", rare)
	measure("all_rows", allRows)
	measure("aggregate_before_index", aggregate)
	write("before_indexes_10k", 50001)
	exec(`CREATE INDEX perf_latest ON chore_logs(household_id,chore_id,completed_at DESC,id DESC)`)
	exec(`ANALYZE chore_logs`)
	exec(`ANALYZE chores`)
	exec(`ANALYZE user_households`)
	measure("lateral_with_index", latest)
	measure("aggregate_with_index", aggregate)
	write("with_latest_index_10k", 51001)
	// Extension and indexes are confined to this test's disposable database.
	exec(`CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public`)
	exec(`CREATE INDEX perf_search ON chore_logs USING GIN(note public.gin_trgm_ops,title public.gin_trgm_ops)`)

	exec(`ANALYZE chore_logs`)
	exec(`ANALYZE chores`)
	exec(`ANALYZE user_households`)
	measure("search_miss_trigram", search)
	measure("search_rare_trigram", rare)
	write("with_both_indexes_10k", 52001)
	var bytes int64
	if err := db.QueryRowContext(ctx, `SELECT pg_indexes_size('chore_logs')`).Scan(&bytes); err != nil {
		t.Fatal(err)
	}
	report["allIndexesBytes"] = bytes
	// Compare a quiet phase with an oversubscribed pool. Each worker makes
	// serial queries; more workers than connections creates real acquisition waits.
	concurrency := map[int]any{}
	for _, workers := range []int{10, 24} {
		began := time.Now()
		pool := db.Stats()
		beforeQueries := trace.Snapshot()
		errs := make(chan error, workers)
		var wg sync.WaitGroup
		latencies := make(chan float64, workers*20)
		for worker := range workers {
			h := int64(worker%10 + 1)
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 20 {
					start := time.Now()
					_, err := store.LatestPerChore(scope(h), h)
					if err == nil {
						_, err = store.Aggregate(scope(h), h, AggregateQuery{CandidateStart: "1970-01-01", CandidateEnd: "9999-01-01", Group: GroupChore})
					}
					if err != nil {
						errs <- err
						return
					}
					latencies <- float64(time.Since(start)) / float64(time.Millisecond)
				}
			}()
		}
		wg.Wait()
		close(errs)
		close(latencies)
		for err := range errs {
			t.Fatal(err)
		}
		elapsed := []float64{}
		for ms := range latencies {
			elapsed = append(elapsed, ms)
		}
		sort.Float64s(elapsed)
		afterPool := db.Stats()
		afterQueries := trace.Snapshot()
		concurrency[workers] = map[string]any{"operations": len(elapsed), "elapsedSeconds": time.Since(began).Seconds(), "p95MS": elapsed[(len(elapsed)*95+99)/100-1], "poolWaits": afterPool.WaitCount - pool.WaitCount, "poolWaitMS": float64(afterPool.WaitDuration-pool.WaitDuration) / float64(time.Millisecond), "queriesPerOperation": float64(afterQueries.Count-beforeQueries.Count) / float64(len(elapsed))}
	}
	report["concurrent"] = concurrency

	path := os.Getenv("NABU_PERF_REPORT")
	if path == "" {
		path = filepath.Join(os.TempDir(), "nabu-history-workload.json")
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("workload report: %s", path)
	// Shape/heap budgets are hardware-independent regressions. Latency is an
	// optional gate for the documented host; shared CI hardware is not capacity.
	for _, name := range []string{"lateral_with_index", "search_miss_trigram", "search_rare_trigram", "aggregate_with_index"} {
		sample := samples[name]
		if sample.Queries != 1 {
			t.Errorf("%s: queries/read=%g, budget=1", name, sample.Queries)
		}
		if sample.AllocBytes > 1024*1024 {
			t.Errorf("%s: allocated bytes/read=%d, budget=1 MiB", name, sample.AllocBytes)
		}
		if os.Getenv("NABU_PERF_ENFORCE") == "1" {
			budget := 100.0
			if name == "aggregate_with_index" {
				budget = 250
			}
			if sample.P95MS > budget {
				t.Errorf("%s: p95=%.1fms, local budget=%.0fms", name, sample.P95MS, budget)
			}
		}
	}
	for workers, result := range concurrency {
		result := result.(map[string]any)
		if result["queriesPerOperation"].(float64) != 2 {
			t.Errorf("%d workers: query budget exceeded", workers)
		}
		if os.Getenv("NABU_PERF_ENFORCE") == "1" && result["p95MS"].(float64) > 1000 {
			t.Errorf("%d workers: p95 exceeded local 1s budget", workers)
		}
	}
}
