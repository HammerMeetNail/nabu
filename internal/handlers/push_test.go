package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/mail"
	"github.com/HammerMeetNail/nabu/internal/push"
)

func setupPushTest(t *testing.T) (*PushHandler, string, *auth.Service) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	pushStore := push.NewMemoryStore()
	handler := NewPushHandler(pushStore)

	user, session := quickRegister(authService, "push@example.com")
	_, _ = householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", "", user.ID,
	)

	return handler, session.ID, authService
}

func TestPushSubscribe(t *testing.T) {
	handler, sessionID, authService := setupPushTest(t)
	body := `{"bindingId":"12345678-1234-1234-1234-123456789abc","subscription":{"endpoint":"https://fcm.googleapis.com/send/abc","keys":{"p256dh":"BPUB","auth":"AUTH"}}}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/push/subscribe", strings.NewReader(body)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Subscribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"subscribed"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestPushSubscribe_BadBody(t *testing.T) {
	handler, sessionID, authService := setupPushTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/push/subscribe", strings.NewReader(`not json`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Subscribe(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestPushSubscribe_RejectsDisallowedEndpoints is the subscribe-time SSRF
// guard: endpoints that point at internal addresses or non-push hosts must be
// rejected before anything is stored.
func TestPushSubscribe_RejectsDisallowedEndpoints(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"loopback http", `{"bindingId":"12345678-1234-1234-1234-123456789abc","subscription":{"endpoint":"http://localhost/x","keys":{"p256dh":"BPUB","auth":"AUTH"}}}`},
		{"cloud metadata", `{"bindingId":"12345678-1234-1234-1234-123456789abc","subscription":{"endpoint":"http://169.254.169.254/latest/meta-data","keys":{"p256dh":"BPUB","auth":"AUTH"}}}`},
		{"private ip", `{"bindingId":"12345678-1234-1234-1234-123456789abc","subscription":{"endpoint":"http://10.0.0.5/x","keys":{"p256dh":"BPUB","auth":"AUTH"}}}`},
		{"internal hostname", `{"bindingId":"12345678-1234-1234-1234-123456789abc","subscription":{"endpoint":"https://metadata.internal/latest/meta-data/","keys":{"p256dh":"BPUB","auth":"AUTH"}}}`},
		{"http scheme", `{"bindingId":"12345678-1234-1234-1234-123456789abc","subscription":{"endpoint":"http://fcm.googleapis.com/x","keys":{"p256dh":"BPUB","auth":"AUTH"}}}`},
		{"not a url", `{"bindingId":"12345678-1234-1234-1234-123456789abc","subscription":{"endpoint":"notaurl","keys":{"p256dh":"BPUB","auth":"AUTH"}}}`},
		{"empty endpoint", `{"bindingId":"12345678-1234-1234-1234-123456789abc","subscription":{"endpoint":"","keys":{"p256dh":"BPUB","auth":"AUTH"}}}`},
		{"missing keys", `{"bindingId":"12345678-1234-1234-1234-123456789abc","subscription":{"endpoint":"https://fcm.googleapis.com/fcm/send/abc","keys":{}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler, sessionID, authService := setupPushTest(t)
			req := withUser(httptest.NewRequest(http.MethodPost, "/api/push/subscribe", strings.NewReader(tc.body)), authService, sessionID)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.Subscribe(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestPushUnsubscribe(t *testing.T) {
	handler, sessionID, authService := setupPushTest(t)

	// First subscribe
	body := `{"bindingId":"12345678-1234-1234-1234-123456789abc","subscription":{"endpoint":"https://fcm.googleapis.com/send/abc","keys":{"p256dh":"BPUB","auth":"AUTH"}}}`
	subReq := withUser(httptest.NewRequest(http.MethodPost, "/api/push/subscribe", strings.NewReader(body)), authService, sessionID)
	subReq.Header.Set("Content-Type", "application/json")
	subRec := httptest.NewRecorder()
	handler.Subscribe(subRec, subReq)

	// Then unsubscribe
	unsubBody := `{"endpoint":"https://fcm.googleapis.com/send/abc"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/push/unsubscribe", strings.NewReader(unsubBody)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Unsubscribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"unsubscribed"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestPushUnsubscribe_BadBody(t *testing.T) {
	handler, sessionID, authService := setupPushTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/push/unsubscribe", strings.NewReader(`not json`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Unsubscribe(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
