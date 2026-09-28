package reminder

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/notification"
	"github.com/HammerMeetNail/nabu/internal/schedule"
	"github.com/HammerMeetNail/nabu/internal/userprefs"
)

// recordingSender counts delivered pushes so tests can assert that a quiet
// window suppressed delivery.
type recordingSender struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (r *recordingSender) SendPushToUser(_ context.Context, userID int64, title, body string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.calls = append(r.calls, fmt.Sprintf("%d:%s", userID, title))
	return nil
}

func (r *recordingSender) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// quietHoursHarness builds a fully wired in-memory scheduler whose only due
// schedule is a daily chore at now's wall-clock time (lead window therefore
// open), so a tick at `now` would deliver exactly one reminder unless a quiet
// window suppresses it.
func quietHoursHarness(t *testing.T, now time.Time) (*Scheduler, *recordingSender, *chore.MemoryStore, *notification.MemoryStore, *reminderTestPrefs) {
	t.Helper()
	remindStore := NewMemoryStore()
	schedStore := schedule.NewMemoryStore()
	choreStore := chore.NewMemoryStore()
	notifStore := notification.NewMemoryStore()
	userPrefsStore := userprefs.NewMemoryStore()

	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	user, _, err := authService.RegisterWithHash(context.Background(), "quiet@example.com", "$2a$10$abcdefghijklmnopqrstuv")
	if err != nil {
		t.Fatalf("RegisterWithHash: %v", err)
	}

	hhStore := household.NewMemoryStore()
	// CreateHousehold registers the owner membership itself.
	hh, err := hhStore.CreateHousehold(context.Background(), "Quiet Home", "", user.ID)
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	c, err := choreStore.CreateChore(context.Background(), chore.Chore{
		HouseholdID: hh.ID, Name: "Feed baby", Icon: "🍼", Color: "#2E86AB",
		Category: "custom", Visibility: chore.VisibilityHousehold, CreatedBy: &user.ID,
	})
	if err != nil {
		t.Fatalf("CreateChore: %v", err)
	}
	_, err = schedStore.Create(context.Background(), schedule.ChoreSchedule{
		HouseholdID: hh.ID, ChoreID: c.ID, FrequencyType: "daily",
		SpecificTime: now.Format("15:04"), TimePeriod: schedule.PeriodAnytime,
		DaysOfWeek: []int{}, IsActive: true,
	})
	if err != nil {
		t.Fatalf("schedule.Create: %v", err)
	}
	// Eligibility for unassigned schedules requires an enabled per-chore pref.
	if err := remindStore.UpdateChoreReminderPref(context.Background(), ChoreReminderPref{
		UserID: user.ID, ChoreID: c.ID, Enabled: true, LeadMinutes: 10,
	}); err != nil {
		t.Fatalf("UpdateChoreReminderPref: %v", err)
	}

	sender := &recordingSender{}
	s := NewScheduler(remindStore, schedStore, schedule.NewService(), notifStore, choreStore, hhStore, userPrefsStore, sender)
	s.now = func() time.Time { return now }
	return s, sender, choreStore, notifStore, &reminderTestPrefs{remindStore: remindStore, user: user, choreID: c.ID}
}

type reminderTestPrefs struct {
	remindStore *MemoryStore
	user        auth.User
	choreID     int64
}

// runTick runs one scheduler tick and reports (delivered, error).
func runTick(t *testing.T, s *Scheduler) {
	t.Helper()
	if err := s.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
}

// The schedule is due at every test clock below; only quiet windows differ.
func TestTickDeliversWithoutQuietHours(t *testing.T) {
	now := time.Date(2026, 9, 27, 23, 0, 0, 0, time.UTC)
	s, sender, _, _, _ := quietHoursHarness(t, now)
	runTick(t, s)
	if got := sender.count(); got != 1 {
		t.Fatalf("delivered = %d, want 1", got)
	}
}

