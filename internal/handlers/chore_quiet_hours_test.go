package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/reminder"
	"github.com/HammerMeetNail/nabu/internal/schedule"
)

func postChore(t *testing.T, handler *ChoreHandler, authService *auth.Service, sessionID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/chores", strings.NewReader(body)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	return rec
}

func TestChoreCreateWithQuietHours(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	rec := postChore(t, handler, authService, sessionID,
		`{"name":"Feed baby","icon":"🍼","color":"#FF0000","category":"care","quietHoursStart":"22:00","quietHoursEnd":"07:00"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created struct {
		Chore chore.Chore `json:"chore"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if created.Chore.QuietHoursStart != "22:00" || created.Chore.QuietHoursEnd != "07:00" {
		t.Fatalf("quiet hours = %q..%q, want 22:00..07:00", created.Chore.QuietHoursStart, created.Chore.QuietHoursEnd)
	}
}

func TestChoreCreateQuietHoursPartialRejected(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	rec := postChore(t, handler, authService, sessionID,
		`{"name":"Feed baby","icon":"🍼","color":"#FF0000","quietHoursStart":"22:00"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestChoreCreateQuietHoursInvalidFormatRejected(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	rec := postChore(t, handler, authService, sessionID,
		`{"name":"Feed baby","icon":"🍼","color":"#FF0000","quietHoursStart":"25:99","quietHoursEnd":"07:00"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestChoreUpdateQuietHoursSetAndClear(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	createRec := postChore(t, handler, authService, sessionID, `{"name":"Feed baby","icon":"🍼","color":"#FF0000"}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body=%s", createRec.Code, createRec.Body.String())
	}

	patch := func(body string) *httptest.ResponseRecorder {
		req := withUser(httptest.NewRequest(http.MethodPatch, "/api/chores/1", strings.NewReader(body)), authService, sessionID)
		req.Header.Set("Content-Type", "application/json")
		req.SetPathValue("id", "1")
		rec := httptest.NewRecorder()
		handler.Update(rec, req)
		return rec
	}

	rec := patch(`{"quietHoursStart":"21:30","quietHoursEnd":"06:30"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("set: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"quietHoursStart":"21:30"`) || !strings.Contains(rec.Body.String(), `"quietHoursEnd":"06:30"`) {
		t.Fatalf("set: body = %s", rec.Body.String())
	}

	// Explicit empty strings clear the window.
	rec = patch(`{"quietHoursStart":"","quietHoursEnd":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"quietHoursStart"`) || strings.Contains(rec.Body.String(), `"quietHoursEnd"`) {
		t.Fatalf("clear: quiet hours still present: %s", rec.Body.String())
	}

	// Malformed bound is rejected.
	rec = patch(`{"quietHoursStart":"9am"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid: status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestChoreReminderPrefsQuietHoursUpdate(t *testing.T) {
	handler, sessionID, authService := setupReminderTest(t)

	patch := func(body string) *httptest.ResponseRecorder {
		req := withUser(httptest.NewRequest(http.MethodPatch, "/api/chore-reminder-prefs/1", strings.NewReader(body)), authService, sessionID)
		req.Header.Set("Content-Type", "application/json")
		req.SetPathValue("choreId", "1")
		rec := httptest.NewRecorder()
		handler.Update(rec, req)
		return rec
	}
	// decodePref unmarshals into a fresh struct each time; reusing one across
	// steps would keep stale values for fields omitted via omitempty.
	decodePref := func(t *testing.T, body []byte) reminder.ChoreReminderPref {
		t.Helper()
		var resp struct {
			Pref reminder.ChoreReminderPref `json:"pref"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return resp.Pref
	}

	rec := patch(`{"enabled":true,"leadMinutes":15,"quietHoursStart":"22:00","quietHoursEnd":"07:00"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	pref := decodePref(t, rec.Body.Bytes())
	if pref.QuietHoursStart != "22:00" || pref.QuietHoursEnd != "07:00" {
		t.Fatalf("quiet hours = %q..%q, want 22:00..07:00", pref.QuietHoursStart, pref.QuietHoursEnd)
	}

	// Partial update must not clobber the other bound.
	rec = patch(`{"quietHoursStart":"23:00"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("partial: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	pref = decodePref(t, rec.Body.Bytes())
	if pref.QuietHoursStart != "23:00" || pref.QuietHoursEnd != "07:00" {
		t.Fatalf("partial update clobbered end: %q..%q", pref.QuietHoursStart, pref.QuietHoursEnd)
	}

	// Malformed time is rejected.
	rec = patch(`{"quietHoursEnd":"7pm"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid: status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	// Empty string clears the bound.
	rec = patch(`{"quietHoursStart":"","quietHoursEnd":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	pref = decodePref(t, rec.Body.Bytes())
	if pref.QuietHoursStart != "" || pref.QuietHoursEnd != "" {
		t.Fatalf("clear: bounds still set: %q..%q", pref.QuietHoursStart, pref.QuietHoursEnd)
	}
}

func setupReminderLogTest(t *testing.T) (*ReminderLogHandler, string, *auth.Service, *reminder.MemoryStore) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	user, session := quickRegister(authService, "log@example.com")

	choreStore := chore.NewMemoryStore()
	schedStore := schedule.NewMemoryStore()
	remindStore := reminder.NewMemoryStore().WithStores(schedStore, choreStore)

	c, err := choreStore.CreateChore(context.Background(), chore.Chore{
		HouseholdID: 1, Name: "Feed baby", Icon: "🍼", Color: "#2E86AB",
		Category: "custom", Visibility: chore.VisibilityHousehold, CreatedBy: &user.ID,
	})
	if err != nil {
		t.Fatalf("CreateChore: %v", err)
	}
	sch, err := schedStore.Create(context.Background(), schedule.ChoreSchedule{
		HouseholdID: 1, ChoreID: c.ID, FrequencyType: "daily",
		SpecificTime: "07:00", TimePeriod: schedule.PeriodAnytime, IsActive: true,
	})
	if err != nil {
		t.Fatalf("schedule.Create: %v", err)
	}
	if err := remindStore.RecordReminder(context.Background(), sch.ID, user.ID, "2026-09-27"); err != nil {
		t.Fatalf("RecordReminder: %v", err)
	}
	return NewReminderLogHandler(remindStore), session.ID, authService, remindStore
}

func TestReminderLogUnauthorized(t *testing.T) {
	handler, _, _, _ := setupReminderLogTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/reminders/log", nil)
	rec := httptest.NewRecorder()
	handler.List(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestReminderLogReturnsOwnRecords(t *testing.T) {
	handler, sessionID, authService, _ := setupReminderLogTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/reminders/log", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.List(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Reminders []reminder.ReminderRecord `json:"reminders"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Reminders) != 1 {
		t.Fatalf("reminders = %+v, want 1 record", resp.Reminders)
	}
	r := resp.Reminders[0]
	if r.ChoreID == 0 || r.ChoreName != "Feed baby" || r.ChoreIcon != "🍼" || r.ScheduledDate != "2026-09-27" {
		t.Fatalf("record = %+v, want enriched own record", r)
	}
}

func TestReminderLogEmpty(t *testing.T) {
	remindStore := reminder.NewMemoryStore()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	_, session := quickRegister(authService, "empty@example.com")
	handler := NewReminderLogHandler(remindStore)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/reminders/log", nil), authService, session.ID)
	rec := httptest.NewRecorder()
	handler.List(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"reminders":[]`) {
		t.Fatalf("body = %s, want empty list", rec.Body.String())
	}
}
