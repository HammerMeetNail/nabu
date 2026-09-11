package reminder

import (
	"context"
	"github.com/HammerMeetNail/nabu/internal/testsync"
	"sync"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/notification"
	"github.com/HammerMeetNail/nabu/internal/schedule"
	"github.com/HammerMeetNail/nabu/internal/userprefs"
)

type countPush struct{ calls int }

func (p *countPush) SendPushToUser(context.Context, int64, string, string) error {
	p.calls++
	return nil
}

type pausedDedup struct {
	Store
	entered, resume chan struct{}
	once            sync.Once
}

func (s *pausedDedup) HasReminder(ctx context.Context, sch, uid int64, date string) (bool, error) {
	s.once.Do(func() { close(s.entered); testsync.Pause(ctx, s.resume) })
	return s.Store.HasReminder(ctx, sch, uid, date)
}

func reminderFixture(t *testing.T, now time.Time, zone string) (*Scheduler, *household.MemoryStore, *countPush, schedule.ChoreSchedule) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	hh := household.NewMemoryStore()
	home, err := hh.CreateHousehold(ctx, "Test", "T", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := hh.AddMember(ctx, home.ID, 2, household.RoleMember); err != nil {
		t.Fatal(err)
	}
	chores := chore.NewMemoryStore()
	ch, err := chores.CreateChore(ctx, chore.Chore{HouseholdID: home.ID, Name: "Private details in a shared chore", Visibility: chore.VisibilityHousehold})
	if err != nil {
		t.Fatal(err)
	}
	schedules := schedule.NewMemoryStore()
	uid := int64(2)
	sch, err := schedules.Create(ctx, schedule.ChoreSchedule{HouseholdID: home.ID, ChoreID: ch.ID, AssignedUserID: &uid, IsActive: true, FrequencyType: "daily", SpecificTime: now.Format("15:04")})
	if err != nil {
		t.Fatal(err)
	}
	prefs := notification.NewMemoryStore()
	if err := prefs.UpdateReminderPreferences(ctx, notification.ReminderPreference{UserID: uid, PushEnabled: true, Timezone: zone}); err != nil {
		t.Fatal(err)
	}
	push := &countPush{}
	s := NewScheduler(NewMemoryStore(), schedules, schedule.NewService(), prefs, chores, hh, userprefs.NewMemoryStore(), push)
	s.now = func() time.Time { return now }
	return s, hh, push, sch
}

func TestRemovedSharedAssignmentNeverReminds(t *testing.T) {
	for _, race := range []bool{false, true} {
		t.Run(map[bool]string{false: "already removed", true: "removed during tick"}[race], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			now := time.Date(2026, 9, 6, 20, 30, 0, 0, time.UTC)
			s, hh, push, sch := reminderFixture(t, now, "UTC")
			if race {
				barrier := &pausedDedup{Store: s.store, entered: make(chan struct{}), resume: make(chan struct{})}
				s.store = barrier
				done := make(chan error, 1)
				go func() { done <- s.tick(ctx) }()
				testsync.Receive(t, ctx, barrier.entered)
				if err := hh.RemoveMember(ctx, sch.HouseholdID, 2); err != nil {
					t.Fatal(err)
				}
				close(barrier.resume)
				if err := testsync.Receive(t, ctx, done); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := hh.RemoveMember(ctx, sch.HouseholdID, 2); err != nil {
					t.Fatal(err)
				}
				if err := s.tick(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if push.calls != 0 {
				t.Fatal("former member received shared chore reminder")
			}
		})
	}
}

func TestRecipientLocalRecurrence(t *testing.T) {
	for _, tc := range []struct {
		zone, instant string
		weekday       int
		want          int
	}{
		{"America/New_York", "2026-09-07T00:30:00Z", 0, 1},
		{"America/New_York", "2026-09-07T00:30:00Z", 1, 0},
		{"Asia/Tokyo", "2026-09-09T23:30:00Z", 4, 1},
		{"Asia/Tokyo", "2026-09-09T23:30:00Z", 3, 0},
	} {
		t.Run(tc.zone+time.Weekday(tc.weekday).String(), func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Fatal(err)
			}
			instant, err := time.Parse(time.RFC3339, tc.instant)
			if err != nil {
				t.Fatal(err)
			}
			s, _, push, sch := reminderFixture(t, instant.In(loc), tc.zone)
			sch.FrequencyType = "weekly"
			sch.DaysOfWeek = []int{tc.weekday}
			if _, err := s.schedStore.Update(context.Background(), sch); err != nil {
				t.Fatal(err)
			}
			if err := s.tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			if push.calls != tc.want {
				t.Fatalf("calls=%d want=%d", push.calls, tc.want)
			}
		})
	}
}

func TestLeadWindowUsesNextDaysRecurrence(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	s := &Scheduler{schedSvc: schedule.NewService()}
	now := time.Date(2026, 9, 6, 23, 55, 0, 0, loc)
	sch := schedule.ChoreSchedule{IsActive: true, FrequencyType: "weekly", DaysOfWeek: []int{1}, SpecificTime: "00:05"}
	day, due := s.dueDate(sch, now, 10)
	if !due || day.Format("2006-01-02") != "2026-09-07" {
		t.Fatalf("next-day lead window %v %v", day, due)
	}
}
