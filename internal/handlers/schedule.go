// internal/handlers/schedule.go

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/schedule"
)

// ScheduleHandler handles HTTP requests for schedule CRUD and queries.
type ScheduleHandler struct {
	store       schedule.Store
	service     *schedule.Service
	auditLogger audit.Logger
}

// NewScheduleHandler creates a new ScheduleHandler.
func NewScheduleHandler(store schedule.Store, service *schedule.Service) *ScheduleHandler {
	return &ScheduleHandler{store: store, service: service, auditLogger: audit.NopLogger{}}
}

// SetAuditLogger attaches a sink for schedule mutation events. A nil logger is
// a no-op (the handler keeps its default NopLogger).
func (h *ScheduleHandler) SetAuditLogger(logger audit.Logger) {
	if logger != nil {
		h.auditLogger = logger
	}
}

func (h *ScheduleHandler) logAudit(ctx context.Context, event string, attrs map[string]string) {
	audit.Emit(ctx, h.auditLogger, event, attrs)
}

// List returns all schedules for the user's household.
// GET /api/schedules
func (h *ScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}
	schedules, err := h.store.ListByHousehold(r.Context(), *user.HouseholdID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if schedules == nil {
		schedules = []schedule.ChoreSchedule{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": schedules})
}

// ForDate returns schedules active on a given date.
// GET /api/schedules/for-date?date=YYYY-MM-DD
func (h *ScheduleHandler) ForDate(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	dateStr := r.URL.Query().Get("date")
	date := time.Now().UTC()
	if dateStr != "" {
		if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			date = parsed
		}
	}

	all, err := h.store.ListByHousehold(r.Context(), *user.HouseholdID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	active := h.service.GetSchedulesForDate(all, date)
	if active == nil {
		active = []schedule.ChoreSchedule{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schedules": active,
		"date":      date.Format("2006-01-02"),
	})
}

// Create adds a new schedule entry.
// POST /api/schedules
func (h *ScheduleHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	var req schedule.ChoreSchedule
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ChoreID == 0 {
		writeError(w, http.StatusBadRequest, "choreId is required")
		return
	}
	if req.TimePeriod == "" {
		req.TimePeriod = schedule.PeriodAnytime
	}
	if req.FrequencyType == "" {
		req.FrequencyType = "once"
	}
	req.HouseholdID = *user.HouseholdID
	req.IsActive = true

	created, err := h.store.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logAudit(r.Context(), "schedule.created", map[string]string{
		"schedule_id": strconv.FormatInt(created.ID, 10),
		"chore_id":    strconv.FormatInt(req.ChoreID, 10),
	})
	writeJSON(w, http.StatusCreated, map[string]any{"schedule": created})
}

// Update partially updates a schedule entry.
// Only fields present in the JSON body are modified; all others are preserved
// from the existing record.  This prevents implicit zero-value overwrites (e.g.
// isActive being reset to false when only timePeriod is being changed).
// PATCH /api/schedules/{id}
func (h *ScheduleHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid schedule id")
		return
	}

	existing, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}
	if existing.HouseholdID != *user.HouseholdID {
		writeError(w, http.StatusForbidden, "not your schedule")
		return
	}

	// Decode as a raw map so we can distinguish "field not sent" from "field
	// sent as zero/false/null".  Only keys present in the payload are applied.
	var raw map[string]json.RawMessage
	if err := readJSON(r, &raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Start from the existing record; patch only what was provided.
	req := existing
	req.ID = id
	req.HouseholdID = *user.HouseholdID

	if v, ok := raw["choreId"]; ok {
		_ = json.Unmarshal(v, &req.ChoreID)
	}
	if v, ok := raw["timePeriod"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			req.TimePeriod = schedule.TimePeriod(s)
		}
	}
	if v, ok := raw["specificTime"]; ok {
		if string(v) == "null" {
			req.SpecificTime = ""
		} else {
			_ = json.Unmarshal(v, &req.SpecificTime)
		}
	}
	if v, ok := raw["frequencyType"]; ok {
		_ = json.Unmarshal(v, &req.FrequencyType)
	}
	if v, ok := raw["isActive"]; ok {
		_ = json.Unmarshal(v, &req.IsActive)
	}
	if v, ok := raw["daysOfWeek"]; ok {
		_ = json.Unmarshal(v, &req.DaysOfWeek)
	}
	if v, ok := raw["intervalDays"]; ok {
		_ = json.Unmarshal(v, &req.IntervalDays)
	}
	if v, ok := raw["dayOfMonth"]; ok {
		_ = json.Unmarshal(v, &req.DayOfMonth)
	}
	if v, ok := raw["monthOfYear"]; ok {
		_ = json.Unmarshal(v, &req.MonthOfYear)
	}
	if v, ok := raw["startDate"]; ok {
		if string(v) == "null" {
			req.StartDate = nil
		} else {
			var d schedule.DateOnly
			if json.Unmarshal(v, &d) == nil {
				req.StartDate = &d
			}
		}
	}
	if v, ok := raw["recurrenceEnd"]; ok {
		if string(v) == "null" {
			req.RecurrenceEnd = nil
		} else {
			var t time.Time
			if json.Unmarshal(v, &t) == nil {
				req.RecurrenceEnd = &t
			}
		}
	}

	updated, err := h.store.Update(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logAudit(r.Context(), "schedule.updated", map[string]string{
		"schedule_id": strconv.FormatInt(id, 10),
	})
	writeJSON(w, http.StatusOK, map[string]any{"schedule": updated})
}

// Delete removes a schedule entry.
// DELETE /api/schedules/{id}
func (h *ScheduleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid schedule id")
		return
	}

	existing, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}
	if existing.HouseholdID != *user.HouseholdID {
		writeError(w, http.StatusForbidden, "not your schedule")
		return
	}

	if err := h.store.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logAudit(r.Context(), "schedule.deleted", map[string]string{
		"schedule_id": strconv.FormatInt(id, 10),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
