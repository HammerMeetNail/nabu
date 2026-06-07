package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/reminder"
)

func setupReminderTest(t *testing.T) (*ChoreReminderPrefsHandler, string, *auth.Service) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	authService.SetAuditLogger(nil)

	store := reminder.NewMemoryStore()
	handler := NewChoreReminderPrefsHandler(store)

	user, session := quickRegister(authService, "reminder@example.com")
	_ = user

	return handler, session.ID, authService
}

func TestChoreReminderPrefs_ListUnauthorized(t *testing.T) {
	store := reminder.NewMemoryStore()
	handler := NewChoreReminderPrefsHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/api/chore-reminder-prefs", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestChoreReminderPrefs_List(t *testing.T) {
	handler, sessionID, authService := setupReminderTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/chore-reminder-prefs", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var body struct {
		Prefs []reminder.ChoreReminderPref `json:"prefs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Prefs) != 0 {
		t.Errorf("expected empty prefs list, got %d items", len(body.Prefs))
	}
}

func TestChoreReminderPrefs_UpdateUnauthorized(t *testing.T) {
	store := reminder.NewMemoryStore()
	handler := NewChoreReminderPrefsHandler(store)
	req := httptest.NewRequest(http.MethodPatch, "/api/chore-reminder-prefs/10",
		strings.NewReader(`{"enabled": true, "leadMinutes": 15}`))
	req.SetPathValue("choreId", "10")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestChoreReminderPrefs_Update(t *testing.T) {
	handler, sessionID, authService := setupReminderTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/chore-reminder-prefs/10",
		strings.NewReader(`{"enabled": true, "leadMinutes": 15}`)), authService, sessionID)
	req.SetPathValue("choreId", "10")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var body struct {
		Pref reminder.ChoreReminderPref `json:"pref"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.Pref.Enabled {
		t.Error("expected enabled=true")
	}
	if body.Pref.LeadMinutes != 15 {
		t.Errorf("LeadMinutes = %d, want 15", body.Pref.LeadMinutes)
	}
}

func TestChoreReminderPrefs_UpdateInvalidChoreId(t *testing.T) {
	handler, sessionID, authService := setupReminderTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/chore-reminder-prefs/abc",
		strings.NewReader(`{"enabled": true}`)), authService, sessionID)
	req.SetPathValue("choreId", "abc")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
