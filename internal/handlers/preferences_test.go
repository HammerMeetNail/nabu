package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dave/choresy/internal/auth"
	"github.com/dave/choresy/internal/household"
	"github.com/dave/choresy/internal/mail"
	"github.com/dave/choresy/internal/userprefs"
)

func setupPrefsTest(t *testing.T) (*PreferencesHandler, string, *auth.Service) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	prefsStore := userprefs.NewMemoryStore()
	prefsService := userprefs.NewService(prefsStore)
	handler := NewPreferencesHandler(prefsService)

	user, session := quickRegister(authService, "prefs@example.com")
	_, _ = householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", "", user.ID,
	)

	return handler, session.ID, authService
}

func TestPreferences_GetUnauthorized(t *testing.T) {
	prefsService := userprefs.NewService(userprefs.NewMemoryStore())
	handler := NewPreferencesHandler(prefsService)
	req := httptest.NewRequest(http.MethodGet, "/api/preferences", nil)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestPreferences_Get(t *testing.T) {
	handler, sessionID, authService := setupPrefsTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/preferences", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"preferences"`) {
		t.Fatalf("missing preferences key, body=%s", rec.Body.String())
	}
}

func TestPreferences_UpdateUnauthorized(t *testing.T) {
	prefsService := userprefs.NewService(userprefs.NewMemoryStore())
	handler := NewPreferencesHandler(prefsService)
	req := httptest.NewRequest(http.MethodPatch, "/api/preferences",
		strings.NewReader(`{"timezone": "America/New_York"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestPreferences_UpdateTimezone(t *testing.T) {
	handler, sessionID, authService := setupPrefsTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/preferences",
		strings.NewReader(`{"timezone": "America/New_York"}`),
	), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"preferences"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestPreferences_UpdateChoreOrder(t *testing.T) {
	handler, sessionID, authService := setupPrefsTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/preferences",
		strings.NewReader(`{"choreOrder": [3, 1, 2]}`),
	), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestPreferences_UpdateHiddenChores(t *testing.T) {
	handler, sessionID, authService := setupPrefsTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/preferences",
		strings.NewReader(`{"hiddenHomeChoreIds": [5, 10]}`),
	), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestPreferences_UpdateBadBody(t *testing.T) {
	handler, sessionID, authService := setupPrefsTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/preferences",
		strings.NewReader(`not json`),
	), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
