package reminder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/household"
)

type recipientPush struct{ users []int64 }

func (p *recipientPush) SendPushToUser(_ context.Context, uid int64, _, _ string) error {
	p.users = append(p.users, uid)
	return nil
}

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
	push := &recipientPush{}
	s := postgresScheduler(db, push)
	s.now = func() time.Time { return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC) }
	if err := s.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(push.users) != 1 || push.users[0] != 1 {
		t.Fatalf("reminder recipients = %v, want only the active owner [1]", push.users)
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
