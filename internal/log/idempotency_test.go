package log_test

import (
	"context"
	"errors"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
	"sync"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
)

func TestIdempotencyRejectsMismatchedSubmission(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name        string
		user, chore int64
		note        string
	}{
		{"other-chore", 10, 101, "original"},
		{"other-author", 11, 100, "original"},
		{"changed-payload", 10, 100, "changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := chorelog.NewService(chorelog.NewMemoryStore())
			_, _, err := svc.LogChoreIdempotent(ctx, chorelog.CreateInput{ActorID: 10, HouseholdID: 1, UserID: 10, ChoreID: 100, Note: "original", IdempotencyKey: "one-submission"})
			if err != nil {
				t.Fatal(err)
			}
			got, created, err := svc.LogChoreIdempotent(ctx, chorelog.CreateInput{ActorID: tc.user, HouseholdID: 1, UserID: tc.user, ChoreID: tc.chore, Note: tc.note, IdempotencyKey: "one-submission"})
			if err == nil || created || got.ID != 0 {
				t.Fatalf("mismatched replay returned data: id=%d created=%v err=%v", got.ID, created, err)
			}
		})
	}
}

type replayBarrierStore struct {
	chorelog.Store
	mu      sync.Mutex
	reads   int
	entered chan struct{}
	release chan struct{}
}

