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

// deliverAll sends one message to each of the user's targets, as a Deliverer
// does, and returns each outcome.
func deliverAll(t *testing.T, svc *Service, userID int64, title, body string) []Outcome {
	t.Helper()
	ctx := context.Background()
	targets, err := svc.Targets(ctx, userID)
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	var outcomes []Outcome
	for _, target := range targets {
		outcomes = append(outcomes, svc.Deliver(ctx, target, Message{UserID: userID, Title: title, Body: body}))
	}
	return outcomes
}

func TestNewService(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}

func TestDeliver_NoSigner(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil) // signer = nil means push disabled

	ctx := context.Background()
	// Save a subscription to ensure the no-signer short circuit runs
	_ = store.SaveSubscription(ctx, 1, Subscription{Endpoint: "https://push.example.com", P256DH: "k", Auth: "a"})

	if outcomes := deliverAll(t, svc, 1, "Hello", "World"); len(outcomes) != 0 {
		t.Fatalf("disabled push delivered: %v", outcomes)
	}
}

func TestDeliver_NoSubscriptions(t *testing.T) {
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

	// User 99 has no subscriptions.
	if outcomes := deliverAll(t, svc, 99, "title", "body"); len(outcomes) != 0 {
		t.Fatalf("outcomes for no subscriptions: %v", outcomes)
	}
}

// TestDeliver_SuccessfulSend exercises the full send path using a valid
// P-256 subscription key and a stub transport returning 201.
func TestDeliver_SuccessfulSend(t *testing.T) {
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

	if outcomes := deliverAll(t, svc, 2, "title", "body"); len(outcomes) != 1 || outcomes[0] != Delivered {
		t.Fatalf("outcomes = %v, want one Delivered", outcomes)
	}
	if len(rt.requests) != 1 {
		t.Errorf("expected 1 request, got %d", len(rt.requests))
	}
	if len(rt.requests) == 1 && rt.requests[0].URL.String() != "https://fcm.googleapis.com/fcm/send/abc" {
		t.Errorf("request URL = %s", rt.requests[0].URL)
	}
}

// TestDeliver_InvalidEndpoint: a syntactically invalid endpoint fails
// validation, so nothing is sent and the device is given up on.
func TestDeliver_InvalidEndpoint(t *testing.T) {
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
	if outcomes := deliverAll(t, svc, 4, "title", "body"); len(outcomes) != 1 || outcomes[0] != GiveUp {
		t.Fatalf("invalid endpoint outcomes = %v, want one GiveUp", outcomes)
	}
}

// TestDeliver_NetworkError: the transport fails after the endpoint passed
// validation, which is worth a retry, and leaks nothing into the log.
func TestDeliver_NetworkError(t *testing.T) {
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

	if outcomes := deliverAll(t, svc, 5, "title", "body"); len(outcomes) != 1 || outcomes[0] != Retry {
		t.Fatalf("network error outcomes = %v, want one Retry", outcomes)
	}
	if len(rt.requests) != 1 {
		t.Errorf("expected 1 attempted request, got %d", len(rt.requests))
	}
	if strings.Contains(captured.String(), "recognizable-secret-capability") || strings.Contains(captured.String(), "private@example.invalid") {
		t.Fatal("transport diagnostics leaked endpoint or private error text")
	}
}
func TestDeliver_StaleEndpointCleanup(t *testing.T) {
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

	if outcomes := deliverAll(t, svc, 1, "title", "body"); len(outcomes) != 1 || outcomes[0] != GiveUp {
		t.Fatalf("410 outcomes = %v, want one GiveUp", outcomes)
	}

	// Subscription should have been removed after 410.
	subs, _ := store.GetSubscriptions(ctx, 1)
	if len(subs) != 0 {
		t.Errorf("expected 0 subscriptions after 410, got %d", len(subs))
	}
}

// TestDeliver_SkipsDisallowedEndpoints is the send-time SSRF guard:
// stored endpoints that fail EndpointAllowed (pre-fix rows or tampered data)
// must never be POSTed to.
func TestDeliver_SkipsDisallowedEndpoints(t *testing.T) {
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

	for _, outcome := range deliverAll(t, svc, 100, "title", "body") {
		if outcome != GiveUp {
			t.Fatalf("disallowed endpoint outcome = %v, want GiveUp", outcome)
		}
	}
	if len(rt.requests) != 0 {
		t.Errorf("expected 0 outbound requests for disallowed endpoints, got %d", len(rt.requests))
	}
}

// A provider's status decides whether a delivery is retried.
func TestDeliver_ProviderStatusOutcomes(t *testing.T) {
	priv, pub, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewVAPIDSigner(priv, pub, "mailto:test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		status int
		want   Outcome
		kept   bool
	}{
		{http.StatusCreated, Delivered, true},
		{http.StatusTooManyRequests, Retry, true},
		{http.StatusBadGateway, Retry, true},
		{http.StatusBadRequest, GiveUp, true},
		{http.StatusNotFound, GiveUp, false},
		{http.StatusGone, GiveUp, false},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			store := NewMemoryStore()
			ctx := context.Background()
			if err := store.SaveSubscription(ctx, 1, Subscription{Endpoint: "https://fcm.googleapis.com/fcm/send/abc",
				P256DH: validP256DH, Auth: validAuth}); err != nil {
				t.Fatal(err)
			}
			svc := NewService(store, signer)
			svc.client = &http.Client{Transport: &stubRoundTripper{status: tc.status}}
			if outcomes := deliverAll(t, svc, 1, "title", "body"); len(outcomes) != 1 || outcomes[0] != tc.want {
				t.Fatalf("outcomes = %v, want %v", outcomes, tc.want)
			}
			subs, _ := store.GetSubscriptions(ctx, 1)
			if (len(subs) == 1) != tc.kept {
				t.Fatalf("subscriptions left = %d, kept %v", len(subs), tc.kept)
			}
		})
	}
}

// A registration replaced after it was listed is not sent to, and not
// retried.
func TestDeliver_ReplacedRegistrationGivesUp(t *testing.T) {
	priv, pub, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewVAPIDSigner(priv, pub, "mailto:test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemoryStore()
	ctx := context.Background()
	sub := Subscription{Endpoint: "https://fcm.googleapis.com/fcm/send/abc", P256DH: validP256DH, Auth: validAuth}
	if err := store.SaveSubscription(ctx, 1, sub); err != nil {
		t.Fatal(err)
	}
	rt := &stubRoundTripper{status: http.StatusCreated}
	svc := NewService(store, signer)
	svc.client = &http.Client{Transport: rt}
	targets, err := svc.Targets(ctx, 1)
	if err != nil || len(targets) != 1 {
		t.Fatalf("targets %d %v", len(targets), err)
	}
	if err := store.SaveSubscription(ctx, 1, sub); err != nil { // a new registration ID
		t.Fatal(err)
	}
	if outcome := svc.Deliver(ctx, targets[0], Message{UserID: 1, Title: "t"}); outcome != GiveUp || len(rt.requests) != 0 {
		t.Fatalf("outcome = %v requests = %d", outcome, len(rt.requests))
	}
}
