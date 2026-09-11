package reminder

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/notification"
	"github.com/HammerMeetNail/nabu/internal/schedule"
	"github.com/HammerMeetNail/nabu/internal/testdb"
	"github.com/HammerMeetNail/nabu/internal/userprefs"
)

func schedulerDatabase(t *testing.T, households, members, chores int) (*sql.DB, *database.QueryMetrics) {
	t.Helper()
	metrics := database.NewQueryMetrics()
	db := testdb.NewWithTracer(t, metrics)
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO households(id,name,invite_code) SELECT n,'Synthetic','invite-'||n FROM generate_series(1,$1::int)n`, []any{households}},
		{`INSERT INTO users(id,email,password_hash,display_name) SELECT n,'synthetic-'||n||'@example.invalid','','Synthetic' FROM generate_series(1,$1::int)n`, []any{households * members}},
		{`INSERT INTO user_households(user_id,household_id,role) SELECT n,(n-1)/$2+1,CASE WHEN (n-1)%$2=0 THEN 'owner' ELSE 'member' END FROM generate_series(1,$1::int)n`, []any{households * members, members}},
		{`INSERT INTO chores(id,household_id,name,visibility) SELECT n,(n-1)/$2+1,'Synthetic chore '||n,'household' FROM generate_series(1,$1::int)n`, []any{households * chores, chores}},
		{`INSERT INTO chore_schedules(id,household_id,chore_id,frequency_type,specific_time) SELECT id,household_id,id,'daily','07:00' FROM chores`, nil},
		{`INSERT INTO chore_reminder_prefs(user_id,chore_id,enabled,lead_minutes) SELECT m.user_id,c.id,true,10 FROM user_households m JOIN chores c ON c.household_id=m.household_id`, nil},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	return db, metrics
}

func postgresScheduler(db *sql.DB, push notification.PushSender) *Scheduler {
	return NewScheduler(NewPostgresStore(db), schedule.NewPostgresStore(db), schedule.NewService(),
		notification.NewPostgresStore(db), chore.NewPostgresStore(db), household.NewPostgresStore(db), userprefs.NewPostgresStore(db), push)
}

func TestPostgresSchedulerCandidateBudget(t *testing.T) {
	db, metrics := schedulerDatabase(t, 60, 6, 12) // 720 schedules, 4,320 eligible pairs.
	push := &countPush{}
	s := postgresScheduler(db, push)
	s.now = func() time.Time { return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC) }
	s.SetQueryCounter(func() uint64 { return metrics.Snapshot().Count })
	var reports []TickReport
	s.observe = func(r TickReport) { reports = append(reports, r) }
	if err := s.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	r := reports[0]
	if r.Candidates != 4320 || r.CandidateQueries != 17 || r.PoolQueries != 17 || r.Due != 0 || push.calls != 0 || r.BudgetReached {
		t.Fatalf("non-due workload=%+v pushes=%d", r, push.calls)
	}
	// One assigned schedule per household is due; assigned recipients do not
	// need an enabled chore preference. Other candidates still share one page.
	if _, err := db.Exec(`UPDATE chore_schedules SET specific_time='12:00',assigned_to_user_id=(household_id-1)*6+1 WHERE (id-1)%12=0`); err != nil {
		t.Fatal(err)
	}
	if err := s.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	r = reports[1]
	if r.Reminded != 60 || push.calls != 60 || r.CandidateQueries > 17 || r.PoolQueries > 380 || r.Failed != 0 {
		t.Fatalf("due workload=%+v pushes=%d", r, push.calls)
	}
	if err := s.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if push.calls != 60 || reports[2].PoolQueries > 17 {
		t.Fatalf("repeated delivery or per-recipient dedup query: %+v", reports[2])
	}
	if path := os.Getenv("NABU_SCHEDULER_REPORT"); path != "" {
		data, err := json.MarshalIndent(reports, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPostgresReminderSnapshotLocalDatesAndAccess(t *testing.T) {
	db, _ := schedulerDatabase(t, 2, 3, 2)
	for _, q := range []string{
		`UPDATE chore_schedules SET specific_time='20:30',frequency_type='weekly',days_of_week='[0]',assigned_to_user_id=2 WHERE id=1`,
		`UPDATE chore_schedules SET is_active=false WHERE id<>1`,
		`DELETE FROM chore_reminder_prefs WHERE user_id=2`,
		`INSERT INTO reminder_preferences(user_id,push_enabled,timezone,default_reminder_lead_minutes) VALUES(2,true,'UTC',10)`,
		`INSERT INTO user_preferences(user_id,timezone) VALUES(2,'America/New_York')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	store := NewPostgresStore(db)
	page, err := store.CandidatePage(context.Background(), candidateCursor{}, 256, time.Date(2026, 9, 7, 0, 30, 0, 0, time.UTC))
	if err != nil || len(page) != 1 {
		t.Fatalf("assigned default candidate: %d %v", len(page), err)
	}
	s := postgresScheduler(db, &countPush{})
	now := time.Date(2026, 9, 7, 0, 30, 0, 0, time.UTC)
	c := page[0]
	day, due := s.dueDate(c.Schedule, now.In(reminderLocation(c.Preferences.Timezone, c.UserTimezone)), c.LeadMinutes)
	if !due || day.Format("2006-01-02") != "2026-09-06" {
		t.Fatal("bulk snapshot changed recipient-local recurrence")
	}
	// Quiet hours intentionally use the notification zone (UTC), even when
	// scheduling falls back to the user's New York preference.
	c.Preferences.QuietHoursStart, c.Preferences.QuietHoursEnd = "00:00", "01:00"
	if !quietAt(c.Preferences, now) {
		t.Fatal("quiet hours used schedule fallback timezone")
	}
	if _, err := db.Exec(`UPDATE chores SET visibility='admins' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if err := s.deliverCandidate(context.Background(), c); err == nil {
		t.Fatal("private change after page read allowed delivery")
	}
	page, err = store.CandidatePage(context.Background(), candidateCursor{}, 256, time.Date(2026, 9, 7, 0, 30, 0, 0, time.UTC))
	if err != nil || len(page) != 0 {
		t.Fatal("private candidate reached member")
	}
	if _, err := db.Exec(`UPDATE chores SET visibility='household' WHERE id=1; DELETE FROM user_households WHERE user_id=2`); err != nil {
		t.Fatal(err)
	}
	if err := s.deliverCandidate(context.Background(), c); err == nil {
		t.Fatal("removed member snapshot allowed delivery")
	}
	page, err = store.CandidatePage(context.Background(), candidateCursor{}, 256, time.Date(2026, 9, 7, 0, 30, 0, 0, time.UTC))
	if err != nil || len(page) != 0 {
		t.Fatal("removed assigned recipient remained eligible")
	}
}
