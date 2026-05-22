package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dave/choresy/internal/auth"
	"github.com/dave/choresy/internal/chore"
	"github.com/dave/choresy/internal/household"
	"github.com/dave/choresy/internal/mail"
	"github.com/dave/choresy/internal/middleware"
)

func setupChoreTest(t *testing.T) (*ChoreHandler, string, *auth.Service, *household.Service) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	choreStore := chore.NewMemoryStore()
	choreService := chore.NewService(choreStore)
	handler := NewChoreHandler(choreService)

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

	return handler, session.ID, authService, householdService
}

func withUser(r *http.Request, authService *auth.Service, sessionID string) *http.Request {
	r.AddCookie(&http.Cookie{Name: "choresy_session", Value: sessionID})
	var authed *http.Request
	rec := httptest.NewRecorder()
	middleware.Session(authService, "choresy_session")(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		authed = req
	})).ServeHTTP(rec, r)
	return authed
}

func TestChoreListEmpty(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/chores", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"chores"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestChoreCreate(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/chores", strings.NewReader(
		`{"name":"Test Chore","icon":"🧹","color":"#FF0000","category":"cleaning"}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"Test Chore"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestChoreCreateEmptyName(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/chores", strings.NewReader(
		`{"name":"","icon":"🧹","color":"#FF0000","category":"cleaning"}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestChoreGetNotFound(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/chores/99999", nil), authService, sessionID)
	req.SetPathValue("id", "99999")
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestChoreSeedDefaults(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/chores/seed-defaults", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.SeedDefaults(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestChoreRequiresHousehold(t *testing.T) {
	handler, _, _, _ := setupChoreTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/chores", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
