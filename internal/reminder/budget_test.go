package reminder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/testsync"
)

type heldPush struct{ entered chan context.Context }

func (p *heldPush) SendPushToUser(ctx context.Context, _ int64, _, _ string) error {
	p.entered <- ctx
	<-ctx.Done()
	return ctx.Err()
}

func TestReminderTickBoundsSlowProviderAndJoinsCancellation(t *testing.T) {
	for _, cancelParent := range []bool{true, false} {
		t.Run(map[bool]string{true: "shutdown", false: "tick deadline"}[cancelParent], func(t *testing.T) {
			s, _, _, _ := reminderFixture(t, time.Now().UTC(), "UTC")
			push := &heldPush{entered: make(chan context.Context, 1)}
			s.pushSender = push
			s.tickTimeout = 250 * time.Millisecond
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer waitCancel()
			done := make(chan error, 1)
			go func() { done <- s.tick(ctx) }()
			deliveryCtx := testsync.Receive(t, waitCtx, push.entered)
			deadline, ok := deliveryCtx.Deadline()
			if !ok || time.Until(deadline) > 250*time.Millisecond {
				t.Fatal("provider escaped tick deadline")
			}
			if cancelParent {
				cancel()
			}
			err := testsync.Receive(t, waitCtx, done)
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("tick completed before joining provider: %v", err)
			}
			if deliveryCtx.Err() == nil {
				t.Fatal("delivery still active after tick returned")
			}
		})
	}
}

func TestMemoryReminderCursorMakesProgressAfterBudget(t *testing.T) {
	s, _, push, first := reminderFixture(t, time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC), "UTC")
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
	if push.calls != 5 || s.cursor != (candidateCursor{}) {
		t.Fatalf("budget starved schedules: pushes=%d cursor=%+v", push.calls, s.cursor)
	}
	if err := s.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if push.calls != 5 {
		t.Fatal("cursor wrapped without dedup")
	}
}
