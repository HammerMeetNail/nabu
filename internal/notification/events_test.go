package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/push"
)

// profiles names the fixture's members for the memory household store.
type profiles map[int64]string

func (profiles) SetMembershipResolver(auth.MembershipResolver) {}

func (p profiles) Profile(_ context.Context, id int64) (auth.User, error) {
	return auth.User{ID: id, DisplayName: p[id]}, nil
}

// fanOutFixture is a household of Alice (1, owner), Bob (2) and Carol (3),
// with one activity, whose events fan out into a memory push queue.
type fanOutFixture struct {
	svc    *Service
	store  *MemoryStore
	events *MemoryEventStore
	jobs   *push.MemoryJobStore
	hh     *household.MemoryStore
	chores *chore.MemoryStore
	home   int64
	dishes chore.Chore
	now    time.Time
}

func newFanOut(t *testing.T) *fanOutFixture {
	t.Helper()
	ctx := context.Background()
	hh := household.NewMemoryStore()
	household.NewService(hh, profiles{1: "Alice", 2: "Bob", 3: "Carol"})
	home, err := hh.CreateHousehold(ctx, "Home", "H", 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{2, 3} {
		if err := hh.AddMember(ctx, home.ID, id, household.RoleMember); err != nil {
			t.Fatal(err)
		}
	}
	chores := chore.NewMemoryStore()
	dishes, err := chores.CreateChore(ctx, chore.Chore{HouseholdID: home.ID, Name: "Dishes", Icon: "🍽️", Visibility: chore.VisibilityHousehold})
	if err != nil {
		t.Fatal(err)
	}
	f := &fanOutFixture{store: NewMemoryStore(), events: NewMemoryEventStore(), jobs: push.NewMemoryJobStore(),
		hh: hh, chores: chores, home: home.ID, dishes: dishes, now: time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)}
	f.svc = NewService(f.store).WithAuthorization(hh, chores).WithPushQueue(f.jobs, nil)
	f.svc.WithEvents(f.events, nil)
	f.svc.now = func() time.Time { return f.now }
	return f
}

func (f *fanOutFixture) logged(t *testing.T, subject, actor int64) {
	t.Helper()
	if err := f.svc.EnqueueChoreLogged(context.Background(), f.home, f.dishes.ID, subject, actor); err != nil {
		t.Fatal(err)
	}
	f.svc.DeliverPendingEvents(context.Background())
}

