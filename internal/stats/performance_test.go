package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/household"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Hide optional optimized readers to retain the original full-log read path.
type hydratedStatsStore struct{ chorelog.Store }

type statsWorkloadTrace struct {
	*database.QueryMetrics
	mu    sync.Mutex
	query string
	args  []any
}

func (s *statsWorkloadTrace) TraceQueryStart(ctx context.Context, c *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "FROM chore_logs") {
		// database/sql prepends pgx result-format options; they are not SQL
		// parameters and must not become placeholders in EXPLAIN EXECUTE.
		args := data.Args
		for len(args) > 0 {
			switch args[0].(type) {
			case pgx.QueryResultFormatsByOID, pgx.QueryResultFormats, pgx.QueryExecMode:
				args = args[1:]
				continue
			}
			break
		}
		s.mu.Lock()
		s.query, s.args = data.SQL, append([]any{}, args...)
		s.mu.Unlock()
	}
	return s.QueryMetrics.TraceQueryStart(ctx, c, data)
}

func TestLocalStatsWorkload(t *testing.T) {
	if os.Getenv("NABU_PERF") != "1" {
		t.Skip("set NABU_PERF=1 for the isolated four-month Stats workload")
	}
	metrics := &statsWorkloadTrace{QueryMetrics: database.NewQueryMetrics()}
	db := testdb.NewWithTracer(t, metrics)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	// Migrations reserve a separate lock connection. After they finish, keep the
	// measured prepared statement on one session for EXPLAIN EXECUTE.
	db.SetMaxOpenConns(1)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO households(id,name,invite_code) VALUES(1,'Synthetic stats','stats-fixture')`)
	exec(`INSERT INTO users(id,email,password_hash,display_name) SELECT n,'stats-'||n||'@example.invalid','','Person '||n FROM generate_series(1,4)n`)
	exec(`INSERT INTO user_households(user_id,household_id,role) SELECT n,1,'member' FROM generate_series(1,4)n`)
	exec(`INSERT INTO chores(id,household_id,name,category,visibility,has_volume_ml,metric_type,metric_unit)
 SELECT n,1,'Task '||n,'care',CASE WHEN n=25 THEN 'admins' ELSE 'household' END,true,'amount','mL' FROM generate_series(1,25)n`)
	pg := chorelog.NewPostgresStore(db)
	chores := aggregateChores{chore.NewPostgresStore(db)}
	members := household.NewPostgresStore(db)
	current := NewService(pg, chores).WithMemberships(members)
	before := NewService(hydratedStatsStore{pg}, chores).WithMemberships(members)
	viewer := WithViewer(ctx, 1)
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	type sample struct {
		Rows            int
		MedianMS, P95MS float64
		AllocBytes      uint64
		Queries         float64
		QueryMS         float64
		PoolWaits       int64
		Plan            json.RawMessage
	}
	report := map[string]sample{}
	measure := func(name string, rows int, call func() (any, error)) {
		t.Helper()
		if _, err := call(); err != nil {
			t.Fatal(err)
		}
		runtime.GC()
		var start, end runtime.MemStats
		runtime.ReadMemStats(&start)
		queries, pool := metrics.Snapshot(), db.Stats()
		times := make([]float64, 20)
		for i := range times {
			began := time.Now()
			if _, err := call(); err != nil {
				t.Fatal(err)
			}
			times[i] = float64(time.Since(began)) / float64(time.Millisecond)
		}
		runtime.ReadMemStats(&end)
		sort.Float64s(times)
		queryEnd := metrics.Snapshot()
		result := sample{Rows: rows, MedianMS: times[10], P95MS: times[18], AllocBytes: (end.TotalAlloc - start.TotalAlloc) / 20, Queries: float64(queryEnd.Count-queries.Count) / 20, QueryMS: float64(queryEnd.Nanoseconds-queries.Nanoseconds) / 20 / float64(time.Millisecond), PoolWaits: db.Stats().WaitCount - pool.WaitCount}
		metrics.mu.Lock()
		query, args := metrics.query, append([]any{}, metrics.args...)
		metrics.mu.Unlock()
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = conn.Raw(func(raw any) error {
			pg := raw.(*stdlib.Conn).Conn()
			var name string
			if err := pg.QueryRow(ctx, `SELECT name FROM pg_prepared_statements WHERE statement=$1`, query).Scan(&name); err != nil {
				return err
			}
			placeholders := make([]string, len(args))
			for i := range args {
				placeholders[i] = fmt.Sprintf("$%d", i+1)
			}
			// Simple protocol safely quotes bound arguments for EXECUTE's utility
			// syntax; this explains the actual warmed statement, including its
			// generic plan, rather than a newly prepared EXPLAIN of the same SQL.
			return pg.QueryRow(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE "+pgx.Identifier{name}.Sanitize()+"("+strings.Join(placeholders, ",")+")", append([]any{pgx.QueryExecModeSimpleProtocol}, args...)...).Scan(&result.Plan)
		})
		_ = conn.Close()
		if err != nil {
			t.Fatal(err)
		}
		report[name] = result
		t.Logf("%s: median %.2f ms, p95 %.2f ms, %d B/op, %.0f queries", name, result.MedianMS, result.P95MS, result.AllocBytes, result.Queries)
		if strings.HasSuffix(name, "/optimized") {
			if result.AllocBytes > 2<<20 || result.Queries > 5 {
				t.Errorf("Stats read exceeded allocation/query budget: %d B/op, %.0f queries", result.AllocBytes, result.Queries)
			}
			if os.Getenv("NABU_PERF_ENFORCE") == "1" && result.P95MS > 250 {
				t.Errorf("Stats read exceeded local latency budget: p95 %.2f ms", result.P95MS)
			}
		}
	}
	previous := 0
	for _, rows := range []int{6000, 24000, 60000} {
		exec(`INSERT INTO chore_logs(household_id,user_id,chore_id,completed_at,log_date,note,indicators,volume_ml,indicator_volumes,duration_seconds)
 SELECT 1,1+((n/25)%4),1+(n%25),$3::timestamptz-((n/25)%120)*interval '1 day'+interval '12 hours',
 ($3::timestamptz-((n/25)%120)*interval '1 day')::date,repeat('Synthetic activity note. ',20),'["a","b"]'::jsonb,100,'{"a":40,"b":60}'::jsonb,60
 FROM generate_series($1::int,$2::int)n`, previous+1, rows, today)
		exec(`ANALYZE chore_logs`)
		previous = rows
		for _, endpoint := range []string{"time-series", "heatmap"} {
			call := func(s *Service) (any, error) {
				if endpoint == "time-series" {
					return s.GetChoreTimeSeries(viewer, 1, 1, "daily", loc)
				}
				return s.GetHeatmap(viewer, 1, today.AddDate(0, -3, 0), today.AddDate(0, 0, 1), loc)
			}
			want, err := call(before)
			if err != nil {
				t.Fatal(err)
			}
			got, err := call(current)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s results changed", endpoint)
			}
			for _, path := range []struct {
				name    string
				service *Service
			}{{"hydrated", before}, {"optimized", current}} {
				measure(fmt.Sprintf("%d/%s/%s", rows, endpoint, path.name), rows, func() (any, error) { return call(path.service) })
			}
		}
	}
	if path := os.Getenv("NABU_PERF_REPORT"); path != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
