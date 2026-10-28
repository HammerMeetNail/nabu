package reminder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/household"
)

// A suspended member keeps their membership row, so both the candidate query
// and the delivery membership check must exclude it explicitly.
func TestPostgresSchedulerSkipsSuspendedMembers(t *testing.T) {
	db, _ := schedulerDatabase(t, 1, 2, 1) // users 1 (owner) and 2 (member), chore 1.
	if _, err := db.Exec(`UPDATE chore_schedules SET specific_time='12:00'`); err != nil {
		t.Fatal(err)
	}
	if err := household.NewPostgresStore(db).SetMemberSuspended(context.Background(), 1, 2, true); err != nil {
		t.Fatal(err)
	}
	s := postgresScheduler(db)
	s.now = func() time.Time { return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC) }
	if err := s.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	var recipients []int64
	rows, err := db.Query(`SELECT user_id FROM push_jobs ORDER BY user_id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			t.Fatal(err)
		}
		recipients = append(recipients, uid)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(recipients) != 1 || recipients[0] != 1 {
		t.Fatalf("reminder recipients = %v, want only the active owner [1]", recipients)
	}
	var sent int
	if err := db.QueryRow(`SELECT count(*) FROM schedule_reminders WHERE user_id=2`).Scan(&sent); err != nil {
		t.Fatal(err)
	}
	if sent != 0 {
		t.Fatalf("suspended member has %d recorded reminders", sent)
	}
}

func TestPostgresWithMemberRejectsSuspendedMember(t *testing.T) {
	db, _ := schedulerDatabase(t, 1, 2, 1)
	store := household.NewPostgresStore(db)
	if err := store.SetMemberSuspended(context.Background(), 1, 2, true); err != nil {
		t.Fatal(err)
	}
	called := false
	err := household.WithMember(context.Background(), store, 2, 1, func(string) error {
		called = true
		return nil
	})
	if !errors.Is(err, household.ErrNotMember) || called {
		t.Fatalf("WithMember(suspended) err=%v delivered=%v", err, called)
	}
	if err := household.WithMember(context.Background(), store, 1, 1, func(string) error { return nil }); err != nil {
		t.Fatalf("WithMember(active owner): %v", err)
	}
}
