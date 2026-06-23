package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/mail"
	"github.com/HammerMeetNail/nabu/internal/schedule"
)

// setupScheduleAuditTest mirrors setupScheduleTest but returns the handler, the
// auth service, the schedule store, a session id, an audit recorder, and the
// authenticated user's id + household id, all from the same setup so tests can
// assert the actor recorded in audit events.
func setupScheduleAuditTest(t *testing.T) (*ScheduleHandler, *auth.Service, schedule.Store, string, *audit.Recorder, int64, int64) {
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
	rec := audit.NewRecorder()
	handler.SetAuditLogger(rec)

	user, session := quickRegister(authService, "audit-alice@example.com")
	if _, err := householdService.CreateHousehold(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"My Home", "", user.ID,
	); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	authed, err := authService.Authenticate(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if authed.HouseholdID == nil {
		t.Fatal("test user has no household")
	}
	return handler, authService, scheduleStore, session.ID, rec, authed.ID, *authed.HouseholdID
}

// requestWithUser runs a request through the session middleware (like withUser)
// so the audit actor is stashed in context, exactly as in production.
func scheduleRequest(authService *auth.Service, sessionID, method, target string, body string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	r.Header.Set("Content-Type", "application/json")
	return withUser(r, authService, sessionID)
}

func createScheduleViaHandler(t *testing.T, handler *ScheduleHandler, authSvc *auth.Service, sessionID string) int64 {
	t.Helper()
	rec := httptest.NewRecorder()
	req := scheduleRequest(authSvc, sessionID, http.MethodPost, "/api/schedules", `{"choreId":3,"frequencyType":"daily","timePeriod":"morning"}`)
	handler.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create schedule: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Schedule struct {
			ID int64 `json:"id"`
		} `json:"schedule"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal create resp: %v", err)
	}
	return resp.Schedule.ID
}

func TestScheduleAudit_Create(t *testing.T) {
	handler, authSvc, _, sessionID, rec, userID, hhID := setupScheduleAuditTest(t)

	id := createScheduleViaHandler(t, handler, authSvc, sessionID)

	ev, ok := rec.Find("schedule.created")
	if !ok {
		t.Fatalf("missing schedule.created; events=%#v", rec.Events())
	}
	if ev.Attrs["chore_id"] != "3" {
		t.Errorf("chore_id = %q, want 3", ev.Attrs["chore_id"])
	}
	if ev.Attrs["schedule_id"] != itoa(id) {
		t.Errorf("schedule_id = %q, want %d", ev.Attrs["schedule_id"], id)
	}
	if ev.Attrs["user_id"] != itoa(userID) {
		t.Errorf("user_id (actor) = %q, want %d", ev.Attrs["user_id"], userID)
	}
	if ev.Attrs["household_id"] != itoa(hhID) {
		t.Errorf("household_id = %q, want %d", ev.Attrs["household_id"], hhID)
	}
}

func TestScheduleAudit_Update(t *testing.T) {
	handler, authSvc, _, sessionID, rec, userID, hhID := setupScheduleAuditTest(t)

	id := createScheduleViaHandler(t, handler, authSvc, sessionID)

	req := scheduleRequest(authSvc, sessionID, http.MethodPatch, "/api/schedules/"+itoa(id), `{"timePeriod":"evening"}`)
	req.SetPathValue("id", itoa(id))
	handler.Update(httptest.NewRecorder(), req)

	ev, ok := rec.Find("schedule.updated")
	if !ok {
		t.Fatalf("missing schedule.updated; events=%#v", rec.Events())
	}
	if ev.Attrs["schedule_id"] != itoa(id) {
		t.Errorf("schedule_id = %q, want %d", ev.Attrs["schedule_id"], id)
	}
	// Actor enrichment must still capture who did it.
	if ev.Attrs["user_id"] != itoa(userID) {
		t.Errorf("user_id = %q, want %d", ev.Attrs["user_id"], userID)
	}
	if ev.Attrs["household_id"] != itoa(hhID) {
		t.Errorf("household_id = %q, want %d", ev.Attrs["household_id"], hhID)
	}
}

func TestScheduleAudit_Delete(t *testing.T) {
	handler, authSvc, _, sessionID, rec, userID, hhID := setupScheduleAuditTest(t)

	id := createScheduleViaHandler(t, handler, authSvc, sessionID)

	req := scheduleRequest(authSvc, sessionID, http.MethodDelete, "/api/schedules/"+itoa(id), "")
	req.SetPathValue("id", itoa(id))
	handler.Delete(httptest.NewRecorder(), req)

	ev, ok := rec.Find("schedule.deleted")
	if !ok {
		t.Fatalf("missing schedule.deleted; events=%#v", rec.Events())
	}
	if ev.Attrs["schedule_id"] != itoa(id) {
		t.Errorf("schedule_id = %q, want %d", ev.Attrs["schedule_id"], id)
	}
	if ev.Attrs["user_id"] != itoa(userID) {
		t.Errorf("user_id = %q, want %d", ev.Attrs["user_id"], userID)
	}
	if ev.Attrs["household_id"] != itoa(hhID) {
		t.Errorf("household_id = %q, want %d", ev.Attrs["household_id"], hhID)
	}
}

func TestScheduleAudit_CrossHouseholdDeleteNotAudited(t *testing.T) {
	handler, authSvc, store, sessionID, rec, _, hhID := setupScheduleAuditTest(t)

	// A schedule owned by a different household, created directly in the store.
	otherHH := hhID + 1000
	other, err := store.Create(context.Background(), schedule.ChoreSchedule{
		HouseholdID: otherHH, ChoreID: 1, FrequencyType: "daily", IsActive: true,
	})
	if err != nil {
		t.Fatalf("store.Create: %v", err)
	}

	req := scheduleRequest(authSvc, sessionID, http.MethodDelete, "/api/schedules/"+itoa(other.ID), "")
	req.SetPathValue("id", itoa(other.ID))
	resp := httptest.NewRecorder()
	handler.Delete(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-household delete, got %d", resp.Code)
	}
	if ev, ok := rec.Find("schedule.deleted"); ok {
		t.Fatalf("expected no audit event for rejected delete; got %#v", ev)
	}
}

func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
