package push

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"
)

// validP256DH and validAuth are real P-256 test keys from encrypt_test.go.
const (
	validP256DH = "BFiD30jh-xT1-ztVT4-JzRZUkVaC3jSJpXSpsu8uy1q86f28QIg8W2iznxdqLqdlg7nYlVru_A1FjmmTmrV31Eo"
	validAuth   = "mfbNpuEjYa06PwSg5azFcw"
)

// stubRoundTripper stands in for real push-host network access: endpoints are
// now restricted to allowlisted hosts (fcm.googleapis.com etc.), so tests use
// a fake transport that records requests instead of dialing.
type stubRoundTripper struct {
	status   int
	err      error
	requests []*http.Request
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	s.requests = append(s.requests, req)
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{
		StatusCode: s.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
}

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
// using a valid P-256 subscription key and a stub transport returning 201.
func TestSendPushToUser_SuccessfulSend(t *testing.T) {
	priv, pub, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	signer, err := NewVAPIDSigner(priv, pub, "mailto:test@example.com")
	if err != nil {
		t.Fatalf("NewVAPIDSigner: %v", err)
	}

	rt := &stubRoundTripper{status: http.StatusCreated}
	svc := NewService(NewMemoryStore(), signer)
	svc.client = &http.Client{Transport: rt}

	store := NewMemoryStore()
	ctx := context.Background()
	_ = store.SaveSubscription(ctx, 2, Subscription{
		Endpoint: "https://fcm.googleapis.com/fcm/send/abc",
		P256DH:   validP256DH,
		Auth:     validAuth,
	})
	svc.store = store

	if err := svc.SendPushToUser(ctx, 2, "title", "body"); err != nil {
		t.Fatalf("SendPushToUser: %v", err)
	}
	if len(rt.requests) != 1 {
		t.Errorf("expected 1 request, got %d", len(rt.requests))
	}
	if len(rt.requests) == 1 && rt.requests[0].URL.String() != "https://fcm.googleapis.com/fcm/send/abc" {
		t.Errorf("request URL = %s", rt.requests[0].URL)
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
		Endpoint: "://invalid-url", // will fail endpoint validation
		P256DH:   validP256DH,
		Auth:     validAuth,
	})

	svc := NewService(store, signer)
	if err := svc.SendPushToUser(ctx, 4, "title", "body"); err != nil {
		t.Fatalf("SendPushToUser with invalid endpoint: %v", err)
	}
}

// TestSendPushToUser_NetworkError covers the client.Do error path (lines 70–72):
// the transport fails after the endpoint passed validation.
func TestSendPushToUser_NetworkError(t *testing.T) {
	var captured bytes.Buffer
	old := log.Writer()
	log.SetOutput(&captured)
	t.Cleanup(func() { log.SetOutput(old) })
	priv, pub, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	signer, err := NewVAPIDSigner(priv, pub, "mailto:test@example.com")
	if err != nil {
		t.Fatalf("NewVAPIDSigner: %v", err)
	}

	rt := &stubRoundTripper{err: errors.New("connection refused for private@example.invalid")}
	svc := NewService(NewMemoryStore(), signer)
	svc.client = &http.Client{Transport: rt}

	store := NewMemoryStore()
	ctx := context.Background()
	_ = store.SaveSubscription(ctx, 5, Subscription{
		Endpoint: "https://fcm.googleapis.com/fcm/send/recognizable-secret-capability",
		P256DH:   validP256DH,
		Auth:     validAuth,
	})
	svc.store = store

	if err := svc.SendPushToUser(ctx, 5, "title", "body"); err != nil {
		t.Fatalf("SendPushToUser with failing transport: %v", err)
	}
	if len(rt.requests) != 1 {
		t.Errorf("expected 1 attempted request, got %d", len(rt.requests))
	}
	if strings.Contains(captured.String(), "recognizable-secret-capability") || strings.Contains(captured.String(), "private@example.invalid") {
		t.Fatal("transport diagnostics leaked endpoint or private error text")
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

	rt := &stubRoundTripper{status: http.StatusGone} // 410 — stale subscription
	svc := NewService(NewMemoryStore(), signer)
	svc.client = &http.Client{Transport: rt}

	store := NewMemoryStore()
	ctx := context.Background()
	_ = store.SaveSubscription(ctx, 1, Subscription{
		Endpoint: "https://fcm.googleapis.com/fcm/send/abc",
		P256DH:   validP256DH,
		Auth:     validAuth,
	})
	svc.store = store

	if err := svc.SendPushToUser(ctx, 1, "title", "body"); err != nil {
		t.Fatalf("SendPushToUser: %v", err)
	}

	// Subscription should have been removed after 410.
	subs, _ := store.GetSubscriptions(ctx, 1)
	if len(subs) != 0 {
		t.Errorf("expected 0 subscriptions after 410, got %d", len(subs))
	}
}

// TestSendPushToUser_SkipsDisallowedEndpoints is the send-time SSRF guard:
// stored endpoints that fail EndpointAllowed (pre-fix rows or tampered data)
// must never be POSTed to.
func TestSendPushToUser_SkipsDisallowedEndpoints(t *testing.T) {
	priv, pub, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	signer, err := NewVAPIDSigner(priv, pub, "mailto:test@example.com")
	if err != nil {
		t.Fatalf("NewVAPIDSigner: %v", err)
	}

	rt := &stubRoundTripper{status: http.StatusCreated}
	svc := NewService(NewMemoryStore(), signer)
	svc.client = &http.Client{Transport: rt}

	store := NewMemoryStore()
	ctx := context.Background()
	disallowed := []string{
		"http://169.254.169.254/latest/meta-data/",    // cloud metadata
		"http://10.0.0.5/x",                           // private IP
		"https://127.0.0.1/admin",                     // loopback
		"https://metadata.internal/latest/meta-data/", // internal hostname
		"http://localhost/x",
		"notaurl",
	}
	for _, ep := range disallowed {
		_ = store.SaveSubscription(ctx, 100, Subscription{
			Endpoint: ep,
			P256DH:   validP256DH,
			Auth:     validAuth,
		})
	}

	if err := svc.SendPushToUser(ctx, 100, "title", "body"); err != nil {
		t.Fatalf("SendPushToUser: %v", err)
	}
	if len(rt.requests) != 0 {
		t.Errorf("expected 0 outbound requests for disallowed endpoints, got %d", len(rt.requests))
	}
}
