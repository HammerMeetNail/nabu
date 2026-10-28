package reminder

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/push"
)

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A reminder's record and its push commit together: it is queued once, and a
// push that can't be queued leaves the reminder for the next tick.
func TestPostgresClaimReminderQueuesItsPushOnce(t *testing.T) {
	db, _ := schedulerDatabase(t, 1, 1, 1) // user 1 in household 1, chore and schedule 1.
	store := NewPostgresStore(db)
	ctx := context.Background()
	job := push.Job{UserID: 1, HouseholdID: 1, ChoreID: 1, Type: "schedule_reminder", Title: "Synthetic chore 1",
		Body: "Due at 7:00 AM", Data: reminderPushData(1), DedupeKey: "reminder:1:1:2026-09-10", ExpiresAt: time.Now().Add(time.Hour)}
	for i, want := range []bool{true, false} {
		claimed, err := store.ClaimReminder(ctx, 1, 1, "2026-09-10", job)
		if err != nil || claimed != want {
			t.Fatalf("claim %d = %v %v, want %v", i, claimed, err, want)
		}
	}
	if n := countRows(t, db, `SELECT count(*) FROM push_jobs`); n != 1 {
		t.Fatalf("queued %d pushes for one reminder", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM schedule_reminders`); n != 1 {
		t.Fatalf("recorded %d reminders", n)
	}

	unqueueable := job
	unqueueable.ChoreID, unqueueable.DedupeKey = 404, "reminder:1:1:2026-09-11"
	if claimed, err := store.ClaimReminder(ctx, 1, 1, "2026-09-11", unqueueable); err == nil || claimed {
		t.Fatalf("claim with a push that can't be queued = %v %v", claimed, err)
	}
	if n := countRows(t, db, `SELECT count(*) FROM schedule_reminders WHERE scheduled_date='2026-09-11'`); n != 0 {
		t.Fatal("reminder recorded without its push")
	}
}