func (f *fanOutFixture) notified(t *testing.T, userID int64) []Notification {
	t.Helper()
	list, _, err := f.svc.List(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func (f *fanOutFixture) pushes() map[int64][]push.Job {
	out := map[int64][]push.Job{}
	for _, job := range f.jobs.Pending() {
		out[job.UserID] = append(out[job.UserID], job)
	}
	return out
}

func TestFanOutSkipsTheSubjectAndTheActor(t *testing.T) {
	for _, tc := range []struct {
		name           string
		subject, actor int64
		want           map[int64]int
	}{
		{"logged their own", 1, 1, map[int64]int{1: 0, 2: 1, 3: 1}},
		{"logged for someone else", 1, 2, map[int64]int{1: 0, 2: 0, 3: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFanOut(t)
			f.logged(t, tc.subject, tc.actor)
			for userID, want := range tc.want {
				if got := len(f.notified(t, userID)); got != want {
					t.Errorf("user %d got %d notifications, want %d", userID, got, want)
				}
			}
			if pending := f.events.Pending(); len(pending) != 0 {
				t.Fatalf("event not finished: %+v", pending)
			}
		})
	}
}

func TestFanOutMessage(t *testing.T) {
	f := newFanOut(t)
	f.logged(t, 1, 1)
	list := f.notified(t, 2)
	if len(list) != 1 || list[0].Type != "chore_logged" || list[0].Title != "🍽️ Dishes" || list[0].Body != "Alice logged this" {
		t.Fatalf("notification = %+v", list)
	}
	jobs := f.pushes()[2]
	if len(jobs) != 1 {
		t.Fatalf("push jobs for Bob = %d", len(jobs))
	}
	job := jobs[0]
	if job.Title != "🍽️ Dishes" || job.Body != "Alice logged this" || job.Type != "chore_logged" ||
		job.HouseholdID != f.home || job.ChoreID != f.dishes.ID || job.Data["choreId"] != f.dishes.ID ||
		job.Data["householdId"] != f.home || job.Data["type"] != "chore_logged" {
		t.Fatalf("push job = %+v", job)
	}
	if !job.ExpiresAt.Equal(f.now.Add(push.JobTTL)) {
		t.Fatalf("push job expires %v", job.ExpiresAt)
	}
}

func TestFanOutPushFollowsPreferences(t *testing.T) {
	for _, tc := range []struct {
		name  string
		prefs *ReminderPreference
		want  int
	}{
		{"default", nil, 1},
		{"push off", &ReminderPreference{UserID: 2, PushEnabled: false}, 0},
		{"another type only", &ReminderPreference{UserID: 2, PushEnabled: true, EnabledPushTypes: []string{"schedule_reminder"}}, 0},
		{"this type", &ReminderPreference{UserID: 2, PushEnabled: true, EnabledPushTypes: []string{"chore_logged"}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFanOut(t)
			if tc.prefs != nil {
				if err := f.store.UpdateReminderPreferences(context.Background(), *tc.prefs); err != nil {
					t.Fatal(err)
				}
			}
			f.logged(t, 1, 1)
			if got := len(f.pushes()[2]); got != tc.want {
				t.Fatalf("push jobs = %d, want %d", got, tc.want)
			}
			// The in-app notification doesn't depend on push preferences.
			if got := len(f.notified(t, 2)); got != 1 {
				t.Fatalf("notifications = %d", got)
			}
		})
	}
}

func TestFanOutSkipsSuspendedMembersAndHiddenActivities(t *testing.T) {
	ctx := context.Background()
	f := newFanOut(t)
	if err := f.hh.SetMemberSuspended(ctx, f.home, 3, true); err != nil {
		t.Fatal(err)
	}
	f.logged(t, 1, 1)
	if len(f.notified(t, 3)) != 0 || len(f.pushes()[3]) != 0 {
		t.Fatal("suspended member was notified")
	}

	f = newFanOut(t)
	if err := f.hh.UpdateMemberRole(ctx, f.home, 3, household.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	f.dishes.Visibility = chore.VisibilityAdmins
	if err := f.chores.UpdateChore(ctx, f.dishes); err != nil {
		t.Fatal(err)
	}
	f.logged(t, 1, 1)
	if len(f.notified(t, 2)) != 0 || len(f.pushes()[2]) != 0 {
		t.Fatal("member was notified of an admins-only activity")
	}
	if len(f.notified(t, 3)) != 1 {
		t.Fatal("admin was not notified of an admins-only activity")
	}
}

func TestFanOutOfADeletedActivityTellsNoOne(t *testing.T) {
	f := newFanOut(t)
	if err := f.svc.EnqueueChoreLogged(context.Background(), f.home, f.dishes.ID, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := f.chores.DeleteChore(context.Background(), f.dishes.ID); err != nil {
		t.Fatal(err)
	}
	f.svc.DeliverPendingEvents(context.Background())
	if len(f.notified(t, 2)) != 0 || len(f.events.Pending()) != 0 {
		t.Fatal("deleted activity notified, or its event was kept")
	}
}

func TestHouseholdJoinedFanOut(t *testing.T) {
	ctx := context.Background()
	f := newFanOut(t)
	if err := f.svc.EnqueueHouseholdJoined(ctx, f.home, 3, "Carol", "Home"); err != nil {
		t.Fatal(err)
	}
	f.svc.DeliverPendingEvents(ctx)
	for _, userID := range []int64{1, 2} {
		list := f.notified(t, userID)
		if len(list) != 1 || list[0].Type != "household_joined" || list[0].Title != "New Member" || list[0].Body != "Carol joined Home" {
			t.Fatalf("user %d notifications = %+v", userID, list)
		}
		jobs := f.pushes()[userID]
		if len(jobs) != 1 || jobs[0].ChoreID != 0 || jobs[0].Data["choreId"] != int64(0) {
			t.Fatalf("user %d push jobs = %+v", userID, jobs)
		}
	}
	if len(f.notified(t, 3)) != 0 {
		t.Fatal("joiner was notified of their own join")
	}
}

// failOnce fails one user's first notification, as a database error would.
type failOnce struct {
	*MemoryStore
	userID int64
	failed bool
}

func (s *failOnce) CreateNotification(ctx context.Context, n Notification) (Notification, error) {
	if n.UserID == s.userID && !s.failed {
		s.failed = true
		return Notification{}, errors.New("database unavailable")
	}
	return s.MemoryStore.CreateNotification(ctx, n)
}

func TestRetriedFanOutWritesNothingTwice(t *testing.T) {
	ctx := context.Background()
	f := newFanOut(t)
	flaky := &failOnce{MemoryStore: f.store, userID: 3}
	f.svc.store = flaky
	f.logged(t, 1, 1)
	if !flaky.failed || len(f.events.Pending()) != 1 {
		t.Fatal("failed fan-out was not kept for a retry")
	}
	// Not due yet.
	f.svc.DeliverPendingEvents(ctx)
	if len(f.notified(t, 3)) != 0 {
		t.Fatal("retried before its delay")
	}
	f.now = f.now.Add(10 * time.Second)
	f.svc.DeliverPendingEvents(ctx)
	for _, userID := range []int64{2, 3} {
		if got := len(f.notified(t, userID)); got != 1 {
			t.Fatalf("user %d notifications after retry = %d", userID, got)
		}
		if got := len(f.pushes()[userID]); got != 1 {
			t.Fatalf("user %d push jobs after retry = %d", userID, got)
		}
	}
	if len(f.events.Pending()) != 0 {
		t.Fatal("event kept after a successful retry")
	}
}

func TestExpiredEventIsDropped(t *testing.T) {
	ctx := context.Background()
	f := newFanOut(t)
	if err := f.svc.EnqueueChoreLogged(ctx, f.home, f.dishes.ID, 1, 1); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(eventTTL)
	f.svc.DeliverPendingEvents(ctx)
	if len(f.notified(t, 2)) != 0 || len(f.events.Pending()) != 0 {
		t.Fatal("expired event fanned out or was kept")
	}
}

func TestEnqueueWakesTheFanOutWorker(t *testing.T) {
	f := newFanOut(t)
	woken := 0
	f.svc.WithEvents(f.events, func() { woken++ })
	if err := f.svc.EnqueueChoreLogged(context.Background(), f.home, f.dishes.ID, 1, 1); err != nil || woken != 1 {
		t.Fatalf("woken %d times, err %v", woken, err)
	}
	unwired := NewService(NewMemoryStore())
	if err := unwired.EnqueueChoreLogged(context.Background(), f.home, f.dishes.ID, 1, 1); err != nil {
		t.Fatal(err)
	}
}

func TestPushGuardFollowsAccess(t *testing.T) {
	ctx := context.Background()
	f := newFanOut(t)
	guard := f.svc.PushGuard()
	job := push.Job{UserID: 2, HouseholdID: f.home, ChoreID: f.dishes.ID}
	ran := 0
	deliver := func() error { ran++; return nil }
	if err := guard(ctx, job, deliver); err != nil || ran != 1 {
		t.Fatalf("member's delivery: ran %d, err %v", ran, err)
	}

	f.dishes.Visibility = chore.VisibilityAdmins
	if err := f.chores.UpdateChore(ctx, f.dishes); err != nil {
		t.Fatal(err)
	}
	if err := guard(ctx, job, deliver); !errors.Is(err, push.ErrNoAccess) || ran != 1 {
		t.Fatalf("hidden activity: ran %d, err %v", ran, err)
	}
	if err := guard(ctx, push.Job{UserID: 2, HouseholdID: f.home}, deliver); err != nil || ran != 2 {
		t.Fatalf("household notification: ran %d, err %v", ran, err)
	}
	if err := f.hh.RemoveMember(ctx, f.home, 2); err != nil {
		t.Fatal(err)
	}
	if err := guard(ctx, push.Job{UserID: 2, HouseholdID: f.home}, deliver); !errors.Is(err, push.ErrNoAccess) || ran != 2 {
		t.Fatalf("former member: ran %d, err %v", ran, err)
	}
}
