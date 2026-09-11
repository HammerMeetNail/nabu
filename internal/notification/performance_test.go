package notification

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

// Opt-in workload, restricted to testdb's fresh random database. Reports contain
// synthetic data and plans only; never configure it with application credentials.
func TestLocalNotificationWorkload(t *testing.T) {
	if os.Getenv("NABU_PERF") != "1" {
		t.Skip("set NABU_PERF=1 for the isolated 500k-notification workload")
	}
	metrics := database.NewQueryMetrics()
	db := testdb.NewWithTracer(t, metrics)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`DROP INDEX IF EXISTS idx_notifications_user_created`)
	exec(`DROP INDEX IF EXISTS idx_notifications_unread`)
	exec(`INSERT INTO users(id,email,password_hash,display_name) SELECT n,'synthetic-'||n||'@example.invalid','','Synthetic' FROM generate_series(1,10)n`)
	exec(`INSERT INTO notifications(user_id,type,title,body,is_read,created_at) SELECT h,'chore_logged','Synthetic notification',repeat('x',100),n%10!=0,'2026-09-10'::timestamptz-n*interval '3 minutes' FROM generate_series(1,10)h CROSS JOIN generate_series(1,50000)n`)
	exec(`VACUUM ANALYZE notifications`)
	store := NewPostgresStore(db)
	svc := NewService(store)
	at := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC).Add(-40000 * 3 * time.Minute)
	cursor := encodeCursor(Notification{ID: 40000, CreatedAt: at})
	samples := map[string]any{}
	report := map[string]any{"scope": "10 synthetic recipients, 50k notifications each, 10% unread, about104days; five 1k-row writes per index phase; isolated local host only", "go": runtime.Version(), "samples": samples}
	measure := func(name string, call func() error, planQuery string, args ...any) {
		t.Helper()
		if err := call(); err != nil {
			t.Fatal(err)
		}
		runtime.GC()
		var m0, m1 runtime.MemStats
		runtime.ReadMemStats(&m0)
		q0 := metrics.Snapshot()
		p0 := db.Stats()
		times := []float64{}
		for range 20 {
			start := time.Now()
			if err := call(); err != nil {
				t.Fatal(err)
			}
			times = append(times, float64(time.Since(start))/float64(time.Millisecond))
		}
		q1 := metrics.Snapshot()
		p1 := db.Stats()
		runtime.ReadMemStats(&m1)
		sort.Float64s(times)
		var plan json.RawMessage
		if err := db.QueryRowContext(ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) "+planQuery, args...).Scan(&plan); err != nil {
			t.Fatal(err)
		}
		samples[name] = map[string]any{"samples": 20, "median_ms": times[10], "p95_ms": times[18], "alloc_bytes": (m1.TotalAlloc - m0.TotalAlloc) / 20, "queries": float64(q1.Count-q0.Count) / 20, "pool_waits": p1.WaitCount - p0.WaitCount, "plan": plan}
		if name == "indexed_page" {
			if q1.Count-q0.Count != 40 {
				t.Fatal("page must use exactly 2 queries including unread count")
			}
			if (m1.TotalAlloc-m0.TotalAlloc)/20 > 1<<20 {
				t.Fatal("page allocation exceeds1MiB")
			}
			if os.Getenv("NABU_PERF_ENFORCE") == "1" && times[18] > 100 {
				t.Fatalf("page p95 %.2fms exceeds100ms local budget", times[18])
			}
		}
	}
	page := func() error {
		p, e := svc.ListPage(ctx, 1, cursor)
		if e == nil && (len(p.Notifications) != 50 || p.NextCursor == "") {
			t.Fatal("page shape changed")
		}
		return e
	}
	unread := func() error { _, e := store.GetUnreadCount(ctx, 1); return e }
	const pageSQL = `SELECT id,user_id,type,title,body,is_read,created_at FROM notifications WHERE user_id=$1 AND (created_at,id)<($2,$3) ORDER BY created_at DESC,id DESC LIMIT 51`
	const countSQL = `SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND is_read=false`
	writeBatches := func(name string) {
		times := []float64{}
		for range 5 {
			start := time.Now()
			exec(`INSERT INTO notifications(user_id,type,title,body,is_read) SELECT 1,'chore_logged','Synthetic write',repeat('x',100),true FROM generate_series(1,1000)`)
			times = append(times, float64(time.Since(start))/float64(time.Millisecond))
		}
		samples[name] = times
	}
	measure("baseline_page", page, pageSQL, 1, at, 40000)
	measure("baseline_deep_offset", func() error { _, e := store.ListNotifications(ctx, 1, 50, 40000); return e }, `SELECT id,user_id,type,title,body,is_read,created_at FROM notifications WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT 50 OFFSET 40000`, 1)
	measure("baseline_unread", unread, countSQL, 1)
	writeBatches("baseline_write_ms")
	exec(`CREATE INDEX idx_notifications_user_created ON notifications(user_id,created_at DESC,id DESC)`)
	exec(`CREATE INDEX idx_notifications_unread ON notifications(user_id) WHERE is_read=false`)
	exec(`VACUUM ANALYZE notifications`)
	measure("indexed_page", page, pageSQL, 1, at, 40000)
	measure("indexed_unread", unread, countSQL, 1)
	writeBatches("indexed_write_ms")
	var size int64
	if err := db.QueryRowContext(ctx, `SELECT pg_indexes_size('notifications')`).Scan(&size); err != nil {
		t.Fatal(err)
	}
	report["total_index_bytes"] = size
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	output := os.Getenv("NABU_PERF_REPORT")
	if output == "" {
		output = filepath.Join(os.TempDir(), "nabu-notification-workload.json")
	}
	if err := os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("local notification workload report: %s", output)
}
