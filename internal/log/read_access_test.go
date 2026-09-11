package log_test

import (
	"context"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/database"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func TestPostgresReadAccessPrecedesLimitAndRechecksMembership(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO households(id,name,invite_code) VALUES (1,'Fixture','fixture'),(2,'Other','other')`,
		`INSERT INTO users(id,email,password_hash,display_name) VALUES (1,'fixture@example.invalid','','Fixture')`,
		`INSERT INTO user_households(user_id,household_id,role) VALUES (1,1,'member')`,
		`INSERT INTO chores(id,household_id,name,visibility) VALUES (10,1,'Visible','household'),(11,1,'Private','admins'),(12,2,'Other','household')`,
		`INSERT INTO chore_logs(id,household_id,user_id,chore_id,completed_at,note) VALUES (1,1,1,10,'2026-01-01','needle visible'),(2,1,1,10,'2026-01-01','needle visible tie'),(3,2,1,12,'2026-03-01','needle other')`,
		`INSERT INTO chore_logs(household_id,user_id,chore_id,completed_at,note) SELECT 1,1,11,'2026-02-01'::timestamptz + n*interval '1 second','needle private' FROM generate_series(10,120) n`,
	} {
		// Explicit fixture IDs must not overlap the sequence-based batch.
		if _, err := db.ExecContext(ctx, `SELECT setval('chore_logs_id_seq',100)`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	s := chorelog.NewPostgresStore(db)
	visible := map[int64]struct{}{10: {}, 11: {}, 12: {}}
	scoped := chorelog.WithReadAccess(ctx, 1, 1, visible)
	delete(visible, 10) // Caller mutation must not change a captured request.
	logs, err := s.SearchHistoryLogs(scoped, 1, "needle", 100)
	if err != nil || len(logs) != 2 || logs[0].ID != 2 {
		t.Fatalf("authorized search: count=%d err=%v", len(logs), err)
	}
	start := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	logs, more, err := s.HistoryLogs(scoped, 1, start, start.AddDate(0, 0, 7))
	if err != nil || len(logs) != 0 || !more {
		t.Fatalf("empty visible page: count=%d more=%v err=%v", len(logs), more, err)
	}
	latest, err := s.LatestPerChore(scoped, 1)
	if err != nil || len(latest) != 1 || latest[10].ID != 2 {
		t.Fatalf("latest: %+v err=%v", latest, err)
	}
	statsQuery := chorelog.StatsQuery{CandidateStart: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), CandidateEnd: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	logs, err = s.StatsLogs(scoped, 1, statsQuery)
	if err != nil || len(logs) != 2 {
		t.Fatalf("stats: count=%d err=%v", len(logs), err)
	}
	for _, l := range logs {
		if l.ChoreID != 10 || l.Note != "" || l.Title != nil {
			t.Fatalf("stats read unrelated data: %+v", l)
		}
	}
	for _, change := range []string{`UPDATE chores SET visibility='admins' WHERE id=10`, `DELETE FROM user_households WHERE user_id=1`} {
		if _, err := db.ExecContext(ctx, change); err != nil {
			t.Fatal(err)
		}
		logs, err = s.SearchHistoryLogs(scoped, 1, "needle", 100)
		if err != nil || len(logs) != 0 {
			t.Fatalf("stale scope search: count=%d err=%v", len(logs), err)
		}
		_, more, err = s.HistoryLogs(scoped, 1, start, start.AddDate(0, 0, 7))
		if err != nil || more {
			t.Fatalf("hidden older existence: more=%v err=%v", more, err)
		}
		latest, err = s.LatestPerChore(scoped, 1)
		if err != nil || len(latest) != 0 {
			t.Fatalf("stale scope latest: %+v err=%v", latest, err)
		}
		rows, err := s.Aggregate(scoped, 1, chorelog.AggregateQuery{CandidateStart: "1970-01-01", CandidateEnd: "9999-01-01", Group: chorelog.GroupMemberTotals})
		if err != nil || len(rows) != 0 {
			t.Fatalf("stale scope aggregate: count=%d err=%v", len(rows), err)
		}
		logs, err = s.StatsLogs(scoped, 1, statsQuery)
		if err != nil || len(logs) != 0 {
			t.Fatalf("stale scope stats: count=%d err=%v", len(logs), err)
		}
		// Exercise membership loss independently of the prior visibility change.
		if _, err := db.ExecContext(ctx, `UPDATE chores SET visibility='household' WHERE id=10`); err != nil {
			t.Fatal(err)
		}

	}
	logs, err = s.SearchHistoryLogs(scoped, 2, "needle", 100)
	if err != nil || len(logs) != 0 {
		t.Fatalf("scope used for wrong household: %v", err)
	}
	logs, err = s.StatsLogs(scoped, 2, statsQuery)
	if err != nil || len(logs) != 0 {
		t.Fatalf("stats scope used for wrong household: %v", err)
	}
}

func TestReadAccessEmptyIsDeniedInMemory(t *testing.T) {
	s := chorelog.NewMemoryStore()
	ctx := context.Background()
	if _, err := s.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: 1, ChoreID: 1, CompletedAt: time.Now(), Note: "needle"}); err != nil {
		t.Fatal(err)
	}
	for _, visible := range []map[int64]struct{}{nil, {}} {
		logs, err := s.SearchHistoryLogs(chorelog.WithReadAccess(ctx, 1, 1, visible), 1, "needle", 100)
		if err != nil || len(logs) != 0 {
			t.Fatal("empty permission set read a log")
		}
	}
}
