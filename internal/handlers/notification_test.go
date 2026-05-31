package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/mail"
	"github.com/HammerMeetNail/nabu/internal/notification"
)

func setupNotificationTest(t *testing.T) (*NotificationHandler, string, *auth.Service) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	notifService := notification.NewService(notification.NewMemoryStore())
	handler := NewNotificationHandler(notifService)

	user, session := quickRegister(authService, "notif@example.com")
	_, _ = householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", "", user.ID,
	)

	return handler, session.ID, authService
}

func TestNotificationList(t *testing.T) {
	handler, sessionID, authService := setupNotificationTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/notifications", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"notifications"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"unreadCount"`) {
		t.Fatalf("missing unreadCount, body = %s", rec.Body.String())
	}
}

func TestNotificationMarkRead(t *testing.T) {
	handler, sessionID, authService := setupNotificationTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/notifications/1/read", nil), authService, sessionID)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	handler.MarkRead(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNotificationMarkRead_InvalidID(t *testing.T) {
	handler, sessionID, authService := setupNotificationTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/notifications/abc/read", nil), authService, sessionID)
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()

	handler.MarkRead(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNotificationMarkAllRead(t *testing.T) {
	handler, sessionID, authService := setupNotificationTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/notifications/read-all", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.MarkAllRead(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNotificationDelete(t *testing.T) {
	handler, sessionID, authService := setupNotificationTest(t)
	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/notifications/1", nil), authService, sessionID)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNotificationDelete_InvalidID(t *testing.T) {
	handler, sessionID, authService := setupNotificationTest(t)
	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/notifications/xyz", nil), authService, sessionID)
	req.SetPathValue("id", "xyz")
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
