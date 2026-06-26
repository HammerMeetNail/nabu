package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	logsvc "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/mail"
	"github.com/HammerMeetNail/nabu/internal/notification"
	"github.com/HammerMeetNail/nabu/internal/schedule"
)

func setupLogTest(t *testing.T) (*LogHandler, string, *auth.Service) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	logStore := logsvc.NewMemoryStore()
	logService := logsvc.NewService(logStore)
	handler := NewLogHandler(logService)

	user, session := quickRegister(authService, "alice@example.com")
	if _, err := householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", "", user.ID,
	); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	return handler, session.ID, authService
}

func TestLogCreate(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)

	// First create a chore
	choreStore := chore.NewMemoryStore()
	choreService := chore.NewService(choreStore)
	seedReq := withUser(httptest.NewRequest(http.MethodPost, "/", nil), authService, sessionID)
	if err := choreService.SeedDefaultChores(seedReq.Context(), 1); err != nil {
		t.Fatalf("SeedDefaultChores: %v", err)
	}

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(
		`{"choreId":1,"note":"done"}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"log"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestLogToday(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/logs/today", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Today(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"logs"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestLogWeek(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/logs/week?start=2024-01-01", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Week(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestLogMonth(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/logs/month?year=2024&month=3", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Month(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"logs"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestLogMonthNoHousehold(t *testing.T) {
	handler, _, _ := setupLogTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/logs/month", nil)
	rec := httptest.NewRecorder()
	handler.Month(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestLogHistory(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/logs/history?before=2024-01-15", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.History(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"logs"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestLogHistoryNoHousehold(t *testing.T) {
	handler, _, _ := setupLogTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/logs/history", nil)
	rec := httptest.NewRecorder()
	handler.History(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestLogLatestPerChore(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/logs/latest", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.LatestPerChore(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"latestLogs"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestLogLatestPerChoreNoHousehold(t *testing.T) {
	handler, _, _ := setupLogTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/logs/latest", nil)
	rec := httptest.NewRecorder()
	handler.LatestPerChore(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// createLogEntry seeds a chore and creates a log entry, returning the log ID.
func createLogEntry(t *testing.T, handler *LogHandler, authService *auth.Service, sessionID string) int64 {
	t.Helper()
	choreStore := chore.NewMemoryStore()
	choreService := chore.NewService(choreStore)
	if err := choreService.SeedDefaultChores(
		withUser(httptest.NewRequest(http.MethodGet, "/", nil), authService, sessionID).Context(), 1,
	); err != nil {
		t.Fatalf("seed chores: %v", err)
	}

	createReq := withUser(httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(
		`{"choreId":1,"note":"test"}`,
	)), authService, sessionID)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.Create(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create log: status = %d, body=%s", createRec.Code, createRec.Body.String())
	}
	return 1
}

func TestLogUpdate(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	createLogEntry(t, handler, authService, sessionID)

	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/logs/1", strings.NewReader(
		`{"note":"updated note"}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogUpdateInvalidID(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/logs/abc", strings.NewReader(
		`{"note":"x"}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestLogUpdateNotFound(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/logs/9999", strings.NewReader(
		`{"note":"x"}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "9999")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestLogDelete(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	createLogEntry(t, handler, authService, sessionID)

	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/logs/1", nil), authService, sessionID)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogDeleteNoHousehold(t *testing.T) {
	handler, _, _ := setupLogTest(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/logs/1", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestLogDeleteInvalidID(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/logs/abc", nil), authService, sessionID)
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestLogWithNotification(t *testing.T) {
	handler, _, _ := setupLogTest(t)

	notifStore := notification.NewMemoryStore()
	notifService := notification.NewService(notifStore)
	choreStore := chore.NewMemoryStore()
	hStore := household.NewMemoryStore()
	handler.WithNotification(notifService, choreStore, hStore)

	if handler.notifService == nil {
		t.Fatal("notifService should be set after WithNotification")
	}
}

func TestLogCreateWithNotification_FanOut(t *testing.T) {
	// Set up a full log handler with notification service wired up
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	logStore := logsvc.NewMemoryStore()
	logService := logsvc.NewService(logStore)

	choreStore := chore.NewMemoryStore()
	choreService := chore.NewService(choreStore)

	notifStore := notification.NewMemoryStore()
	notifService := notification.NewService(notifStore)

	handler := NewLogHandler(logService)
	handler.WithNotification(notifService, choreStore, householdStore)

	user, session := quickRegister(authService, "alice@example.com")
	if _, err := householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", "", user.ID,
	); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	// Seed chores so choreId=1 exists
	seedReq := withUser(httptest.NewRequest(http.MethodPost, "/", nil), authService, session.ID)
	_ = choreService.SeedDefaultChores(seedReq.Context(), 1)

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(
		`{"choreId":1,"note":"fan out test"}`,
	)), authService, session.ID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	// Give goroutine time to run
	t.Log("fan-out goroutine launched; log created successfully")
}

