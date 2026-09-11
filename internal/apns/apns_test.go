package apns

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func testP8(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// ─── Provider token signer ───────────────────────────────────────────────────

func TestProviderTokenSigner_MintsES256(t *testing.T) {
	signer, err := NewProviderTokenSigner(testP8(t), "KEY123", "TEAM456")
	if err != nil {
		t.Fatalf("NewProviderTokenSigner: %v", err)
	}

	token, err := signer.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}

	headerJSON, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var header map[string]string
	_ = json.Unmarshal(headerJSON, &header)
	if header["alg"] != "ES256" || header["kid"] != "KEY123" {
		t.Fatalf("header = %v", header)
	}

	claimsJSON, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	_ = json.Unmarshal(claimsJSON, &claims)
	if claims["iss"] != "TEAM456" {
		t.Fatalf("claims = %v", claims)
	}

	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if len(sig) != 64 {
		t.Fatalf("signature length = %d, want raw 64-byte R||S", len(sig))
	}
}

func TestProviderTokenSigner_CachesUnderAnHour(t *testing.T) {
	signer, err := NewProviderTokenSigner(testP8(t), "K", "T")
	if err != nil {
		t.Fatalf("NewProviderTokenSigner: %v", err)
	}
	now := time.Now()
	signer.now = func() time.Time { return now }

	t1, _ := signer.Token()
	now = now.Add(10 * time.Minute)
	t2, _ := signer.Token()
	if t1 != t2 {
		t.Fatal("token should be cached within 50 minutes")
	}

	now = now.Add(45 * time.Minute) // 55 total
	t3, _ := signer.Token()
	if t3 == t1 {
		t.Fatal("token should refresh after 50 minutes")
	}
}

func TestNewProviderTokenSigner_Rejections(t *testing.T) {
	if _, err := NewProviderTokenSigner("not a pem", "K", "T"); err == nil {
		t.Fatal("expected error for garbage key")
	}
	if _, err := NewProviderTokenSigner(testP8(t), "", "T"); err == nil {
		t.Fatal("expected error for missing key id")
	}
	if _, err := NewProviderTokenSigner(testP8(t), "K", ""); err == nil {
		t.Fatal("expected error for missing team id")
	}
}

// ─── Sender ──────────────────────────────────────────────────────────────────

type fakeAPNs struct {
	mu       sync.Mutex
	requests []fakeAPNsRequest
	// respond maps device token → (status, reason); default 200.
	respond map[string]struct {
		status int
		reason string
	}
}

type fakeAPNsRequest struct {
	token   string
	headers http.Header
	payload map[string]any
}

func (f *fakeAPNs) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/3/device/")
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)

		f.mu.Lock()
		f.requests = append(f.requests, fakeAPNsRequest{token: token, headers: r.Header.Clone(), payload: payload})
		resp, ok := f.respond[token]
		f.mu.Unlock()

		if !ok {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(resp.status)
		_ = json.NewEncoder(w).Encode(map[string]string{"reason": resp.reason})
	}
}

func newTestClient(t *testing.T, store Store, fake *fakeAPNs) *Client {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	signer, err := NewProviderTokenSigner(testP8(t), "KEY123", "TEAM456")
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	client := NewClient(store, signer, "com.nabu.app")
	client.hostOverride = srv.URL
	return client
}

func TestClient_SendsAlertWithHeadersAndData(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	_ = store.RegisterDevice(ctx, Device{UserID: 1, Token: "aabbcc", Environment: EnvironmentSandbox, BundleID: "com.nabu.app"})

	fake := &fakeAPNs{}
	client := newTestClient(t, store, fake)

	err := client.SendPushToUserWithData(ctx, 1, "Reminder", "Feed Baby", map[string]any{
		"choreId": 5, "type": "schedule_reminder", "category": "NABU_REMINDER",
	})
	if err != nil {
		t.Fatalf("SendPushToUserWithData: %v", err)
	}

	if len(fake.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(fake.requests))
	}
	req := fake.requests[0]
	if req.token != "aabbcc" {
		t.Fatalf("token = %q", req.token)
	}
	if req.headers.Get("apns-topic") != "com.nabu.app" {
		t.Fatalf("apns-topic = %q", req.headers.Get("apns-topic"))
	}
	if req.headers.Get("apns-push-type") != "alert" {
		t.Fatalf("apns-push-type = %q", req.headers.Get("apns-push-type"))
	}
	if !strings.HasPrefix(req.headers.Get("Authorization"), "bearer ") {
		t.Fatalf("Authorization = %q", req.headers.Get("Authorization"))
	}

	aps, _ := req.payload["aps"].(map[string]any)
	alert, _ := aps["alert"].(map[string]any)
	if alert["title"] != "Nabu" || alert["body"] != "Open Nabu to check your household updates." {
		t.Fatalf("alert = %v", alert)
	}
	if aps["category"] != "NABU_REMINDER" {
		t.Fatalf("category = %v", aps["category"])
	}
	if req.payload["choreId"] != float64(5) || req.payload["type"] != "schedule_reminder" {
		t.Fatalf("data fields = %v", req.payload)
	}
}

