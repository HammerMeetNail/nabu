package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/mail"
	"github.com/HammerMeetNail/nabu/internal/push"
)

func setupPushAuditTest(t *testing.T) (*PushHandler, *auth.Service, string, *audit.Recorder, int64, int64) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	handler := NewPushHandler(push.NewMemoryStore())
	rec := audit.NewRecorder()
	handler.SetAuditLogger(rec)

	user, session := quickRegister(authService, "push-audit@example.com")
	_, _ = householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", "", user.ID,
	)

	authed, err := authService.Authenticate(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if authed.HouseholdID == nil {
		t.Fatal("test user has no household")
	}
	return handler, authService, session.ID, rec, authed.ID, *authed.HouseholdID
}

func TestPushAudit_Subscribed(t *testing.T) {
	handler, authSvc, sessionID, rec, userID, hhID := setupPushAuditTest(t)

	body := `{"subscription":{"endpoint":"https://fcm.googleapis.com/send/abc","keys":{"p256dh":"BPUB","auth":"AUTH"}}}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/push/subscribe", strings.NewReader(body)), authSvc, sessionID)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.Subscribe(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", resp.Code, resp.Body.String())
	}
	ev, ok := rec.Find("push.subscribed")
	if !ok {
		t.Fatalf("missing push.subscribed; events=%#v", rec.Events())
	}
	if ev.Attrs["user_id"] != itoa(userID) {
		t.Errorf("user_id = %q, want %d", ev.Attrs["user_id"], userID)
	}
	if ev.Attrs["household_id"] != itoa(hhID) {
		t.Errorf("household_id = %q, want %d", ev.Attrs["household_id"], hhID)
	}
}

func TestPushAudit_Unsubscribed(t *testing.T) {
	handler, authSvc, sessionID, rec, userID, hhID := setupPushAuditTest(t)

	// Subscribe first so unsubscribe has a target.
	subBody := `{"subscription":{"endpoint":"https://fcm.googleapis.com/send/abc","keys":{"p256dh":"BPUB","auth":"AUTH"}}}`
	subReq := withUser(httptest.NewRequest(http.MethodPost, "/api/push/subscribe", strings.NewReader(subBody)), authSvc, sessionID)
	subReq.Header.Set("Content-Type", "application/json")
	handler.Subscribe(httptest.NewRecorder(), subReq)

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/push/unsubscribe", strings.NewReader(`{"endpoint":"https://fcm.googleapis.com/send/abc"}`)), authSvc, sessionID)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.Unsubscribe(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", resp.Code, resp.Body.String())
	}
	ev, ok := rec.Find("push.unsubscribed")
	if !ok {
		t.Fatalf("missing push.unsubscribed; events=%#v", rec.Events())
	}
	if ev.Attrs["user_id"] != itoa(userID) {
		t.Errorf("user_id = %q, want %d", ev.Attrs["user_id"], userID)
	}
	if ev.Attrs["household_id"] != itoa(hhID) {
		t.Errorf("household_id = %q, want %d", ev.Attrs["household_id"], hhID)
	}
}

func TestPushAudit_BadBodyNotAudited(t *testing.T) {
	handler, authSvc, sessionID, rec, _, _ := setupPushAuditTest(t)

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/push/subscribe", strings.NewReader(`not json`)), authSvc, sessionID)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.Subscribe(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	if ev, ok := rec.Find("push.subscribed"); ok {
		t.Fatalf("expected no event on failure; got %#v", ev)
	}
}
