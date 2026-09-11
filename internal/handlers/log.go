package handlers

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/notification"
	"github.com/HammerMeetNail/nabu/internal/schedule"
)

type LogHandler struct {
	service            *log.Service
	notifService       *notification.Service // optional; nil disables notifications
	deliveryChores     chore.Store
	deliveryHouseholds household.Store
	choreStore         chore.Store
	householdStore     household.Store
	scheduleStore      schedule.Store
	dispatch           func(func())
	background         context.Context
}

// backdateFollowUpTolerance is the grace period used to decide whether a log
// records a fresh/current completion versus a backdated historical entry. A
// log whose completion time falls within this window of "now" is treated as
// current and may clear/replace an existing follow-up; anything older is
// treated as a backdated entry and leaves existing follow-ups untouched.
//
// The value is generous (6 hours) to cover two scenarios that would otherwise
// be misclassified:
//  1. The schedule-tab log sheet pre-fills the when input to the scheduled
//     time (e.g. 15:30); the user may tap Log without updating it even though
//     the wall clock is hours later.
//  2. The user's local date and the server's UTC date differ (timezone
//     offset). A date-based comparison breaks across timezone boundaries,
//     while completedAt vs time.Now() both use absolute UTC timestamps.
const backdateFollowUpTolerance = 6 * time.Hour

func NewLogHandler(service *log.Service) *LogHandler {
	return &LogHandler{service: service, background: context.Background(), dispatch: func(f func()) { f() }}
}

func (h *LogHandler) SetBackground(ctx context.Context, dispatch func(func())) {
	h.background, h.dispatch = ctx, dispatch
}

// WithNotification attaches the services required to fan out chore-logged
// notifications to other household members after a successful log creation.
func (h *LogHandler) WithNotification(ns *notification.Service, cs chore.Store, hs household.Store) *LogHandler {
	h.notifService = ns
	h.deliveryChores, h.deliveryHouseholds = cs, hs
	if h.choreStore == nil {
		h.WithChoreStore(cs, hs)
	}
	return h
}

// WithScheduleStore attaches a schedule store so the handler can manage
// follow-up schedules when logs are created.
func (h *LogHandler) WithScheduleStore(ss schedule.Store) *LogHandler {
	h.scheduleStore = ss
	return h
}

// WithChoreStore attaches the chore and household stores for visibility checks.
func (h *LogHandler) WithChoreStore(cs chore.Store, hs household.Store) *LogHandler {
	h.choreStore = cs
	h.service.WithAccess(cs, hs)
	h.householdStore = hs
	return h
}

func (h *LogHandler) visibleChoreIDs(ctx context.Context, userID, householdID int64) (map[int64]struct{}, error) {
	if h.choreStore == nil {
		return nil, nil
	}
	return chore.NewService(h.choreStore).WithMemberships(h.householdStore).VisibleChoreIDs(ctx, userID, householdID)
}

func (h *LogHandler) canViewChore(ctx context.Context, userID, householdID, choreID int64) (chore.Chore, bool) {
	if h.choreStore == nil {
		return chore.Chore{}, true
	}
	c, err := h.choreStore.GetChore(ctx, choreID)
	if err != nil {
		return chore.Chore{}, false
	}
	if c.HouseholdID != householdID {
		return chore.Chore{}, false
	}
	if c.Visibility == chore.VisibilityAdmins {
		if h.householdStore == nil {
			return c, false
		}
		role, err := h.householdStore.GetMembershipForHousehold(ctx, userID, householdID)
		if err != nil {
			return c, false
		}
		if role != household.RoleOwner && role != household.RoleAdmin {
			return c, false
		}
	}
	return c, true
}

func (h *LogHandler) isAdmin(ctx context.Context, userID, householdID int64) bool {
	if h.householdStore == nil {
		return true
	}
	role, err := h.householdStore.GetMembershipForHousehold(ctx, userID, householdID)
	if err != nil {
		return false
	}
	return role == household.RoleOwner || role == household.RoleAdmin
}

