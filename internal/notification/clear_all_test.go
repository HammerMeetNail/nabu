package notification

import (
	"context"
	"errors"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func TestClearAllNotifications(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			var store Store = NewMemoryStore()
			if backend == "postgres" {
				db := testdb.New(t)
				if err := database.Migrate(ctx, db); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO users(id,email,password_hash,display_name) VALUES(1,'one@example.invalid','','One'),(2,'two@example.invalid','','Two')`); err != nil {
					t.Fatal(err)
				}
				store = NewPostgresStore(db)
			}
			for i := 0; i < 120; i++ {
				n, err := store.CreateNotification(ctx, Notification{UserID: 1, Type: "chore_logged", Title: "History"})
				if err != nil {
					t.Fatal(err)
				}
				if i%2 == 0 {
					if err := store.MarkRead(ctx, n.ID, 1); err != nil {
						t.Fatal(err)
					}
				}
			}
			other, err := store.CreateNotification(ctx, Notification{UserID: 2, Type: "chore_logged", Title: "Other recipient"})
			if err != nil {
				t.Fatal(err)
			}
			service := NewService(store)
			first, err := service.ListPage(ctx, 1, "")
			if err != nil || first.NextCursor == "" || first.UnreadCount != 60 {
				t.Fatalf("setup page: %+v %v", first, err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				if err := service.ClearAll(ctx, 1); err != nil {
					t.Fatal(err)
				}
				for _, cursor := range []string{"", first.NextCursor} {
					page, err := service.ListPage(ctx, 1, cursor)
					if err != nil || len(page.Notifications) != 0 || page.UnreadCount != 0 || page.NextCursor != "" {
						t.Fatalf("history survived clear: %+v %v", page, err)
					}
				}
			}
			foreign, err := service.ListPage(ctx, 2, "")
			if err != nil || len(foreign.Notifications) != 1 || foreign.Notifications[0].ID != other.ID || foreign.UnreadCount != 1 {
				t.Fatalf("another recipient changed: %+v %v", foreign, err)
			}
			if _, err := store.CreateNotification(ctx, Notification{UserID: 1, Type: "chore_logged", Title: "New arrival"}); err != nil {
				t.Fatal(err)
			}
			fresh, err := service.ListPage(ctx, 1, "")
			if err != nil || len(fresh.Notifications) != 1 || fresh.UnreadCount != 1 {
				t.Fatalf("new arrival suppressed: %+v %v", fresh, err)
			}
		})
	}
}

func TestClearAllNotificationsCanceled(t *testing.T) {
	store := NewMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := store.CreateNotification(ctx, Notification{UserID: 1}); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := NewService(store).ClearAll(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	rows, err := store.ListNotifications(context.Background(), 1, 50, 0)
	if err != nil || len(rows) != 1 {
		t.Fatal("canceled clear deleted notifications")
	}
}
