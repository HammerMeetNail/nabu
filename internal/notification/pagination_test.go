package notification

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func TestNotificationCursorRejectsInvalidPositions(t *testing.T) {
	for _, raw := range []string{"!", strings.Repeat("a", 257), base64.RawURLEncoding.EncodeToString([]byte(`{"at":"2026-09-10T00:00:00Z","id":0}`)), base64.RawURLEncoding.EncodeToString([]byte(`{"at":"2026-09-10T00:00:00Z","id":1,"userId":99}`))} {
		if _, err := DecodeCursor(raw); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("invalid cursor accepted: %v", err)
		}
	}
}
func TestNotificationMemoryNewestFirstAndCursorStability(t *testing.T) { notificationPages(t, false) }
func TestNotificationPostgresCursorStability(t *testing.T)             { notificationPages(t, true) }
func notificationPages(t *testing.T, postgres bool) {
	ctx := context.Background()
	at := time.Date(2026, 9, 10, 12, 0, 0, 123000, time.UTC)
	var store Store
	if postgres {
		db := testdb.New(t)
		if err := database.Migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO users(id,email,password_hash,display_name) VALUES(1,'one@example.invalid','','Synthetic'),(2,'two@example.invalid','','Synthetic')`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO notifications(user_id,type,title,created_at,is_read) SELECT 1,'chore_logged','Synthetic '||n,$1,n>50 FROM generate_series(1,120)n`, at); err != nil {
			t.Fatal(err)
		}
		store = NewPostgresStore(db)
	} else {
		memory := NewMemoryStore()
		for n := int64(1); n <= 120; n++ {
			memory.notifs = append(memory.notifs, Notification{ID: n, UserID: 1, Type: "chore_logged", Title: fmt.Sprintf("Synthetic %d", n), CreatedAt: at, IsRead: n > 50})
		}
		memory.idSeq = 120
		store = memory
	}
	if _, err := store.CreateNotification(ctx, Notification{UserID: 2, Title: "FOREIGN-CANARY", Type: "chore_logged"}); err != nil {
		t.Fatal(err)
	}
	legacy, err := store.ListNotifications(ctx, 1, 1, 0)
	if err != nil || len(legacy) != 1 || legacy[0].ID != 120 {
		t.Fatalf("newest notification not first: %v %v", legacy, err)
	}
	svc := NewService(store)
	first, err := svc.ListPage(ctx, 1, "")
	if err != nil || len(first.Notifications) != 50 || first.NextCursor == "" || first.UnreadCount != 50 {
		t.Fatalf("first page: %d %d %v", len(first.Notifications), first.UnreadCount, err)
	}
	seen := map[int64]bool{}
	for _, n := range first.Notifications {
		if !n.IsRead {
			t.Fatal("first page should be entirely read")
		}
		seen[n.ID] = true
	}
	boundary := first.Notifications[49].ID
	if err := svc.Delete(ctx, boundary, 1); err != nil {
		t.Fatal(err)
	}
	newRow, err := store.CreateNotification(ctx, Notification{UserID: 1, Title: "new arrival", Type: "chore_logged"})
	if err != nil {
		t.Fatal(err)
	}
	cursor := first.NextCursor
	for pageNo := 0; cursor != ""; pageNo++ {
		if pageNo > 4 {
			t.Fatal("cursor loop")
		}
		page, err := svc.ListPage(ctx, 1, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Notifications) > PageSize {
			t.Fatal("unbounded page")
		}
		for _, n := range page.Notifications {
			if n.UserID != 1 || seen[n.ID] || n.ID == newRow.ID {
				t.Fatalf("foreign,duplicate,or shifted row: %d", n.ID)
			}
			seen[n.ID] = true
		}
		cursor = page.NextCursor
	}
	if len(seen) != 120 {
		t.Fatalf("page gap after insert/deleted cursor: %d", len(seen))
	}
	// The cursor is only a position; using it for another recipient reveals no
	// rows belonging to the first user, and mutations also require that user.
	other, err := svc.ListPage(ctx, 2, first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range other.Notifications {
		if n.UserID != 2 {
			t.Fatal("cross-user read")
		}
	}
	if err := svc.MarkRead(ctx, newRow.ID, 2); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, newRow.ID, 2); err != nil {
		t.Fatal(err)
	}
	latest, err := svc.ListPage(ctx, 1, "")
	if err != nil || latest.Notifications[0].ID != newRow.ID || latest.Notifications[0].IsRead {
		t.Fatal("cross-user mutation or newest ordering failure")
	}
	if err := svc.MarkAllRead(ctx, 1); err != nil {
		t.Fatal(err)
	}
	allRead, err := svc.ListPage(ctx, 1, "")
	if err != nil || allRead.UnreadCount != 0 || allRead.NextCursor == "" {
		t.Fatal("read status hid older pages")
	}
}
