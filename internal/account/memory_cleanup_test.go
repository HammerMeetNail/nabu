package account

import (
	"context"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/apns"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/daynote"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/notification"
	"github.com/HammerMeetNail/nabu/internal/push"
	"github.com/HammerMeetNail/nabu/internal/reminder"
	"github.com/HammerMeetNail/nabu/internal/schedule"
	"github.com/HammerMeetNail/nabu/internal/userprefs"
)

func TestMemoryDeletionCleansDependentDataAndRejectsStaleWrites(t *testing.T) {
	svc, users, households := setup(t)
	ctx := context.Background()
	deleting := mustCreateUser(t, users, "delete@example.invalid")
	other := mustCreateUser(t, users, "keep@example.invalid")
	solo, err := households.CreateHousehold(ctx, "Solo", "S", deleting.ID)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := households.CreateHousehold(ctx, "Shared", "S", other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := households.AddMember(ctx, shared.ID, deleting.ID, household.RoleMember); err != nil {
		t.Fatal(err)
	}
	chores := chore.NewMemoryStore()
	logs := chorelog.NewMemoryStore()
	schedules := schedule.NewMemoryStore()
	reminders := reminder.NewMemoryStore()
	notifications := notification.NewMemoryStore()
	pushes := push.NewMemoryStore()
	devices := apns.NewMemoryStore()
	prefs := userprefs.NewMemoryStore()
	notes := daynote.NewMemoryStore()
	svc.SetMemoryCleanup(func(uid int64, hhs []int64) {
		d := lifecycle.NewDeletion(uid, hhs)
		for _, s := range []any{chores, logs, schedules, reminders, notifications, pushes, devices, prefs, notes} {
			s.(interface{ CleanupAccount(*lifecycle.Deletion) }).CleanupAccount(d)
		}
	})
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	ownChore, err := chores.CreateChore(ctx, chore.Chore{HouseholdID: solo.ID, Name: "Solo", CreatedBy: &deleting.ID})
	must(err)
	retained, err := chores.CreateChore(ctx, chore.Chore{HouseholdID: shared.ID, Name: "Shared private", Visibility: chore.VisibilityAdmins, CreatedBy: &deleting.ID})
	must(err)
	entry, err := logs.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: shared.ID, ChoreID: retained.ID, UserID: deleting.ID, Note: "Delete personal log"})
	must(err)
	_, err = logs.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: shared.ID, ChoreID: retained.ID, UserID: other.ID})
	must(err)
	sch, err := schedules.Create(ctx, schedule.ChoreSchedule{HouseholdID: shared.ID, ChoreID: retained.ID, AssignedUserID: &deleting.ID})
	must(err)
	_, err = schedules.Create(ctx, schedule.ChoreSchedule{HouseholdID: solo.ID, ChoreID: ownChore.ID})
	must(err)
	must(reminders.UpdateChoreReminderPref(ctx, reminder.ChoreReminderPref{UserID: deleting.ID, ChoreID: retained.ID, Enabled: true}))
	must(reminders.RecordReminder(ctx, sch.ID, deleting.ID, "2026-09-10"))
	_, err = notifications.CreateNotification(ctx, notification.Notification{UserID: deleting.ID, Body: "Delete personal notification"})
	must(err)
	must(notifications.UpdateReminderPreferences(ctx, notification.ReminderPreference{UserID: deleting.ID, Timezone: "Asia/Tokyo"}))
	sub := push.Subscription{Endpoint: "https://fcm.googleapis.com/synthetic"}
	must(pushes.SaveSubscription(ctx, deleting.ID, sub))
	device := apns.Device{UserID: deleting.ID, Token: "synthetic"}
	must(devices.RegisterDevice(ctx, device))
	must(prefs.Upsert(ctx, deleting.ID, userprefs.Preferences{Timezone: "Asia/Tokyo"}))
	_, err = notes.Upsert(ctx, solo.ID, "2026-09-10", "Delete sole household note", deleting.ID)
	must(err)
	_, err = notes.Upsert(ctx, shared.ID, "2026-09-10", "Retain household note", deleting.ID)
	must(err)
	must(svc.DeleteAccount(ctx, deleting.ID))
	if _, err := chores.GetChore(ctx, ownChore.ID); err == nil {
		t.Fatal("sole-household chore remains")
	}
	kept, err := chores.GetChore(ctx, retained.ID)
	must(err)
	if kept.CreatedBy != nil || kept.Visibility != chore.VisibilityAdmins {
		t.Fatal("retained private chore ownership/visibility changed")
	}
	if _, err := logs.GetLog(ctx, entry.ID); err == nil {
		t.Fatal("personal log remains")
	}
	if entries, err := logs.ListLogsRange(ctx, shared.ID, time.Time{}, time.Now().AddDate(1, 0, 0)); err != nil || len(entries) != 1 {
		t.Fatalf("remaining member's log lost: %d %v", len(entries), err)
	}
	keptSchedule, err := schedules.Get(ctx, sch.ID)
	must(err)
	if keptSchedule.AssignedUserID != nil {
		t.Fatal("deleted-user assignment remains")
	}
	if p, err := reminders.GetChoreReminderPrefs(ctx, deleting.ID); err != nil || len(p) != 0 {
		t.Fatal("personal reminder preferences remain")
	}
	if n, err := notifications.ListNotifications(ctx, deleting.ID, 50, 0); err != nil || len(n) != 0 {
		t.Fatal("personal notifications remain")
	}
	if subs, err := pushes.GetSubscriptions(ctx, deleting.ID); err != nil || len(subs) != 0 {
		t.Fatal("browser subscription remains")
	}
	if ds, err := devices.DevicesForUser(ctx, deleting.ID); err != nil || len(ds) != 0 {
		t.Fatal("APNs device remains")
	}
	if p, err := prefs.Get(ctx, deleting.ID); err != nil || p.Timezone != "" {
		t.Fatal("personal preferences remain")
	}
	if ns, err := notes.ListRange(ctx, solo.ID, time.Time{}, time.Now().AddDate(1, 0, 0)); err != nil || len(ns) != 0 {
		t.Fatal("sole-household notes remain")
	}
	checks := map[string]func() error{
		"chore": func() error {
			_, err := chores.CreateChore(ctx, chore.Chore{HouseholdID: solo.ID, Name: "stale"})
			return err
		},
		"log":      func() error { _, err := logs.CreateLog(ctx, entry); return err },
		"schedule": func() error { _, err := schedules.Update(ctx, sch); return err },
		"log effects": func() error {
			_, err := schedules.ApplyLogEffects(ctx, schedule.LogEffects{LogID: entry.ID, HouseholdID: shared.ID, ChoreID: retained.ID})
			return err
		},
		"reminder": func() error {
			return reminders.UpdateChoreReminderPref(ctx, reminder.ChoreReminderPref{UserID: deleting.ID, ChoreID: retained.ID})
		},
		"notification": func() error {
			_, err := notifications.CreateNotification(ctx, notification.Notification{UserID: deleting.ID})
			return err
		},
		"push":        func() error { return pushes.SaveSubscription(ctx, deleting.ID, sub) },
		"device":      func() error { return devices.RegisterDevice(ctx, device) },
		"preferences": func() error { return prefs.Upsert(ctx, deleting.ID, userprefs.Preferences{}) },
		"day note":    func() error { _, err := notes.Upsert(ctx, shared.ID, "2026-09-10", "stale", deleting.ID); return err },
	}
	for name, write := range checks {
		t.Run(name, func(t *testing.T) {
			if err := write(); err == nil {
				t.Fatal("stale request recreated deleted data")
			}
		})
	}
}
