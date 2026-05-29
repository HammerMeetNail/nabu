package push

import (
	"context"
	"testing"
)

func TestMemoryStore_SaveAndGet(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	sub := Subscription{Endpoint: "https://push.example.com/1", P256DH: "key1", Auth: "auth1"}
	if err := store.SaveSubscription(ctx, 1, sub); err != nil {
		t.Fatalf("SaveSubscription: %v", err)
	}

	subs, err := store.GetSubscriptions(ctx, 1)
	if err != nil {
		t.Fatalf("GetSubscriptions: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("got %d subscriptions, want 1", len(subs))
	}
	if subs[0].Endpoint != sub.Endpoint {
		t.Errorf("endpoint = %q, want %q", subs[0].Endpoint, sub.Endpoint)
	}
}

func TestMemoryStore_SaveUpdatesExisting(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	sub := Subscription{Endpoint: "https://push.example.com/1", P256DH: "key1", Auth: "auth1"}
	store.SaveSubscription(ctx, 1, sub)

	// Update with same endpoint but new keys
	sub2 := Subscription{Endpoint: "https://push.example.com/1", P256DH: "key2", Auth: "auth2"}
	store.SaveSubscription(ctx, 1, sub2)

	subs, _ := store.GetSubscriptions(ctx, 1)
	if len(subs) != 1 {
		t.Fatalf("got %d subscriptions after update, want 1", len(subs))
	}
	if subs[0].P256DH != "key2" {
		t.Errorf("P256DH = %q, want key2", subs[0].P256DH)
	}
}

func TestMemoryStore_GetEmpty(t *testing.T) {
	store := NewMemoryStore()
	subs, err := store.GetSubscriptions(context.Background(), 99)
	if err != nil {
		t.Fatalf("GetSubscriptions: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("got %d subscriptions for unknown user, want 0", len(subs))
	}
}

func TestMemoryStore_DeleteSubscription(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	store.SaveSubscription(ctx, 1, Subscription{Endpoint: "https://a.example.com", P256DH: "k", Auth: "a"})
	store.SaveSubscription(ctx, 1, Subscription{Endpoint: "https://b.example.com", P256DH: "k2", Auth: "a2"})

	if err := store.DeleteSubscription(ctx, 1, "https://a.example.com"); err != nil {
		t.Fatalf("DeleteSubscription: %v", err)
	}

	subs, _ := store.GetSubscriptions(ctx, 1)
	if len(subs) != 1 {
		t.Fatalf("got %d subscriptions after delete, want 1", len(subs))
	}
	if subs[0].Endpoint != "https://b.example.com" {
		t.Errorf("remaining endpoint = %q, want https://b.example.com", subs[0].Endpoint)
	}
}

func TestMemoryStore_DeleteNonExistentEndpoint(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	store.SaveSubscription(ctx, 1, Subscription{Endpoint: "https://a.example.com", P256DH: "k", Auth: "a"})

	// Deleting a non-existent endpoint should not error and should not remove existing ones
	if err := store.DeleteSubscription(ctx, 1, "https://nonexistent.example.com"); err != nil {
		t.Fatalf("DeleteSubscription: %v", err)
	}
	subs, _ := store.GetSubscriptions(ctx, 1)
	if len(subs) != 1 {
		t.Errorf("got %d subscriptions, want 1", len(subs))
	}
}

func TestMemoryStore_MultipleUsers(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	store.SaveSubscription(ctx, 1, Subscription{Endpoint: "https://user1.example.com", P256DH: "k1", Auth: "a1"})
	store.SaveSubscription(ctx, 2, Subscription{Endpoint: "https://user2.example.com", P256DH: "k2", Auth: "a2"})

	subs1, _ := store.GetSubscriptions(ctx, 1)
	subs2, _ := store.GetSubscriptions(ctx, 2)

	if len(subs1) != 1 || subs1[0].Endpoint != "https://user1.example.com" {
		t.Errorf("user 1 subs: %+v", subs1)
	}
	if len(subs2) != 1 || subs2[0].Endpoint != "https://user2.example.com" {
		t.Errorf("user 2 subs: %+v", subs2)
	}
}
