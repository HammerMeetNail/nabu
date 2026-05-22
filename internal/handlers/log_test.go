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
