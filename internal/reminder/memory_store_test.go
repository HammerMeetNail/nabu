package reminder

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/push"
)

func TestMemoryStore_ChoreReminderPrefs(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	got, err := store.GetChoreReminderPref(ctx, 1, 10)
	if err != nil {
		t.Fatalf("GetChoreReminderPref: %v", err)
	}
	if got.Enabled {
		t.Error("expected disabled by default")
	}
	if got.LeadMinutes != 10 {
		t.Errorf("LeadMinutes = %d, want 10", got.LeadMinutes)
	}

	err = store.UpdateChoreReminderPref(ctx, ChoreReminderPref{
		UserID: 1, ChoreID: 10, Enabled: true, LeadMinutes: 15,
	})
	if err != nil {
		t.Fatalf("UpdateChoreReminderPref: %v", err)
	}

	got, err = store.GetChoreReminderPref(ctx, 1, 10)
	if err != nil {
		t.Fatalf("GetChoreReminderPref after update: %v", err)
	}
	if !got.Enabled {
		t.Error("expected enabled after update")
	}
	if got.LeadMinutes != 15 {
		t.Errorf("LeadMinutes = %d, want 15", got.LeadMinutes)
	}

	prefs, err := store.GetChoreReminderPrefs(ctx, 1)
	if err != nil {
		t.Fatalf("GetChoreReminderPrefs: %v", err)
	}
	if len(prefs) != 1 {
		t.Errorf("expected 1 pref for user 1, got %d", len(prefs))
	}
}

func TestMemoryStore_ReminderDedup(t *testing.T) {
	jobs := push.NewMemoryJobStore()
	store := NewMemoryStore().WithJobs(jobs)
	ctx := context.Background()
	claim := func(scheduleID, userID int64, date string) bool {
		t.Helper()
		claimed, err := store.ClaimReminder(ctx, scheduleID, userID, date, push.Job{UserID: userID, HouseholdID: 1,
			DedupeKey: fmt.Sprintf("%d:%d:%s", scheduleID, userID, date), ExpiresAt: time.Now().Add(time.Hour)})
		if err != nil {
			t.Fatalf("ClaimReminder: %v", err)
		}
		return claimed
	}

	if !claim(1, 100, "2025-01-01") {
		t.Error("first claim refused")
	}
	if claim(1, 100, "2025-01-01") {
		t.Error("second claim of the same reminder accepted")
	}
	if !claim(1, 100, "2025-01-02") {
		t.Error("claim for a different date refused")
	}
	if !claim(2, 100, "2025-01-01") {
		t.Error("claim for a different schedule refused")
	}
	if got := len(jobs.Pending()); got != 3 {
		t.Errorf("queued %d pushes, want one per claimed reminder", got)
	}
}

// failingJobs refuses every job, as an unavailable database would.
type failingJobs struct{ push.JobStore }

func (failingJobs) EnqueuePush(context.Context, push.Job) error {
	return errors.New("database unavailable")
}

func TestMemoryStore_FailedQueueLeavesReminderUnclaimed(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore().WithJobs(failingJobs{})
	if claimed, err := store.ClaimReminder(ctx, 1, 100, "2025-01-01", push.Job{}); err == nil || claimed {
		t.Fatalf("claimed=%v err=%v", claimed, err)
	}
	store.WithJobs(push.NewMemoryJobStore())
	if claimed, err := store.ClaimReminder(ctx, 1, 100, "2025-01-01", push.Job{ExpiresAt: time.Now().Add(time.Hour)}); err != nil || !claimed {
		t.Fatalf("retry claimed=%v err=%v", claimed, err)
	}
}

func TestMemoryStore_PurgeOldReminders(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	claim := func(date string) bool {
		t.Helper()
		claimed, err := store.ClaimReminder(ctx, 1, 100, date, push.Job{})
		if err != nil {
			t.Fatal(err)
		}
		return claimed
	}

	claim("2020-01-01")
	claim("2099-01-01")

	n, err := store.PurgeOldReminders(ctx)
	if err != nil {
		t.Fatalf("PurgeOldReminders: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 purged, got %d", n)
	}
	if !claim("2020-01-01") {
		t.Error("old reminder should have been purged")
	}
	if claim("2099-01-01") {
		t.Error("future reminder should remain")
	}
}