func filterLogsByVisible(logs []log.ChoreLog, visible map[int64]struct{}) []log.ChoreLog {
	if visible == nil {
		return logs
	}
	var out []log.ChoreLog
	for _, l := range logs {
		if _, ok := visible[l.ChoreID]; ok {
			out = append(out, l)
		}
	}
	if out == nil {
		return []log.ChoreLog{}
	}
	return out
}

// fanOutNotification creates notifications for all household members except
// the one attributed on the log and the one who performed the action.
// It is always called in a goroutine so that push / DB latency never delays
// the HTTP response.
func (h *LogHandler) fanOutNotification(householdID, loggerID, actorID, choreID int64) {
	if h.notifService == nil {
		return
	}
	ctx, cancel := context.WithTimeout(h.background, 20*time.Second)
	defer cancel()

	c, err := h.deliveryChores.GetChore(ctx, choreID)
	if err != nil {
		return
	}
	// For Admins-only tasks, only notify other admins/owners.
	isPrivate := c.Visibility == chore.VisibilityAdmins
	members, err := h.deliveryHouseholds.GetMembers(ctx, householdID)
	if err != nil {
		return
	}
	var filtered []household.Member
	if isPrivate {
		for _, m := range members {
			if m.Role == household.RoleOwner || m.Role == household.RoleAdmin {
				filtered = append(filtered, m)
			}
		}
		members = filtered
		if len(members) == 0 {
			return
		}
	}
	mi := make([]notification.MemberInfo, len(members))
	for i, m := range members {
		mi[i] = notification.MemberInfo{UserID: m.UserID, DisplayName: m.DisplayName}
	}
	h.notifService.NotifyChoreLogged(ctx, mi, loggerID, actorID, c.Name, c.Icon, notification.Scope{HouseholdID: householdID, ChoreID: choreID})
}

