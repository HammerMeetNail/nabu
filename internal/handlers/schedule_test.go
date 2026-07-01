// internal/handlers/schedule_test.go

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/mail"
	"github.com/HammerMeetNail/nabu/internal/schedule"
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

	user, session := quickRegister(authService, "alice@example.com")
	if _, err := householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", "", user.ID,
	); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	return handler, session.ID, authService
}

// TestScheduleCreateRejectsForeignChore verifies that a user cannot create a
// schedule referencing a chore owned by another household (cross-household IDOR).
func TestScheduleCreateRejectsForeignChore(t *testing.T) {
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)

	choreStore := chore.NewMemoryStore()
	scheduleStore := schedule.NewMemoryStore()
	handler := NewScheduleHandler(scheduleStore, schedule.NewService()).WithChoreStore(choreStore)

	user, session := quickRegister(authService, "alice@example.com")
	ctx := context.Background()
	hh, err := householdService.CreateHousehold(ctx, "My Home", "", user.ID)
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	ownChore, err := choreStore.CreateChore(ctx, chore.Chore{HouseholdID: hh.ID, Name: "Dishes"})
	if err != nil {
		t.Fatalf("create own chore: %v", err)
	}
	foreignChore, err := choreStore.CreateChore(ctx, chore.Chore{HouseholdID: hh.ID + 999, Name: "Litter"})
	if err != nil {
		t.Fatalf("create foreign chore: %v", err)
	}

	// Referencing another household's chore must be rejected.
	body := fmt.Sprintf(`{"choreId":%d,"frequencyType":"daily","timePeriod":"morning"}`, foreignChore.ID)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/schedules", strings.NewReader(body)), authService, session.ID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign chore: expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}

	// Referencing an owned chore must succeed.
	body = fmt.Sprintf(`{"choreId":%d,"frequencyType":"daily","timePeriod":"morning"}`, ownChore.ID)
	req = withUser(httptest.NewRequest(http.MethodPost, "/api/schedules", strings.NewReader(body)), authService, session.ID)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("own chore: expected 201, got %d, body=%s", rec.Code, rec.Body.String())
	}
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
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	sched := createResp["schedule"].(map[string]any)
	id := int64(sched["id"].(float64))

	// Update
	updateReq := withUser(
		httptest.NewRequest(http.MethodPatch, "/api/schedules/",
			strings.NewReader(`{"frequencyType":"daily","timePeriod":"morning","isActive":true}`)),
		authService, sessionID,
	)
	updateReq.SetPathValue("id", strings.TrimSpace(strings.ReplaceAll(strings.Repeat("0", 10)+string(rune('0'+id)), "", "")))
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

func TestScheduleForDateNoHousehold(t *testing.T) {
	handler, _, _ := setupScheduleTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/schedules/for-date?date=2026-01-01", nil)
	rec := httptest.NewRecorder()
	handler.ForDate(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestScheduleCreateNoHousehold(t *testing.T) {
	handler, _, _ := setupScheduleTest(t)
	req := httptest.NewRequest(http.MethodPost, "/api/schedules",
		strings.NewReader(`{"choreId":1,"frequencyType":"daily","timePeriod":"morning"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestScheduleCreateInvalidBody(t *testing.T) {
	handler, sessionID, authService := setupScheduleTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/schedules",
		strings.NewReader(`{{{invalid`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestScheduleUpdateNoHousehold(t *testing.T) {
	handler, _, _ := setupScheduleTest(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/schedules/1",
		strings.NewReader(`{"timePeriod":"morning"}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestScheduleUpdateInvalidID(t *testing.T) {
	handler, sessionID, authService := setupScheduleTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/schedules/abc",
		strings.NewReader(`{"timePeriod":"morning"}`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestScheduleUpdateNotFound(t *testing.T) {
	handler, sessionID, authService := setupScheduleTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/schedules/9999",
		strings.NewReader(`{"timePeriod":"morning"}`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "9999")
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestScheduleUpdateInvalidBody(t *testing.T) {
	// Create a schedule first so store.Get succeeds
	handler, sessionID, authService := setupScheduleTest(t)
	createReq := withUser(httptest.NewRequest(http.MethodPost, "/api/schedules",
		strings.NewReader(`{"choreId":3,"frequencyType":"daily","timePeriod":"morning"}`)),
		authService, sessionID)
	createReq.Header.Set("Content-Type", "application/json")
	handler.Create(httptest.NewRecorder(), createReq)

	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/schedules/1",
		strings.NewReader(`{{{invalid`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestScheduleUpdateAllFields(t *testing.T) {
	handler, sessionID, authService := setupScheduleTest(t)
	createReq := withUser(httptest.NewRequest(http.MethodPost, "/api/schedules",
		strings.NewReader(`{"choreId":7,"frequencyType":"daily","timePeriod":"morning"}`)),
		authService, sessionID)
	createReq.Header.Set("Content-Type", "application/json")
	handler.Create(httptest.NewRecorder(), createReq)

	body := `{
		"choreId":8,
		"timePeriod":"evening",
		"specificTime":"20:00",
		"frequencyType":"weekly",
		"isActive":false,
		"daysOfWeek":[1,3,5],
		"intervalDays":3,
		"dayOfMonth":15,
		"monthOfYear":6,
		"startDate":"2026-01-01",
		"recurrenceEnd":"2026-12-31T00:00:00Z"
	}`
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/schedules/1",
		strings.NewReader(body)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestScheduleUpdateNullFields(t *testing.T) {
	handler, sessionID, authService := setupScheduleTest(t)
	createReq := withUser(httptest.NewRequest(http.MethodPost, "/api/schedules",
		strings.NewReader(`{"choreId":9,"frequencyType":"daily","timePeriod":"morning"}`)),
		authService, sessionID)
	createReq.Header.Set("Content-Type", "application/json")
	handler.Create(httptest.NewRecorder(), createReq)

	body := `{"specificTime":null,"startDate":null,"recurrenceEnd":null}`
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/schedules/1",
		strings.NewReader(body)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestScheduleDeleteNoHousehold(t *testing.T) {
	handler, _, _ := setupScheduleTest(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/schedules/1", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handler.Delete(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestScheduleDeleteInvalidID(t *testing.T) {
	handler, sessionID, authService := setupScheduleTest(t)
	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/schedules/abc", nil),
		authService, sessionID)
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()
	handler.Delete(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestScheduleDeleteNotFound(t *testing.T) {
	handler, sessionID, authService := setupScheduleTest(t)
	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/schedules/9999", nil),
		authService, sessionID)
	req.SetPathValue("id", "9999")
	rec := httptest.NewRecorder()
	handler.Delete(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
}