func TestClient_PrunesTerminallyInvalidTokens(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	_ = store.RegisterDevice(ctx, Device{UserID: 1, Token: "dead01", Environment: EnvironmentSandbox, BundleID: "b"})
	_ = store.RegisterDevice(ctx, Device{UserID: 1, Token: "bad002", Environment: EnvironmentSandbox, BundleID: "b"})
	_ = store.RegisterDevice(ctx, Device{UserID: 1, Token: "live03", Environment: EnvironmentSandbox, BundleID: "b"})
	_ = store.RegisterDevice(ctx, Device{UserID: 1, Token: "busy04", Environment: EnvironmentSandbox, BundleID: "b"})

	fake := &fakeAPNs{respond: map[string]struct {
		status int
		reason string
	}{
		"dead01": {http.StatusGone, "Unregistered"},
		"bad002": {http.StatusBadRequest, "BadDeviceToken"},
		"busy04": {http.StatusTooManyRequests, "TooManyRequests"},
	}}
	client := newTestClient(t, store, fake)

	if err := client.SendPushToUser(ctx, 1, "t", "b"); err != nil {
		t.Fatalf("SendPushToUser: %v", err)
	}

	devices, _ := store.DevicesForUser(ctx, 1)
	remaining := map[string]bool{}
	for _, d := range devices {
		remaining[d.Token] = true
	}
	if remaining["dead01"] || remaining["bad002"] {
		t.Fatalf("terminal tokens not pruned: %v", remaining)
	}
	if !remaining["live03"] || !remaining["busy04"] {
		t.Fatalf("healthy/transient tokens must survive: %v", remaining)
	}
}

func TestClient_NoDevicesNoRequests(t *testing.T) {
	fake := &fakeAPNs{}
	client := newTestClient(t, NewMemoryStore(), fake)

	if err := client.SendPushToUser(context.Background(), 42, "t", "b"); err != nil {
		t.Fatalf("SendPushToUser: %v", err)
	}
	if len(fake.requests) != 0 {
		t.Fatalf("requests = %d, want 0", len(fake.requests))
	}
}

func TestClient_HostSelectionByEnvironment(t *testing.T) {
	signer, _ := NewProviderTokenSigner(testP8(t), "K", "T")
	client := NewClient(NewMemoryStore(), signer, "b")
	if got := client.host(EnvironmentSandbox); got != hostSandbox {
		t.Fatalf("sandbox host = %q", got)
	}
	if got := client.host(EnvironmentProduction); got != hostProduction {
		t.Fatalf("production host = %q", got)
	}
	if got := client.host(""); got != hostProduction {
		t.Fatalf("default host = %q, want production", got)
	}
}

// ─── Store ───────────────────────────────────────────────────────────────────

func TestMemoryStore_TokenTakeoverAndIsolation(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	_ = store.RegisterDevice(ctx, Device{UserID: 1, Token: "tok1", Environment: EnvironmentSandbox, BundleID: "b"})
	_ = store.RegisterDevice(ctx, Device{UserID: 2, Token: "tok2", Environment: EnvironmentSandbox, BundleID: "b"})

	// User 2 cannot unregister user 1's token.
	_ = store.UnregisterDevice(ctx, 2, "tok1", "")
	d1, _ := store.DevicesForUser(ctx, 1)
	if len(d1) != 1 {
		t.Fatal("cross-user unregister must not remove the token")
	}

	// Re-registration by another user takes the token over (device changed accounts).
	_ = store.RegisterDevice(ctx, Device{UserID: 2, Token: "tok1", Environment: EnvironmentSandbox, BundleID: "b"})
	d1, _ = store.DevicesForUser(ctx, 1)
	d2, _ := store.DevicesForUser(ctx, 2)
	if len(d1) != 0 || len(d2) != 2 {
		t.Fatalf("takeover failed: user1=%d user2=%d", len(d1), len(d2))
	}

	// Owner unregister works.
	_ = store.UnregisterDevice(ctx, 2, "tok2", "")
	d2, _ = store.DevicesForUser(ctx, 2)
	if len(d2) != 1 {
		t.Fatalf("owner unregister failed: %d", len(d2))
	}
}
