package reminder

import (
	"context"
	"testing"
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
	store := NewMemoryStore()
	ctx := context.Background()

	exists, err := store.HasReminder(ctx, 1, 100, "2025-01-01")
	if err != nil {
		t.Fatalf("HasReminder: %v", err)
	}
	if exists {
		t.Error("expected reminder not to exist yet")
	}

	err = store.RecordReminder(ctx, 1, 100, "2025-01-01")
	if err != nil {
		t.Fatalf("RecordReminder: %v", err)
	}

	exists, err = store.HasReminder(ctx, 1, 100, "2025-01-01")
	if err != nil {
		t.Fatalf("HasReminder after record: %v", err)
	}
	if !exists {
		t.Error("expected reminder to exist after record")
	}

	exists, err = store.HasReminder(ctx, 1, 100, "2025-01-02")
	if err != nil {
		t.Fatalf("HasReminder different date: %v", err)
	}
	if exists {
		t.Error("expected reminder not to exist for different date")
	}

	exists, err = store.HasReminder(ctx, 2, 100, "2025-01-01")
	if err != nil {
		t.Fatalf("HasReminder different user: %v", err)
	}
	if exists {
		t.Error("expected reminder not to exist for different user")
	}
}

func TestMemoryStore_PurgeOldReminders(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	store.RecordReminder(ctx, 1, 100, "2020-01-01")
	_ = store.RecordReminder(ctx, 1, 100, "2099-01-01")

	n, err := store.PurgeOldReminders(ctx)
	if err != nil {
		t.Fatalf("PurgeOldReminders: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 purged, got %d", n)
	}

	exists, _ := store.HasReminder(ctx, 1, 100, "2020-01-01")
	if exists {
		t.Error("old reminder should have been purged")
	}

	exists, _ = store.HasReminder(ctx, 1, 100, "2099-01-01")
	if !exists {
		t.Error("future reminder should remain")
	}
}
