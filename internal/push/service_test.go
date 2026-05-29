package push

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// validP256DH and validAuth are real P-256 test keys from encrypt_test.go.
const (
	validP256DH = "BFiD30jh-xT1-ztVT4-JzRZUkVaC3jSJpXSpsu8uy1q86f28QIg8W2iznxdqLqdlg7nYlVru_A1FjmmTmrV31Eo"
	validAuth   = "mfbNpuEjYa06PwSg5azFcw"
)

func TestNewService(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}

func TestSendPushToUser_NoSigner(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil) // signer = nil means push disabled

	ctx := context.Background()
	// Save a subscription to ensure the no-signer short circuit runs
	_ = store.SaveSubscription(ctx, 1, Subscription{Endpoint: "https://push.example.com", P256DH: "k", Auth: "a"})

	if err := svc.SendPushToUser(ctx, 1, "Hello", "World"); err != nil {
		t.Fatalf("SendPushToUser with nil signer: %v", err)
	}
}

func TestSendPushToUser_NoSubscriptions(t *testing.T) {
	store := NewMemoryStore()
	priv, pub, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	signer, err := NewVAPIDSigner(priv, pub, "mailto:test@example.com")
	if err != nil {
		t.Fatalf("NewVAPIDSigner: %v", err)
	}
	svc := NewService(store, signer)

	// User 99 has no subscriptions — should log and return nil
	if err := svc.SendPushToUser(context.Background(), 99, "title", "body"); err != nil {
		t.Fatalf("SendPushToUser: %v", err)
	}
}

// TestSendPushToUser_SuccessfulSend exercises the full send path (lines 44–82)
// using a valid P-256 subscription key and a test HTTP server that returns 201.
func TestSendPushToUser_SuccessfulSend(t *testing.T) {
	priv, pub, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	signer, err := NewVAPIDSigner(priv, pub, "mailto:test@example.com")
	if err != nil {
		t.Fatalf("NewVAPIDSigner: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated) // 201 — standard Web Push response
	}))
	defer ts.Close()

	store := NewMemoryStore()
	ctx := context.Background()
	_ = store.SaveSubscription(ctx, 2, Subscription{
		Endpoint: ts.URL,
		P256DH:   validP256DH,
		Auth:     validAuth,
	})

	svc := NewService(store, signer)
	if err := svc.SendPushToUser(ctx, 2, "title", "body"); err != nil {
		t.Fatalf("SendPushToUser: %v", err)
	}
}

// TestSendPushToUser_InvalidEndpoint covers the http.NewRequestWithContext error
// path (lines 60–63): the endpoint URL is syntactically invalid so the request
// cannot be constructed, but SendPushToUser must still return nil.
func TestSendPushToUser_InvalidEndpoint(t *testing.T) {
	priv, pub, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	signer, err := NewVAPIDSigner(priv, pub, "mailto:test@example.com")
	if err != nil {
		t.Fatalf("NewVAPIDSigner: %v", err)
	}

	store := NewMemoryStore()
	ctx := context.Background()
	_ = store.SaveSubscription(ctx, 4, Subscription{
		Endpoint: "://invalid-url", // will fail NewRequestWithContext
		P256DH:   validP256DH,
		Auth:     validAuth,
	})

	svc := NewService(store, signer)
	if err := svc.SendPushToUser(ctx, 4, "title", "body"); err != nil {
		t.Fatalf("SendPushToUser with invalid endpoint: %v", err)
	}
}

// TestSendPushToUser_NetworkError covers the client.Do error path (lines 70–72):
// the test server is closed before the request is made.
func TestSendPushToUser_NetworkError(t *testing.T) {
	priv, pub, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	signer, err := NewVAPIDSigner(priv, pub, "mailto:test@example.com")
	if err != nil {
		t.Fatalf("NewVAPIDSigner: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	// Close the server before the push is sent so client.Do fails.
	ts.Close()

	store := NewMemoryStore()
	ctx := context.Background()
	_ = store.SaveSubscription(ctx, 5, Subscription{
		Endpoint: ts.URL,
		P256DH:   validP256DH,
		Auth:     validAuth,
	})

	svc := NewService(store, signer)
	if err := svc.SendPushToUser(ctx, 5, "title", "body"); err != nil {
		t.Fatalf("SendPushToUser with closed server: %v", err)
	}
}
func TestSendPushToUser_StaleEndpointCleanup(t *testing.T) {
	priv, pub, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	signer, err := NewVAPIDSigner(priv, pub, "mailto:test@example.com")
	if err != nil {
		t.Fatalf("NewVAPIDSigner: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone) // 410 — stale subscription
	}))
	defer ts.Close()

	store := NewMemoryStore()
	ctx := context.Background()
	_ = store.SaveSubscription(ctx, 1, Subscription{
		Endpoint: ts.URL,
		P256DH:   validP256DH,
		Auth:     validAuth,
	})

	svc := NewService(store, signer)
	if err := svc.SendPushToUser(ctx, 1, "title", "body"); err != nil {
		t.Fatalf("SendPushToUser: %v", err)
	}

	// Subscription should have been removed after 410.
	subs, _ := store.GetSubscriptions(ctx, 1)
	if len(subs) != 0 {
		t.Errorf("expected 0 subscriptions after 410, got %d", len(subs))
	}
}
