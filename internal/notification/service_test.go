package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/notification"
)

func newSvc() *notification.Service {
	return notification.NewService(notification.NewMemoryStore())
}

// ─── List / UnreadCount ───────────────────────────────────────────────────────

func TestList_Empty(t *testing.T) {
	svc := newSvc()
	notifs, unread, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(notifs) != 0 || unread != 0 {
		t.Errorf("expected empty list, got %d notifs / %d unread", len(notifs), unread)
	}
}

func TestUnreadCount_AfterCreateAndMarkRead(t *testing.T) {
	store := notification.NewMemoryStore()
	svc := notification.NewService(store)
	ctx := context.Background()

	// Create two notifications for user 1 directly via the store
	_, _ = store.CreateNotification(ctx, notification.Notification{UserID: 1, Type: "chore_logged", Title: "T1", Body: "B1"})
	n2, _ := store.CreateNotification(ctx, notification.Notification{UserID: 1, Type: "chore_logged", Title: "T2", Body: "B2"})

	count, err := svc.UnreadCount(ctx, 1)
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	// Mark one read
	if err := svc.MarkRead(ctx, n2.ID, 1); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	count, _ = svc.UnreadCount(ctx, 1)
	if count != 1 {
		t.Errorf("count after MarkRead = %d, want 1", count)
	}
}

func TestMarkAllRead(t *testing.T) {
	store := notification.NewMemoryStore()
	svc := notification.NewService(store)
	ctx := context.Background()

	_, _ = store.CreateNotification(ctx, notification.Notification{UserID: 1, Type: "chore_logged", Title: "T1"})
	_, _ = store.CreateNotification(ctx, notification.Notification{UserID: 1, Type: "chore_logged", Title: "T2"})

	if err := svc.MarkAllRead(ctx, 1); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	count, _ := svc.UnreadCount(ctx, 1)
	if count != 0 {
		t.Errorf("count after MarkAllRead = %d, want 0", count)
	}
}

func TestDelete_RemovesNotification(t *testing.T) {
	store := notification.NewMemoryStore()
	svc := notification.NewService(store)
	ctx := context.Background()

	n, _ := store.CreateNotification(ctx, notification.Notification{UserID: 1, Type: "chore_logged", Title: "T"})
	if err := svc.Delete(ctx, n.ID, 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	notifs, _, _ := svc.List(ctx, 1)
	if len(notifs) != 0 {
		t.Errorf("expected 0 notifs after delete, got %d", len(notifs))
	}
}

// ─── Preferences ─────────────────────────────────────────────────────────────

func TestGetNotificationPreferences_Defaults(t *testing.T) {
	svc := newSvc()
	prefs, err := svc.GetNotificationPreferences(context.Background(), 99)
	if err != nil {
		t.Fatalf("GetNotificationPreferences: %v", err)
	}
	// Default should have push enabled
	if !prefs.PushEnabled {
		t.Error("default prefs should have PushEnabled=true")
	}
	if prefs.UserID != 99 {
		t.Errorf("UserID = %d, want 99", prefs.UserID)
	}
}

func TestUpdateNotificationPreferences(t *testing.T) {
	svc := newSvc()
	ctx := context.Background()

	p := notification.ReminderPreference{
		UserID:           1,
		PushEnabled:      false,
		EmailEnabled:     true,
		Timezone:         "America/New_York",
		EnabledPushTypes: []string{"chore_logged"},
	}
	if err := svc.UpdateNotificationPreferences(ctx, p); err != nil {
		t.Fatalf("UpdateNotificationPreferences: %v", err)
	}
	got, _ := svc.GetNotificationPreferences(ctx, 1)
	if got.PushEnabled {
		t.Error("PushEnabled should be false")
	}
	if got.Timezone != "America/New_York" {
		t.Errorf("Timezone = %q", got.Timezone)
	}
}

// ─── AvailableNotificationTypes ───────────────────────────────────────────────

func TestAvailableNotificationTypes(t *testing.T) {
	types := notification.AvailableNotificationTypes()
	if len(types) == 0 {
		t.Fatal("expected at least one notification type")
	}
	for _, ti := range types {
		if ti.Type == "" || ti.Label == "" {
			t.Errorf("incomplete type info: %+v", ti)
		}
	}
}

// ─── MemoryStore direct tests ─────────────────────────────────────────────────

func TestMemoryStore_MarkRead_NoMatch(t *testing.T) {
	store := notification.NewMemoryStore()
	// No notifications exist – MarkRead should return nil (loop exits without match)
	err := store.MarkRead(context.Background(), 999, 1)
	if err != nil {
		t.Errorf("MarkRead with no match: got %v, want nil", err)
	}
}

func TestMemoryStore_DeleteNotification_NoMatch(t *testing.T) {
	store := notification.NewMemoryStore()
	// No notifications exist – DeleteNotification should return nil
	err := store.DeleteNotification(context.Background(), 999, 1)
	if err != nil {
		t.Errorf("DeleteNotification with no match: got %v, want nil", err)
	}
}

func TestMemoryStore_ListNotifications_OffsetBeyondTotal(t *testing.T) {
	store := notification.NewMemoryStore()
	ctx := context.Background()
	// Create only 1 notification; offset 5 > len → start clamped to 1
	_, _ = store.CreateNotification(ctx, notification.Notification{UserID: 1, Type: "chore_logged", Title: "T"})
	result, err := store.ListNotifications(ctx, 1, 10, 5)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 results with offset beyond total, got %d", len(result))
	}
}

func TestList_PaginationLimit(t *testing.T) {
	store := notification.NewMemoryStore()
	svc := notification.NewService(store)
	ctx := context.Background()

	for i := 0; i < 60; i++ {
		_, _ = store.CreateNotification(ctx, notification.Notification{
			UserID:    1,
			Type:      "chore_logged",
			Title:     "T",
			CreatedAt: time.Now(),
		})
	}

	notifs, _, err := svc.List(ctx, 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(notifs) != 50 {
		t.Errorf("expected 50 (limit), got %d", len(notifs))
	}
}