func (s *replayBarrierStore) FindLogByIdempotencyKey(ctx context.Context, hid int64, key string) (*chorelog.ChoreLog, error) {
	l, err := s.Store.FindLogByIdempotencyKey(ctx, hid, key)
	s.mu.Lock()
	s.reads++
	wait := s.reads <= 2
	s.mu.Unlock()
	if wait {
		s.entered <- struct{}{}
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return l, err
}

func checkConcurrentReplay(t *testing.T, base chorelog.Store, in chorelog.CreateInput, mismatch bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store := &replayBarrierStore{Store: base, entered: make(chan struct{}, 2), release: make(chan struct{})}
	svc := chorelog.NewService(store)
	type result struct {
		log     chorelog.ChoreLog
		created bool
		err     error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		req := in
		if mismatch && i == 1 {
			req.Note += " different"
		}
		go func() {
			l, created, err := svc.LogChoreIdempotent(ctx, req)
			results <- result{l, created, err}
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-store.entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(store.release)
	created, conflicts := 0, 0
	var id int64
	for i := 0; i < 2; i++ {
		select {
		case got := <-results:
			if got.err != nil {
				if !mismatch || !errors.Is(got.err, chorelog.ErrIdempotencyConflict) || got.log.ID != 0 {
					t.Fatalf("replay: %+v", got)
				}
				conflicts++
				continue
			}
			if got.created {
				created++
			}
			if id != 0 && id != got.log.ID {
				t.Fatal("concurrent retry created distinct logs")
			}
			id = got.log.ID
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if created != 1 || (mismatch && conflicts != 1) {
		t.Fatalf("created=%d conflicts=%d", created, conflicts)
	}
	logs, err := base.ListLogsRange(ctx, in.HouseholdID, time.Now().AddDate(-1, 0, 0), time.Now().AddDate(1, 0, 0))
	if err != nil || len(logs) != 1 {
		t.Fatalf("stored logs=%d err=%v", len(logs), err)
	}
}

func TestConcurrentIdempotencyReplay(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		checkConcurrentReplay(t, chorelog.NewMemoryStore(), chorelog.CreateInput{HouseholdID: 1, ActorID: 10, UserID: 10, ChoreID: 100, Note: "original", IdempotencyKey: "race"}, mismatch)
	}
}

func TestReplayRequiresCurrentVisibilityAndOriginalActor(t *testing.T) {
	ctx := context.Background()
	hs := household.NewMemoryStore()
	hh, err := hs.CreateHousehold(ctx, "Home", "H", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := hs.AddMember(ctx, hh.ID, 2, household.RoleMember); err != nil {
		t.Fatal(err)
	}
	cs := chore.NewMemoryStore()
	c, err := cs.CreateChore(ctx, chore.Chore{HouseholdID: hh.ID, Name: "Shared"})
	if err != nil {
		t.Fatal(err)
	}
	store := chorelog.NewMemoryStore()
	svc := chorelog.NewService(store).WithAccess(cs, hs)
	in := chorelog.CreateInput{HouseholdID: hh.ID, ActorID: 2, UserID: 2, ChoreID: c.ID, Note: "original", IdempotencyKey: "saved"}
	l, _, err := svc.LogChoreIdempotent(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	// Same attributed author, different actor: knowing another actor's key
	// does not turn the request into that actor's original submission.
	other := in
	other.ActorID = 1
	if got, _, err := svc.LogChoreIdempotent(ctx, other); !errors.Is(err, chorelog.ErrIdempotencyConflict) || got.ID != 0 {
		t.Fatalf("actor replay: id=%d err=%v", got.ID, err)
	}
	l.Note = "private-canary"
	if err := store.UpdateLog(ctx, l); err != nil {
		t.Fatal(err)
	}
	// Editing a log must not invalidate an otherwise authorized retry.
	if got, created, err := svc.LogChoreIdempotent(ctx, in); err != nil || created || got.ID != l.ID {
		t.Fatalf("edited replay: id=%d created=%v err=%v", got.ID, created, err)
	}
	c.Visibility = chore.VisibilityAdmins
	if err := cs.UpdateChore(ctx, c); err != nil {
		t.Fatal(err)
	}
	if got, _, err := svc.LogChoreIdempotent(ctx, in); err == nil || got.ID != 0 {
		t.Fatalf("private replay returned id=%d err=%v", got.ID, err)
	}
}

type accessRaceStore struct {
	chorelog.Store
	conflict bool
	reads    int
	entered  chan struct{}
	resume   chan struct{}
}

func (s *accessRaceStore) FindLogByIdempotencyKey(ctx context.Context, hid int64, key string) (*chorelog.ChoreLog, error) {
	s.reads++
	if s.conflict && s.reads == 1 {
		return nil, nil
	}
	l, err := s.Store.FindLogByIdempotencyKey(ctx, hid, key)
	close(s.entered)
	select {
	case <-s.resume:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return l, err
}

func (s *accessRaceStore) CreateLog(context.Context, chorelog.ChoreLog) (chorelog.ChoreLog, error) {
	return chorelog.ChoreLog{}, chorelog.ErrIdempotencyConflict
}

func TestReplayRechecksAccessAfterLookup(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		for _, revokeMembership := range []bool{false, true} {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			hs := household.NewMemoryStore()
			hh, err := hs.CreateHousehold(ctx, "Home", "H", 1)
			if err != nil {
				t.Fatal(err)
			}
			if err := hs.AddMember(ctx, hh.ID, 2, household.RoleMember); err != nil {
				t.Fatal(err)
			}
			cs := chore.NewMemoryStore()
			c, err := cs.CreateChore(ctx, chore.Chore{HouseholdID: hh.ID, Name: "Shared"})
			if err != nil {
				t.Fatal(err)
			}
			base := chorelog.NewMemoryStore()
			in := chorelog.CreateInput{HouseholdID: hh.ID, ActorID: 2, UserID: 2, ChoreID: c.ID, Note: "canary", IdempotencyKey: "saved"}
			if _, _, err := chorelog.NewService(base).WithAccess(cs, hs).LogChoreIdempotent(ctx, in); err != nil {
				t.Fatal(err)
			}
			barrier := &accessRaceStore{Store: base, conflict: conflict, entered: make(chan struct{}), resume: make(chan struct{})}
			svc := chorelog.NewService(barrier).WithAccess(cs, hs)
			type result struct {
				id  int64
				err error
			}
			done := make(chan result, 1)
			go func() { l, _, err := svc.LogChoreIdempotent(ctx, in); done <- result{l.ID, err} }()
			select {
			case <-barrier.entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if revokeMembership {
				err = hs.RemoveMember(ctx, hh.ID, 2)
			} else {
				c.Visibility = chore.VisibilityAdmins
				err = cs.UpdateChore(ctx, c)
			}
			if err != nil {
				t.Fatal(err)
			}
			close(barrier.resume)
			select {
			case got := <-done:
				if got.err == nil || got.id != 0 {
					t.Fatalf("revoked replay conflict=%v membership=%v: %+v", conflict, revokeMembership, got)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			cancel()
		}
	}
}
