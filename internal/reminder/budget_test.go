package reminder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/push"
	"github.com/HammerMeetNail/nabu/internal/testsync"
)

// slowClaims holds each claim until its context ends, as a stalled database
// would.
type slowClaims struct {
	Store
	entered chan context.Context
}

func (s *slowClaims) ClaimReminder(ctx context.Context, _, _ int64, _ string, _ push.Job) (bool, error) {
	s.entered <- ctx
	<-ctx.Done()
	return false, ctx.Err()
}

// The tick only queues reminders, so a slow provider can't hold it; a slow
// database is bounded by its deadline and ends with the tick.
func TestReminderTickBoundsSlowClaimsAndJoinsCancellation(t *testing.T) {
	for _, cancelParent := range []bool{true, false} {
		t.Run(map[bool]string{true: "shutdown", false: "tick deadline"}[cancelParent], func(t *testing.T) {
			s, _, _, _ := reminderFixture(t, time.Now().UTC(), "UTC")
			claims := &slowClaims{Store: s.store, entered: make(chan context.Context, 1)}
			s.store = claims
			s.tickTimeout = 250 * time.Millisecond
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer waitCancel()
			done := make(chan error, 1)
			go func() { done <- s.tick(ctx) }()
			claimCtx := testsync.Receive(t, waitCtx, claims.entered)
			deadline, ok := claimCtx.Deadline()
			if !ok || time.Until(deadline) > 250*time.Millisecond {
				t.Fatal("claim escaped tick deadline")
			}
			if cancelParent {
				cancel()
			}
			err := testsync.Receive(t, waitCtx, done)
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("tick completed before joining its claim: %v", err)
			}
			if claimCtx.Err() == nil {
				t.Fatal("claim still active after tick returned")
			}
		})
	}
}

func TestMemoryReminderCursorMakesProgressAfterBudget(t *testing.T) {
	s, _, queued, first := reminderFixture(t, time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC), "UTC")
	for range 4 {
		copy := first
		copy.ID = 0
		if _, err := s.schedStore.Create(context.Background(), copy); err != nil {
			t.Fatal(err)
		}
	}
	s.maxCandidates = 2
	for range 3 {
		if err := s.tick(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if queued.count() != 5 || s.cursor != (candidateCursor{}) {
		t.Fatalf("budget starved schedules: queued=%d cursor=%+v", queued.count(), s.cursor)
	}
	if err := s.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queued.count() != 5 {
		t.Fatal("cursor wrapped without dedup")
	}
}
