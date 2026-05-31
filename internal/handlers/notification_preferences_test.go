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

func setupNotifPrefsTest(t *testing.T) (*NotificationPreferencesHandler, string, *auth.Service) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	notifService := notification.NewService(notification.NewMemoryStore())
	handler := NewNotificationPreferencesHandler(notifService)

	user, session := quickRegister(authService, "notifprefs@example.com")
	_, _ = householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", "", user.ID,
	)

	return handler, session.ID, authService
}

func TestNotificationPreferences_GetUnauthorized(t *testing.T) {
	notifService := notification.NewService(notification.NewMemoryStore())
	handler := NewNotificationPreferencesHandler(notifService)
	req := httptest.NewRequest(http.MethodGet, "/api/notification-preferences", nil)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestNotificationPreferences_Get(t *testing.T) {
	handler, sessionID, authService := setupNotifPrefsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/notification-preferences", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"preferences"`) {
		t.Fatalf("missing preferences key, body=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"availableTypes"`) {
		t.Fatalf("missing availableTypes key, body=%s", rec.Body.String())
	}
}

func TestNotificationPreferences_UpdateUnauthorized(t *testing.T) {
	notifService := notification.NewService(notification.NewMemoryStore())
	handler := NewNotificationPreferencesHandler(notifService)
	req := httptest.NewRequest(http.MethodPatch, "/api/notification-preferences",
		strings.NewReader(`{"pushEnabled": false}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestNotificationPreferences_Update(t *testing.T) {
	handler, sessionID, authService := setupNotifPrefsTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/notification-preferences",
		strings.NewReader(`{"pushEnabled": false, "emailEnabled": true}`),
	), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"preferences"`) {
		t.Fatalf("missing preferences, body=%s", rec.Body.String())
	}
}

func TestNotificationPreferences_UpdateBadBody(t *testing.T) {
	handler, sessionID, authService := setupNotifPrefsTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/notification-preferences",
		strings.NewReader(`not json`),
	), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNotificationPreferences_UpdateEnabledTypes(t *testing.T) {
	handler, sessionID, authService := setupNotifPrefsTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/notification-preferences",
		strings.NewReader(`{"enabledPushTypes": ["chore_logged"]}`),
	), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}