// Household-level quiet hours on the chore suppress the reminder.
func TestTickSuppressedByChoreQuietHours(t *testing.T) {
	now := time.Date(2026, 9, 27, 23, 0, 0, 0, time.UTC) // inside 22:00–07:00
	s, sender, choreStore, _, _ := quietHoursHarness(t, now)
	c, err := choreStore.GetChore(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetChore: %v", err)
	}
	c.QuietHoursStart, c.QuietHoursEnd = "22:00", "07:00"
	if err := choreStore.UpdateChore(context.Background(), c); err != nil {
		t.Fatalf("UpdateChore: %v", err)
	}
	runTick(t, s)
	if got := sender.count(); got != 0 {
		t.Fatalf("delivered = %d, want 0 (chore quiet hours)", got)
	}
}

// A chore quiet window that does not cover the current time must not suppress.
func TestTickNotSuppressedByDistantChoreQuietHours(t *testing.T) {
	now := time.Date(2026, 9, 27, 23, 0, 0, 0, time.UTC) // outside 01:00–02:00
	s, sender, choreStore, _, _ := quietHoursHarness(t, now)
	c, err := choreStore.GetChore(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetChore: %v", err)
	}
	c.QuietHoursStart, c.QuietHoursEnd = "01:00", "02:00"
	if err := choreStore.UpdateChore(context.Background(), c); err != nil {
		t.Fatalf("UpdateChore: %v", err)
	}
	runTick(t, s)
	if got := sender.count(); got != 1 {
		t.Fatalf("delivered = %d, want 1 (window does not cover now)", got)
	}
}

// Per-chore, per-user quiet hours (chore_reminder_prefs) suppress the reminder.
func TestTickSuppressedByUserChoreQuietHours(t *testing.T) {
	now := time.Date(2026, 9, 27, 23, 0, 0, 0, time.UTC) // inside 22:00–07:00
	s, sender, _, _, prefs := quietHoursHarness(t, now)
	if err := prefs.remindStore.UpdateChoreReminderPref(context.Background(), ChoreReminderPref{
		UserID: prefs.user.ID, ChoreID: prefs.choreID, Enabled: true, LeadMinutes: 10,
		QuietHoursStart: "22:00", QuietHoursEnd: "07:00",
	}); err != nil {
		t.Fatalf("UpdateChoreReminderPref: %v", err)
	}
	runTick(t, s)
	if got := sender.count(); got != 0 {
		t.Fatalf("delivered = %d, want 0 (per-user chore quiet hours)", got)
	}
}

// The pre-existing global quiet hours still suppress (regression guard).
func TestTickSuppressedByGlobalQuietHours(t *testing.T) {
	now := time.Date(2026, 9, 27, 23, 0, 0, 0, time.UTC)
	s, sender, _, notifStore, prefs := quietHoursHarness(t, now)
	if err := notifStore.UpdateReminderPreferences(context.Background(), notification.ReminderPreference{
		UserID: prefs.user.ID, PushEnabled: true, Timezone: "UTC",
		QuietHoursStart: "22:00", QuietHoursEnd: "07:00", DefaultReminderLeadMinutes: 10,
	}); err != nil {
		t.Fatalf("UpdateReminderPreferences: %v", err)
	}
	runTick(t, s)
	if got := sender.count(); got != 0 {
		t.Fatalf("delivered = %d, want 0 (global quiet hours)", got)
	}
}

// A wrapping overnight chore window (22:00–07:00) suppresses at 03:00.
func TestTickSuppressedByOvernightChoreQuietHours(t *testing.T) {
	now := time.Date(2026, 9, 28, 3, 0, 0, 0, time.UTC) // inside 22:00–07:00
	s, sender, choreStore, _, _ := quietHoursHarness(t, now)
	c, err := choreStore.GetChore(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetChore: %v", err)
	}
	c.QuietHoursStart, c.QuietHoursEnd = "22:00", "07:00"
	if err := choreStore.UpdateChore(context.Background(), c); err != nil {
		t.Fatalf("UpdateChore: %v", err)
	}
	runTick(t, s)
	if got := sender.count(); got != 0 {
		t.Fatalf("delivered = %d, want 0 (overnight window)", got)
	}
}