func (h *LogHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	var req struct {
		ChoreID          int64          `json:"choreId"`
		Title            *string        `json:"title,omitempty"`
		Note             string         `json:"note"`
		Indicators       []string       `json:"indicators"`
		IndicatorVolumes map[string]int `json:"indicatorVolumes"`
		Date             string         `json:"date"`        // optional ISO date "YYYY-MM-DD"; defaults to today
		Hour             *int           `json:"hour"`        // optional calendar slot hour (0-23)
		CompletedAt      string         `json:"completedAt"` // optional RFC3339 timestamp for backdating
		MetricUnit       *string        `json:"metricUnit"`
		VolumeML         *int           `json:"volumeML"`        // optional volume in mL
		Rating           *int           `json:"rating"`          // optional rating 0-50 (tenths of a star)
		DurationSeconds  *int           `json:"durationSeconds"` // optional elapsed seconds for duration-metric chores
		Subject          *string        `json:"subject"`         // optional subject tag (Phase 5.5)
		UserID           *int64         `json:"userId"`          // optional: log on behalf of another household member
		FollowUpMinutes  int            `json:"followUpMinutes"`
		FollowUpTime     string         `json:"followUpTime"`   // local ISO datetime for schedule placement
		IdempotencyKey   string         `json:"idempotencyKey"` // optional client token to de-dup offline replays
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	logUserID := user.ID
	if req.UserID != nil && *req.UserID != user.ID {
		// Verify the requested user is a member of the household.
		if h.householdStore != nil {
			members, err := h.householdStore.GetMembers(r.Context(), *user.HouseholdID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to verify member")
				return
			}
			found := false
			for _, m := range members {
				if m.UserID == *req.UserID {
					found = true
					break
				}
			}
			if !found {
				writeError(w, http.StatusForbidden, "user is not a member of this household")
				return
			}
		}
		logUserID = *req.UserID
	}

	// Verify the chore belongs to this household and is visible to the requester.
	var choreForVis chore.Chore
	if h.choreStore != nil {
		c, err := h.choreStore.GetChore(r.Context(), req.ChoreID)
		if err != nil {
			writeError(w, http.StatusNotFound, "chore not found")
			return
		}
		if c.HouseholdID != *user.HouseholdID {
			writeError(w, http.StatusForbidden, "chore does not belong to your household")
			return
		}
		// Visibility check: member cannot see Admins-only task.
		if c.Visibility == chore.VisibilityAdmins {
			if !h.isAdmin(r.Context(), user.ID, *user.HouseholdID) {
				writeError(w, http.StatusNotFound, "chore not found")
				return
			}
			// Private task may only be attributed to an admin/owner.
			if !h.isAdmin(r.Context(), logUserID, *user.HouseholdID) {
				writeError(w, http.StatusBadRequest, "private task log may only be attributed to an admin")
				return
			}
		}
		choreForVis = c
		_ = choreForVis
	}

	var logDate *time.Time
	if req.Date != "" {
		t, err := time.Parse("2006-01-02", req.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid date format, expected YYYY-MM-DD")
			return
		}
		logDate = &t
	}

	var logCompletedAt *time.Time
	if req.CompletedAt != "" {
		t, err := time.Parse(time.RFC3339, req.CompletedAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid completedAt format, expected RFC3339")
			return
		}
		logCompletedAt = &t
	}

	idemKey := strings.TrimSpace(req.IdempotencyKey)
	if len(idemKey) > 64 {
		writeError(w, http.StatusBadRequest, "idempotencyKey too long")
		return
	}
	if req.FollowUpMinutes < 0 || req.FollowUpMinutes > 7*24*60 {
		writeError(w, http.StatusBadRequest, "invalid follow-up interval")
		return
	}
	if req.FollowUpTime != "" {
		if _, err := time.Parse("2006-01-02T15:04", req.FollowUpTime); err != nil {
			writeError(w, http.StatusBadRequest, "invalid followUpTime format")
			return
		}
	}
	entry, created, err := h.service.LogChoreIdempotent(r.Context(), log.CreateInput{
		HouseholdID: *user.HouseholdID, ActorID: user.ID, UserID: logUserID, ChoreID: req.ChoreID,
		Title: req.Title, Note: req.Note, Indicators: req.Indicators, IndicatorVolumes: req.IndicatorVolumes,
		MetricUnit: stringValue(req.MetricUnit), Date: logDate, SlotHour: req.Hour, CompletedAt: logCompletedAt, VolumeML: req.VolumeML,
		Rating: req.Rating, DurationSeconds: req.DurationSeconds, Subject: req.Subject,
		IdempotencyKey: idemKey, FollowUpMinutes: req.FollowUpMinutes, FollowUpTime: req.FollowUpTime,
	})
	if err != nil {
		// Validation failures are the caller's fault: 400 with the clear
		// message. Everything else gets a static 409 so store errors (e.g.
		// idempotency-key unique races, connection failures) never leak
		// pgx internals.
		if errors.Is(err, log.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, log.ErrNotFound) {
			writeError(w, http.StatusNotFound, "chore not found")
			return
		}
		writeError(w, http.StatusConflict, "could not create log")
		return
	}

	applied := created
	if h.scheduleStore != nil {
		effects := schedule.LogEffects{
			LogID: entry.ID, HouseholdID: *user.HouseholdID, ChoreID: req.ChoreID,
			UpdateFollowUp:      logCompletedAt == nil || !logCompletedAt.Before(entry.CreatedAt.Add(-backdateFollowUpTolerance)),
			LastFollowUpMinutes: req.FollowUpMinutes,
		}
		if effects.UpdateFollowUp && req.FollowUpMinutes > 0 && req.FollowUpTime != "" {
			t, _ := time.Parse("2006-01-02T15:04", req.FollowUpTime) // validated before the log is saved
			effects.FollowUp = &schedule.ChoreSchedule{
				HouseholdID: *user.HouseholdID, ChoreID: req.ChoreID, FrequencyType: "once",
				TimePeriod: schedule.PeriodAnytime, SpecificTime: t.Format("15:04"),
				StartDate: &schedule.DateOnly{Time: t}, IsActive: true, IsFollowUp: true,
			}
		}
		applied, err = h.scheduleStore.ApplyLogEffects(r.Context(), effects)
		if err != nil {
			writeServerError(w, "log saved; retry to finish its follow-up", err)
			return
		}
		// Postgres updates this preference in the effect transaction. Memory
		// mode keeps its separate chore store synchronized after application.
		if _, memory := h.scheduleStore.(*schedule.MemoryStore); memory && applied && effects.UpdateFollowUp && h.choreStore != nil {
			c, err := h.choreStore.GetChore(r.Context(), req.ChoreID)
			if err == nil && c.HouseholdID == *user.HouseholdID {
				c.LastFollowUpMinutes = req.FollowUpMinutes
				_ = h.choreStore.UpdateChore(r.Context(), c)
			}
		}
	}

	// Fire-and-forget: notify other household members.
	if applied && h.notifService != nil {
		hhID := *user.HouseholdID
		loggerID := logUserID
		choreID := req.ChoreID
		h.dispatch(func() { h.fanOutNotification(hhID, loggerID, user.ID, choreID) })
	}

	writeJSON(w, http.StatusCreated, map[string]any{"log": entry})
}

func (h *LogHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid log id")
		return
	}

	var req struct {
		Title            *string        `json:"title,omitempty"`
		Note             string         `json:"note"`
		Indicators       []string       `json:"indicators"`
		IndicatorVolumes map[string]int `json:"indicatorVolumes"`
		MetricUnit       *string        `json:"metricUnit"`
		VolumeML         *int           `json:"volumeML"`
		Rating           *int           `json:"rating"`          // optional rating 0-50 (tenths of a star)
		DurationSeconds  *int           `json:"durationSeconds"` // optional elapsed seconds for duration-metric chores
		Subject          *string        `json:"subject"`         // optional subject tag (Phase 5.5)
		UserID           *int64         `json:"userId"`          // optional: change who the log is attributed to
		CompletedAt      string         `json:"completedAt"`     // optional: new completion timestamp
		Hour             *int           `json:"hour"`            // optional: new slot hour
		Date             string         `json:"date"`            // optional: new log date
	}
	rawFields, err := readPatchJSON(r, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	fields := log.LogFields{}
	for name := range rawFields {
		fields[name] = true
	}
	if fields["userId"] && req.UserID == nil || fields["completedAt"] && req.CompletedAt == "" {
		writeError(w, http.StatusBadRequest, "userId and completedAt cannot be null or empty")
		return
	}

	// If changing userId, verify the target user is a household member and for private tasks, admin.
	var userID *int64
	if req.UserID != nil {
		if h.householdStore != nil && user.HouseholdID != nil {
			members, err := h.householdStore.GetMembers(r.Context(), *user.HouseholdID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to verify member")
				return
			}
			found := false
			for _, m := range members {
				if m.UserID == *req.UserID {
					found = true
					break
				}
			}
			if !found {
				writeError(w, http.StatusForbidden, "user is not a member of this household")
				return
			}
		}
		userID = req.UserID
	}

	// Visibility check: resolve the log and its chore before mutation.
	if h.choreStore != nil {
		existingLog, err := h.service.GetLogForHousehold(r.Context(), id, *user.HouseholdID)
		if err != nil {
			writeError(w, http.StatusNotFound, "log not found")
			return
		}
		if _, ok := h.canViewChore(r.Context(), user.ID, *user.HouseholdID, existingLog.ChoreID); !ok {
			writeError(w, http.StatusNotFound, "log not found")
			return
		}
		// If re-assigning to a different user, private chore may only be attributed to admin.
		if userID != nil {
			if c, ok := h.canViewChore(r.Context(), user.ID, *user.HouseholdID, existingLog.ChoreID); ok && c.Visibility == chore.VisibilityAdmins {
				if !h.isAdmin(r.Context(), *userID, *user.HouseholdID) {
					writeError(w, http.StatusBadRequest, "private task log may only be attributed to an admin")
					return
				}
			}
		}
	}

	var logDate *time.Time
	if req.Date != "" {
		t, err := time.Parse("2006-01-02", req.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid date format, expected YYYY-MM-DD")
			return
		}
		logDate = &t
	}

	var logCompletedAt *time.Time
	if req.CompletedAt != "" {
		t, err := time.Parse(time.RFC3339, req.CompletedAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid completedAt format, expected RFC3339")
			return
		}
		logCompletedAt = &t
	}

	if err := h.service.UpdateLog(r.Context(), id, *user.HouseholdID, req.Title, req.Note, req.Indicators, req.IndicatorVolumes, req.VolumeML, userID, logCompletedAt, req.Hour, logDate, req.Rating, req.DurationSeconds, req.Subject, log.Patch{ActorID: user.ID, Fields: fields, MetricUnit: req.MetricUnit}); err != nil {
		if errors.Is(err, log.ErrNotFound) {
			writeError(w, http.StatusNotFound, "log not found")
			return
		}
		if errors.Is(err, log.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeServerError(w, "failed to update log", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *LogHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid log id")
		return
	}

	if h.choreStore != nil {
		existingLog, err := h.service.GetLogForHousehold(r.Context(), id, *user.HouseholdID)
		if err != nil {
			writeError(w, http.StatusNotFound, "log not found")
			return
		}
		if _, ok := h.canViewChore(r.Context(), user.ID, *user.HouseholdID, existingLog.ChoreID); !ok {
			writeError(w, http.StatusNotFound, "log not found")
			return
		}
	}

	if err := h.service.UndoLog(r.Context(), *user.HouseholdID, id); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *LogHandler) Today(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}
	visible, err := h.visibleChoreIDs(r.Context(), user.ID, *user.HouseholdID)
	if err != nil {
		writeServerError(w, "failed to verify chore access", err)
		return
	}
	readCtx := r.Context()
	if h.choreStore != nil {
		readCtx = log.WithReadAccess(readCtx, *user.HouseholdID, user.ID, visible)
	}

	dateStr := r.URL.Query().Get("date")
	date := today()
	if dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err == nil {
			date = parsed
		}
	}

	logs, err := h.service.GetDayLogs(readCtx, *user.HouseholdID, date)
	if err != nil {
		writeServerError(w, "failed to load today's logs", err)
		return
	}
	if logs == nil {
		logs = []log.ChoreLog{}
	}
	// Filter by visible chores before summary.
	logs = filterLogsByVisible(logs, visible)

	summary := h.service.DailySummaryFromLogs(date, logs)

	writeJSON(w, http.StatusOK, map[string]any{
		"logs":    logs,
		"summary": summary,
		"date":    date.Format("2006-01-02"),
	})
}

func (h *LogHandler) Week(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}
	visible, err := h.visibleChoreIDs(r.Context(), user.ID, *user.HouseholdID)
	if err != nil {
		writeServerError(w, "failed to verify chore access", err)
		return
	}
	readCtx := r.Context()
	if h.choreStore != nil {
		readCtx = log.WithReadAccess(readCtx, *user.HouseholdID, user.ID, visible)
	}

	startStr := r.URL.Query().Get("start")
	start := today()
	if startStr != "" {
		parsed, err := time.Parse("2006-01-02", startStr)
		if err == nil {
			start = parsed
		}
	}

	logs, err := h.service.GetWeekLogs(readCtx, *user.HouseholdID, start)
	if err != nil {
		writeServerError(w, "failed to load week logs", err)
		return
	}
	logs = filterLogsByVisible(logs, visible)

	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

func (h *LogHandler) Month(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}
	visible, err := h.visibleChoreIDs(r.Context(), user.ID, *user.HouseholdID)
	if err != nil {
		writeServerError(w, "failed to verify chore access", err)
		return
	}
	readCtx := r.Context()
	if h.choreStore != nil {
		readCtx = log.WithReadAccess(readCtx, *user.HouseholdID, user.ID, visible)
	}

	yearStr := r.URL.Query().Get("year")
	monthStr := r.URL.Query().Get("month")
	year := today().Year()
	month := 1

	if y, err := strconv.Atoi(yearStr); err == nil {
		year = y
	}
	if m, err := strconv.Atoi(monthStr); err == nil {
		month = m
	}

	logs, err := h.service.GetMonthLogs(readCtx, *user.HouseholdID, year, time.Month(month))
	if err != nil {
		writeServerError(w, "failed to load month logs", err)
		return
	}
	logs = filterLogsByVisible(logs, visible)

	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

func (h *LogHandler) History(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}
	visible, err := h.visibleChoreIDs(r.Context(), user.ID, *user.HouseholdID)
	if err != nil {
		writeServerError(w, "failed to verify chore access", err)
		return
	}
	readCtx := r.Context()
	if h.choreStore != nil {
		readCtx = log.WithReadAccess(readCtx, *user.HouseholdID, user.ID, visible)
	}

	// Text search across note/title spans all history and bypasses the
	// windowed pagination — search results are a flat, capped, newest-first
	// list.
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		if len(q) > 100 {
			q = q[:100]
		}
		logs, err := h.service.SearchHistoryLogs(readCtx, *user.HouseholdID, q, 100)
		if err != nil {
			writeServerError(w, "failed to search history", err)
			return
		}
		logs = filterLogsByVisible(logs, visible)
		writeJSON(w, http.StatusOK, map[string]any{
			"logs":    logs,
			"hasMore": false,
			"query":   q,
		})
		return
	}

	var before time.Time
	beforeStr := r.URL.Query().Get("before")
	if beforeStr != "" {
		parsed, err := time.Parse("2006-01-02", beforeStr)
		if err == nil {
			before = parsed
		}
	}
	if before.IsZero() {
		before = today().AddDate(0, 0, 1)
	}
	end := before
	if end.After(today().AddDate(0, 0, 1)) {
		end = today().AddDate(0, 0, 1)
	}
	start := end.AddDate(0, 0, -7)

	logs, hasMore, err := h.service.GetHistoryLogs(readCtx, *user.HouseholdID, start, end)
	if err != nil {
		writeServerError(w, "failed to load history", err)
		return
	}
	logs = filterLogsByVisible(logs, visible)

	writeJSON(w, http.StatusOK, map[string]any{
		"logs":    logs,
		"hasMore": hasMore,
		"start":   start.Format("2006-01-02"),
		"end":     end.Format("2006-01-02"),
	})
}

