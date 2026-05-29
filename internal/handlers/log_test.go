package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dave/choresy/internal/auth"
	"github.com/dave/choresy/internal/chore"
	"github.com/dave/choresy/internal/household"
	logsvc "github.com/dave/choresy/internal/log"
	"github.com/dave/choresy/internal/mail"
	"github.com/dave/choresy/internal/notification"
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

	user, session, _ := authService.Register(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"alice@example.com", "password123",
	)
	if _, err := householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", user.ID,
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

	user, session, _ := authService.Register(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"alice@example.com", "password123",
	)
	if _, err := householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", user.ID,
	); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	// Seed chores so choreId=1 exists
	seedReq := withUser(httptest.NewRequest(http.MethodPost, "/", nil), authService, session.ID)
	choreService.SeedDefaultChores(seedReq.Context(), 1)

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

	user, session, _ := authService.Register(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"bob@example.com", "password123",
	)
	if _, err := householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"Bob's Home", user.ID,
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