func TestLogCreateNoHousehold(t *testing.T) {
	handler, _, _ := setupLogTest(t)
	req := httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(`{"choreId":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLogCreateInvalidBody(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/logs",
		strings.NewReader(`{{{invalid`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestLogCreateInvalidDate(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/logs",
		strings.NewReader(`{"choreId":1,"date":"not-a-date"}`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogCreateInvalidCompletedAt(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/logs",
		strings.NewReader(`{"choreId":1,"completedAt":"not-rfc3339"}`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogCreateUserNotMember(t *testing.T) {
	// Set up handler with householdStore wired via WithNotification
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	logStore := logsvc.NewMemoryStore()
	logService := logsvc.NewService(logStore)
	notifStore := notification.NewMemoryStore()
	notifService := notification.NewService(notifStore)
	handler := NewLogHandler(logService)
	handler.WithNotification(notifService, chore.NewMemoryStore(), householdStore)

	user, session := quickRegister(authService, "bob@example.com")
	if _, err := householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"Bob's Home", "", user.ID,
	); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	// Try to log on behalf of userId=9999 who is not a member
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/logs",
		strings.NewReader(`{"choreId":1,"userId":9999}`)), authService, session.ID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogUpdateInvalidBody(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	createLogEntry(t, handler, authService, sessionID)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/logs/1",
		strings.NewReader(`{{{invalid`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogUpdateInvalidDate(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	createLogEntry(t, handler, authService, sessionID)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/logs/1",
		strings.NewReader(`{"date":"not-a-date"}`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogUpdateInvalidCompletedAt(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	createLogEntry(t, handler, authService, sessionID)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/logs/1",
		strings.NewReader(`{"completedAt":"not-rfc3339"}`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogTodayNoHousehold(t *testing.T) {
	handler, _, _ := setupLogTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/logs/today", nil)
	rec := httptest.NewRecorder()
	handler.Today(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLogTodayWithDate(t *testing.T) {
	handler, sessionID, authService := setupLogTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/logs/today?date=2026-01-15", nil),
		authService, sessionID)
	rec := httptest.NewRecorder()
	handler.Today(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogWeekNoHousehold(t *testing.T) {
	handler, _, _ := setupLogTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/logs/week", nil)
	rec := httptest.NewRecorder()
	handler.Week(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// setupLogTestWithFollowUp wiring builds a LogHandler with a schedule store
// (and chore store) attached so that the follow-up logic in Create runs.
// It returns the handler, a valid session ID + auth service, the schedule
// store, the chore store, and the household ID used by the seeded user.
func setupLogTestWithFollowUp(t *testing.T) (*LogHandler, string, *auth.Service, *schedule.MemoryStore, *chore.MemoryStore, int64) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	logStore := logsvc.NewMemoryStore()
	logService := logsvc.NewService(logStore)

	choreStore := chore.NewMemoryStore()
	choreService := chore.NewService(choreStore)
	notifStore := notification.NewMemoryStore()
	notifService := notification.NewService(notifStore)
	scheduleStore := schedule.NewMemoryStore()

	handler := NewLogHandler(logService)
	handler.WithNotification(notifService, choreStore, householdStore)
	handler.WithScheduleStore(scheduleStore)

	user, session := quickRegister(authService, "alice@example.com")
	if _, err := householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", "", user.ID,
	); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	// Seed default chores for household 1 (the household just created).
	if err := choreService.SeedDefaultChores(
		withUser(httptest.NewRequest(http.MethodPost, "/", nil), authService, session.ID).Context(), 1,
	); err != nil {
		t.Fatalf("SeedDefaultChores: %v", err)
	}

	return handler, session.ID, authService, scheduleStore, choreStore, 1
}

// followUpCount returns the number of follow-up schedules for choreID.
func followUpCount(s *schedule.MemoryStore, householdID, choreID int64) int {
	schs, _ := s.ListByHousehold(context.TODO(), householdID)
	n := 0
	for _, sc := range schs {
		if sc.ChoreID == choreID && sc.IsFollowUp {
			n++
		}
	}
	return n
}

func TestLogCreateFollowUpBackdatePreservesExisting(t *testing.T) {
	handler, sessionID, authService, scheduleStore, _, householdID := setupLogTestWithFollowUp(t)

	const choreID = 1
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)

	fuTime := func(base time.Time) string {
		fu := base.Add(3 * time.Hour)
		return fu.Format("2006-01-02T15:04")
	}
	fmtDate := func(t time.Time) string { return t.Format("2006-01-02") }

	// 1) Current log (today's date) with a 3h follow-up -> creates a follow-up.
	body := `{"choreId":1,"date":"` + fmtDate(now) + `","completedAt":"` + now.Format(time.RFC3339) + `","followUpMinutes":180,"followUpTime":"` + fuTime(now) + `"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(body)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create current log: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := followUpCount(scheduleStore, householdID, choreID); got != 1 {
		t.Fatalf("after current log: follow-up count=%d, want 1", got)
	}

	// 2) Backdated log (yesterday's date, no follow-up) must NOT delete the existing follow-up.
	body = `{"choreId":1,"date":"` + fmtDate(yesterday) + `","completedAt":"` + yesterday.Format(time.RFC3339) + `","followUpMinutes":0}`
	req = withUser(httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(body)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create backdated log: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := followUpCount(scheduleStore, householdID, choreID); got != 1 {
		t.Fatalf("after backdated log without follow-up: follow-up count=%d, want 1 (existing must be preserved)", got)
	}

	// 3) Backdated log (yesterday's date) WITH a follow-up must not create a
	//    second follow-up nor replace the existing one (anchored to the past).
	body = `{"choreId":1,"date":"` + fmtDate(yesterday) + `","completedAt":"` + yesterday.Format(time.RFC3339) + `","followUpMinutes":180,"followUpTime":"` + fuTime(yesterday) + `"}`
	req = withUser(httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(body)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create backdated log with follow-up: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := followUpCount(scheduleStore, householdID, choreID); got != 1 {
		t.Fatalf("after backdated log with follow-up: follow-up count=%d, want 1 (existing must be preserved, no dupes)", got)
	}

	// 4) A fresh/current log (today's date) without a follow-up still clears
	//    the existing follow-up.
	body = `{"choreId":1,"date":"` + fmtDate(now) + `","completedAt":"` + now.Format(time.RFC3339) + `","followUpMinutes":0}`
	req = withUser(httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(body)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create current log (clear): status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := followUpCount(scheduleStore, householdID, choreID); got != 0 {
		t.Fatalf("after current log without follow-up: follow-up count=%d, want 0 (current log must clear)", got)
	}
}

// TestLogCreateFollowUpTodayWithOlderHourClearsExisting verifies that a log
// for today's date clears and replaces an existing follow-up even when the
// completedAt hour is behind the current wall clock (e.g. when the when
// input was pre-filled from a schedule's rounded-down hour).  This scenario
// used to be misclassified as backdated by the 10-minute tolerance.
func TestLogCreateFollowUpTodayWithOlderHourClearsExisting(t *testing.T) {
	handler, sessionID, authService, scheduleStore, _, householdID := setupLogTestWithFollowUp(t)

	const choreID = 1
	now := time.Now().UTC()
	fmtDate := func(t time.Time) string { return t.Format("2006-01-02") }

	fuTime := func(base time.Time) string {
		fu := base.Add(3 * time.Hour)
		return fu.Format("2006-01-02T15:04")
	}

	// 1) Log with today's date at 12:30 with a 3h follow-up (creates a
	//    follow-up around 15:30).
	base := time.Date(now.Year(), now.Month(), now.Day(), 12, 30, 0, 0, time.UTC)
	body := `{"choreId":1,"date":"` + fmtDate(base) + `","completedAt":"` + base.Format(time.RFC3339) + `","followUpMinutes":180,"followUpTime":"` + fuTime(base) + `"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(body)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first log: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := followUpCount(scheduleStore, householdID, choreID); got != 1 {
		t.Fatalf("after first log: follow-up count=%d, want 1", got)
	}

	// 2) Log again with today's date at 15:00 (round hour — mimics the
	//    schedule-tab pre-filling the when input to :00) with a new follow-up.
	//    The old follow-up must be deleted and replaced; today's date must
	//    not be considered backdated regardless of the hour.
	later := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, time.UTC)
	body2 := `{"choreId":1,"date":"` + fmtDate(later) + `","completedAt":"` + later.Format(time.RFC3339) + `","followUpMinutes":180,"followUpTime":"` + fuTime(later) + `"}`
	req2 := withUser(httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(body2)), authService, sessionID)
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	handler.Create(rec2, req2)
	if rec2.Code != http.StatusCreated {
		t.Fatalf("second log: status=%d body=%s", rec2.Code, rec2.Body.String())
	}
	if got := followUpCount(scheduleStore, householdID, choreID); got != 1 {
		t.Fatalf("after second log (today with older hour): follow-up count=%d, want 1 (old must be replaced)", got)
	}
}

func TestLogCreateFollowUpReplacesExisting(t *testing.T) {
	handler, sessionID, authService, scheduleStore, _, householdID := setupLogTestWithFollowUp(t)

	const choreID = 1
	now := time.Now().UTC()

	fuTime := func(base time.Time) string {
		fu := base.Add(3 * time.Hour)
		return fu.Format("2006-01-02T15:04")
	}

	// 1) Log with a 3h follow-up -> creates a follow-up schedule.
	body := `{"choreId":1,"completedAt":"` + now.Format(time.RFC3339) + `","followUpMinutes":180,"followUpTime":"` + fuTime(now) + `"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(body)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first log: status=%d body=%s", rec.Code, rec.Body.String())
	}
	schedules, _ := scheduleStore.ListByHousehold(context.TODO(), householdID)
	firstFuID := int64(0)
	for _, s := range schedules {
		if s.ChoreID == choreID && s.IsFollowUp {
			firstFuID = s.ID
		}
	}
	if firstFuID == 0 {
		t.Fatal("first log did not create a follow-up")
	}

	// 2) Log again with a NEW 3h follow-up. The old follow-up must be
	//    deleted and replaced; there must not be two follow-ups.
	later := now.Add(1 * time.Minute)
	body2 := `{"choreId":1,"completedAt":"` + later.Format(time.RFC3339) + `","followUpMinutes":180,"followUpTime":"` + fuTime(later) + `"}`
	req2 := withUser(httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(body2)), authService, sessionID)
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	handler.Create(rec2, req2)
	if rec2.Code != http.StatusCreated {
		t.Fatalf("second log: status=%d body=%s", rec2.Code, rec2.Body.String())
	}

	schedules2, _ := scheduleStore.ListByHousehold(context.TODO(), householdID)
	found := int64(0)
	for _, s := range schedules2 {
		if s.ChoreID == choreID && s.IsFollowUp {
			found = s.ID
		}
	}
	if found == 0 {
		t.Fatal("second log did not create a replacement follow-up")
	}
	if found == firstFuID {
		t.Fatal("old follow-up was not replaced (same ID survived)")
	}

	// Only one follow-up must exist.
	if n := followUpCount(scheduleStore, householdID, choreID); n != 1 {
		t.Fatalf("after replace: follow-up count=%d, want 1", n)
	}
}
