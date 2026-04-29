// internal/handlers/schedule_test.go

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dave/choresy/internal/auth"
	"github.com/dave/choresy/internal/household"
	"github.com/dave/choresy/internal/mail"
	"github.com/dave/choresy/internal/schedule"
)

func setupScheduleTest(t *testing.T) (*ScheduleHandler, string, *auth.Service) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	scheduleStore := schedule.NewMemoryStore()
	scheduleService := schedule.NewService()
	handler := NewScheduleHandler(scheduleStore, scheduleService)

	user, session, _ := authService.Register(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"alice@example.com", "password123",
	)
	householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", user.ID,
	)
	return handler, session.ID, authService
}

func TestScheduleListEmpty(t *testing.T) {
	handler, sessionID, authService := setupScheduleTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/schedules", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["schedules"] == nil {
		t.Fatal("response missing 'schedules' key")
	}
}

func TestScheduleCreate(t *testing.T) {
	handler, sessionID, authService := setupScheduleTest(t)
	body := `{"choreId":1,"frequencyType":"daily","timePeriod":"morning"}`
	req := withUser(
		httptest.NewRequest(http.MethodPost, "/api/schedules", strings.NewReader(body)),
		authService, sessionID,
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"morning"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestScheduleCreateMissingChoreID(t *testing.T) {
	handler, sessionID, authService := setupScheduleTest(t)
	body := `{"frequencyType":"daily","timePeriod":"morning"}`
	req := withUser(
		httptest.NewRequest(http.MethodPost, "/api/schedules", strings.NewReader(body)),
		authService, sessionID,
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestScheduleForDate(t *testing.T) {
	handler, sessionID, authService := setupScheduleTest(t)

	// Create a daily schedule first
	createReq := withUser(
		httptest.NewRequest(http.MethodPost, "/api/schedules",
			strings.NewReader(`{"choreId":1,"frequencyType":"daily","timePeriod":"morning"}`)),
		authService, sessionID,
	)
	createReq.Header.Set("Content-Type", "application/json")
	handler.Create(httptest.NewRecorder(), createReq)

	// Now query for-date
	req := withUser(
		httptest.NewRequest(http.MethodGet, "/api/schedules/for-date?date=2026-04-28", nil),
		authService, sessionID,
	)
	rec := httptest.NewRecorder()
	handler.ForDate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"morning"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestScheduleUpdateDelete(t *testing.T) {
	handler, sessionID, authService := setupScheduleTest(t)

	// Create
	createReq := withUser(
		httptest.NewRequest(http.MethodPost, "/api/schedules",
			strings.NewReader(`{"choreId":5,"frequencyType":"daily","timePeriod":"evening"}`)),
		authService, sessionID,
	)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.Create(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	sched := createResp["schedule"].(map[string]any)
	id := int64(sched["id"].(float64))

	// Update
	updateReq := withUser(
		httptest.NewRequest(http.MethodPatch, "/api/schedules/",
			strings.NewReader(`{"frequencyType":"daily","timePeriod":"morning","isActive":true}`)),
		authService, sessionID,
	)
	updateReq.SetPathValue("id", strings.TrimSpace(strings.Replace(strings.Repeat("0", 10)+string(rune('0'+id)), "", "", -1)))
	// Set path value properly
	updateReq2 := withUser(
		httptest.NewRequest(http.MethodPatch, "/api/schedules/1",
			strings.NewReader(`{"frequencyType":"daily","timePeriod":"morning","isActive":true}`)),
		authService, sessionID,
	)
	updateReq2.SetPathValue("id", "1")
	updateReq2.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.Update(updateRec, updateReq2)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updateRec.Code, updateRec.Body.String())
	}

	// Delete
	deleteReq := withUser(
		httptest.NewRequest(http.MethodDelete, "/api/schedules/1", nil),
		authService, sessionID,
	)
	deleteReq.SetPathValue("id", "1")
	deleteRec := httptest.NewRecorder()
	handler.Delete(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestScheduleRequiresHousehold(t *testing.T) {
	handler, _, _ := setupScheduleTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/schedules", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