func today() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// csvSafe defeats spreadsheet formula injection (CSV injection): any cell
// whose first character a spreadsheet would treat as a formula gets a leading
// apostrophe, which Excel/Sheets render literally. Apply to every string cell
// that carries user-controlled content before csv.Writer.Write.
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// Export streams the household's logs as CSV for a date range (and optional
// chore). GET /api/logs/export?start=YYYY-MM-DD&end=YYYY-MM-DD&choreId=N.
// Useful for pediatrician visits and spreadsheets.
func (h *LogHandler) Export(w http.ResponseWriter, r *http.Request) {
	r, cancel := exportContext(r)
	defer cancel()
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}
	visible, err := h.visibleChoreIDs(r.Context(), user.ID, *user.HouseholdID)
	if err != nil {
		exportError(w, err)
		return
	}
	readCtx := r.Context()
	if h.choreStore != nil {
		readCtx = log.WithReadAccess(readCtx, *user.HouseholdID, user.ID, visible)
	}

	hid := *user.HouseholdID

	parseDay := func(s string) (time.Time, bool) {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return time.Time{}, false
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
	}
	end := today().AddDate(0, 0, 1)
	if s := r.URL.Query().Get("end"); s != "" {
		if t, ok := parseDay(s); ok {
			end = t.AddDate(0, 0, 1) // inclusive of the end day
		} else {
			writeError(w, http.StatusBadRequest, "invalid end date")
			return
		}
	}
	start := end.AddDate(0, 0, -30)
	if s := r.URL.Query().Get("start"); s != "" {
		if t, ok := parseDay(s); ok {
			start = t
		} else {
			writeError(w, http.StatusBadRequest, "invalid start date")
			return
		}
	}
	if !start.Before(end) {
		writeError(w, http.StatusBadRequest, "start must be before end")
		return
	}

	var filterChoreID int64
	if s := r.URL.Query().Get("choreId"); s != "" {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid choreId")
			return
		}
		// Visibility check: the chore must be visible to the caller.
		if h.choreStore != nil {
			if _, ok := h.canViewChore(r.Context(), user.ID, hid, id); !ok {
				writeError(w, http.StatusNotFound, "chore not found")
				return
			}
		}
		filterChoreID = id
		if h.choreStore != nil {
			readCtx = log.WithReadAccess(readCtx, hid, user.ID, map[int64]struct{}{id: {}})
		}
	}

	logs, err := h.service.GetLogsInRange(readCtx, hid, start, end)
	if err != nil {
		exportError(w, err)
		return
	}
	logs = filterLogsByVisible(logs, visible)

	// Build id→name lookups for chores and members (only visible chores).
	choreNames := map[int64]string{}
	if h.choreStore != nil {
		chores, err := h.choreStore.ListChores(r.Context(), hid)
		if err != nil {
			exportError(w, err)
			return
		}
		for _, c := range chores {
			if visible != nil {
				if _, ok := visible[c.ID]; !ok {
					continue
				}
			}
			choreNames[c.ID] = c.Name
		}
	}
	memberNames := map[int64]string{}
	if h.householdStore != nil {
		members, err := h.householdStore.GetMembers(r.Context(), hid)
		if err != nil {
			exportError(w, err)
			return
		}
		{
			for _, m := range members {
				name := m.DisplayName
				if name == "" {
					name = m.Email
				}
				memberNames[m.UserID] = name
			}
		}
	}

	buffer := &exportBuffer{ctx: r.Context()}
	cw := csv.NewWriter(buffer)
	_ = cw.Write([]string{"date", "time", "chore", "member", "title", "note", "volume_ml", "indicators", "indicator_volumes", "rating", "duration_seconds", "subject", "metric_unit"})
	for _, l := range logs {
		if filterChoreID != 0 && l.ChoreID != filterChoreID {
			continue
		}
		ts := l.CompletedAt.UTC()
		title := ""
		if l.Title != nil {
			title = *l.Title
		}
		vol := ""
		if l.VolumeML != nil {
			vol = strconv.Itoa(*l.VolumeML)
		}
		rating := ""
		if l.Rating != nil {
			rating = strconv.FormatFloat(float64(*l.Rating)/10.0, 'f', -1, 64)
		}
		durationSec := ""
		if l.DurationSeconds != nil {
			durationSec = strconv.Itoa(*l.DurationSeconds)
		}
		subject := ""
		if l.Subject != nil {
			subject = *l.Subject
		}
		indVol := ""
		if len(l.IndicatorVolumes) > 0 {
			parts := make([]string, 0, len(l.IndicatorVolumes))
			for k, v := range l.IndicatorVolumes {
				parts = append(parts, fmt.Sprintf("%s=%d", k, v))
			}
			sort.Strings(parts)
			indVol = strings.Join(parts, "; ")
		}
		_ = cw.Write([]string{
			csvSafe(ts.Format("2006-01-02")),
			csvSafe(ts.Format("15:04")),
			csvSafe(choreNames[l.ChoreID]),
			csvSafe(memberNames[l.UserID]),
			csvSafe(title),
			csvSafe(l.Note),
			csvSafe(vol),
			csvSafe(strings.Join(l.Indicators, "; ")),
			csvSafe(indVol),
			csvSafe(rating),
			csvSafe(durationSec),
			csvSafe(subject),
			csvSafe(l.MetricUnit),
		})
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		exportError(w, err)
		return
	}
	// Metadata is fetched after the log query. Recheck access after every read
	// so a visibility change cannot expose a newly private chore name.
	if h.choreStore != nil {
		current, err := h.visibleChoreIDs(r.Context(), user.ID, hid)
		if err != nil {
			exportError(w, err)
			return
		}
		for _, entry := range logs {
			if filterChoreID != 0 && entry.ChoreID != filterChoreID {
				continue
			}
			if _, ok := current[entry.ChoreID]; !ok {
				writeError(w, http.StatusForbidden, "chore access changed; try again")
				return
			}
		}
	}
	sendExport(w, r, buffer, "nabu-logs.csv")
}

func (h *LogHandler) LatestPerChore(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}
	visible, err := h.visibleChoreIDs(r.Context(), user.ID, *user.HouseholdID)
	if err != nil {
		writeServerError(w, "failed to verify chore access", err)
		return
	}
	readCtx := r.Context()
	if h.choreStore != nil {
		readCtx = log.WithReadAccess(readCtx, *user.HouseholdID, user.ID, visible)
	}

	result, err := h.service.LatestPerChore(readCtx, *user.HouseholdID)
	if err != nil {
		writeServerError(w, "failed to load latest logs", err)
		return
	}
	if visible != nil {
		for cid := range result {
			if _, ok := visible[cid]; !ok {
				delete(result, cid)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"latestLogs": result})
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
