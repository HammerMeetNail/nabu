package auth

import (
	"context"
	"github.com/HammerMeetNail/nabu/internal/testsync"
	"testing"
	"time"
)

func TestMemorySessionGuardHoldsRevocationBoundary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s := NewMemoryStore()
	u, err := s.CreateUser(ctx, "guard@example.invalid", "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSession(ctx, u.ID, 0, "guard", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	entered, resume := make(chan struct{}), make(chan struct{})
	delivered := make(chan error, 1)
	go func() {
		delivered <- s.WithSession(ctx, u.ID, "guard", func() error { close(entered); testsync.Pause(ctx, resume); return ctx.Err() })
	}()
	testsync.Receive(t, ctx, entered)
	if s.mu.TryLock() {
		s.mu.Unlock()
		t.Fatal("entered callback does not guard session revocation")
	}
	revoked := make(chan error, 1)
	go func() { revoked <- s.DeleteSession(ctx, "guard") }()
	testsync.Eventually(t, ctx, func() bool {
		if s.mu.TryRLock() {
			s.mu.RUnlock()
			return false
		}
		return true
	})
	close(resume)
	if err := testsync.Receive(t, ctx, delivered); err != nil {
		t.Fatal(err)
	}
	if err := testsync.Receive(t, ctx, revoked); err != nil {
		t.Fatal(err)
	}
	if err := s.WithSession(ctx, u.ID, "guard", func() error { t.Error("delivery after revocation"); return nil }); err == nil {
		t.Fatal("expected revoked session")
	}
}
