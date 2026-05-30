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

	user, session := quickRegister(authService, "alice@example.com")
	if _, err := householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", "", user.ID,
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

	// F13: empty names are now rejected by handler-level validation (400).
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
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

func TestChoreUpdate(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	// Create a chore first
	createReq := withUser(httptest.NewRequest(http.MethodPost, "/api/chores", strings.NewReader(
		`{"name":"Old Name","icon":"🧹","color":"#FF0000","category":"cleaning"}`,
	)), authService, sessionID)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.Create(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body=%s", createRec.Code, createRec.Body.String())
	}

	// Extract the chore ID from the response body using path value simulation
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/chores/1", strings.NewReader(
		`{"name":"New Name","icon":"🧹","color":"#FF0000","category":"cleaning"}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "updated") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestChoreUpdateInvalidID(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/chores/abc", strings.NewReader(
		`{"name":"X"}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestChoreDelete(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	// Create a custom chore first
	createReq := withUser(httptest.NewRequest(http.MethodPost, "/api/chores", strings.NewReader(
		`{"name":"To Delete","icon":"🗑️","color":"#FF0000","category":"custom"}`,
	)), authService, sessionID)
	createReq.Header.Set("Content-Type", "application/json")
	handler.Create(httptest.NewRecorder(), createReq)

	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/chores/1", nil), authService, sessionID)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestChoreDeleteInvalidID(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/chores/abc", nil), authService, sessionID)
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestChoreReorder(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	// Seed some chores first
	seedReq := withUser(httptest.NewRequest(http.MethodPost, "/api/chores/seed-defaults", nil), authService, sessionID)
	handler.SeedDefaults(httptest.NewRecorder(), seedReq)

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/chores/reorder", strings.NewReader(
		`{"choreIds":[1,2,3]}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Reorder(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestChoreReorderNoHousehold(t *testing.T) {
	handler, _, _, _ := setupChoreTest(t)
	req := httptest.NewRequest(http.MethodPost, "/api/chores/reorder", strings.NewReader(`{"choreIds":[1]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Reorder(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestChoreRestoreDefault(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	// Seed defaults so chore 1 exists
	seedReq := withUser(httptest.NewRequest(http.MethodPost, "/api/chores/seed-defaults", nil), authService, sessionID)
	handler.SeedDefaults(httptest.NewRecorder(), seedReq)

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/chores/1/restore-default", nil), authService, sessionID)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	handler.RestoreDefault(rec, req)

	// Chore 1 is predefined so restore should work
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestChoreRestoreDefaultInvalidID(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/chores/abc/restore-default", nil), authService, sessionID)
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()

	handler.RestoreDefault(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestChoreGetDefaults(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/chores/defaults", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.GetDefaults(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"defaults"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestChoreGetInvalidID(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/chores/abc", nil), authService, sessionID)
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()
	handler.Get(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestChoreGetSuccess(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	// Create a chore first
	createReq := withUser(httptest.NewRequest(http.MethodPost, "/api/chores", strings.NewReader(
		`{"name":"Get Me","icon":"🧹","color":"#FF0000","category":"cleaning"}`,
	)), authService, sessionID)
	createReq.Header.Set("Content-Type", "application/json")
	handler.Create(httptest.NewRecorder(), createReq)

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/chores/1", nil), authService, sessionID)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handler.Get(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"Get Me"`) {
		t.Fatalf("body missing chore name: %s", rec.Body.String())
	}
}

func TestChoreCreateNoHousehold(t *testing.T) {
	handler, _, _, _ := setupChoreTest(t)
	req := httptest.NewRequest(http.MethodPost, "/api/chores", strings.NewReader(
		`{"name":"X","icon":"🧹","color":"#FF0000","category":"cleaning"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestChoreCreateInvalidBody(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/chores",
		strings.NewReader(`{{{invalid`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestChoreUpdateInvalidBody(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/chores/1",
		strings.NewReader(`{{{invalid`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestChoreUpdateNotFound(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/chores/9999", strings.NewReader(
		`{"name":"New"}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "9999")
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestChoreReorderInvalidBody(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/chores/reorder",
		strings.NewReader(`{{{invalid`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Reorder(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestChoreSeedDefaultsNoHousehold(t *testing.T) {
	handler, _, _, _ := setupChoreTest(t)
	req := httptest.NewRequest(http.MethodPost, "/api/chores/seed-defaults", nil)
	rec := httptest.NewRecorder()
	handler.SeedDefaults(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// ─── F13+F14: Server-side validation ─────────────────────────────────────────

func TestChoreCreateEmptyNameRejected(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	body := `{"name":"","icon":"🧹","color":"#FF0000","category":"cleaning"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/chores", strings.NewReader(body)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty name: status = %d, want 400", rec.Code)
	}
}

func TestChoreCreateNameTooLong(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	longName := strings.Repeat("x", 61)
	body := `{"name":"` + longName + `","icon":"🧹","color":"#FF0000","category":"cleaning"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/chores", strings.NewReader(body)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("name too long: status = %d, want 400", rec.Code)
	}
}

func TestChoreCreateInvalidColorRejected(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	body := `{"name":"Sweep","icon":"🧹","color":"red","category":"cleaning"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/chores", strings.NewReader(body)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid color: status = %d, want 400", rec.Code)
	}
}

func TestChoreCreateValidHexColorAccepted(t *testing.T) {
	handler, sessionID, authService, _ := setupChoreTest(t)
	body := `{"name":"Sweep","icon":"🧹","color":"#1A2B3C","category":"cleaning"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/chores", strings.NewReader(body)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("valid color: status = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
}
