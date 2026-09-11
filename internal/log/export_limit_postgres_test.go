package log

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/readlimit"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func TestPostgresExportLimitBoundsHydrationBeforeAllocation(t *testing.T) {
	db := testdb.New(t)
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO households(id,name,invite_code) VALUES(1,'Synthetic','invite')`,
		`INSERT INTO users(id,email,password_hash,display_name) VALUES(1,'synthetic@example.invalid','','Synthetic')`,
		`INSERT INTO user_households(user_id,household_id,role) VALUES(1,1,'owner')`,
		`INSERT INTO chores(id,household_id,name) VALUES(1,1,'Synthetic')`,
		`INSERT INTO chore_logs(household_id,user_id,chore_id,completed_at,note) SELECT 1,1,1,'2026-09-10'::timestamptz,repeat('x',2000) FROM generate_series(1,20000)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s := NewPostgresStore(db)
	ctx := WithReadAccess(readlimit.With(context.Background(), 100), 1, 1, map[int64]struct{}{1: {}})
	start := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	// Warm the driver before recording allocation for the bounded read.
	_, _ = s.ListLogsRange(ctx, 1, start, start.AddDate(0, 0, 1))
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	logs, err := s.ListLogsRange(ctx, 1, start, start.AddDate(0, 0, 1))
	runtime.ReadMemStats(&after)
	if !errors.Is(err, readlimit.ErrExceeded) || logs != nil {
		t.Fatalf("large history became partial success: %d %v", len(logs), err)
	}
	allocated := after.TotalAlloc - before.TotalAlloc
	if allocated > 2<<20 {
		t.Fatalf("bounded101-row read allocated %d bytes for a40MB history", allocated)
	}
	t.Logf("20,000 synthetic rows /40MB note payload: bounded101-row read allocated %d bytes", allocated)
}
