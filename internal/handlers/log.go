package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/notification"
	"github.com/HammerMeetNail/nabu/internal/schedule"
)

type LogHandler struct {
	service        *log.Service
	notifService   *notification.Service // optional; nil disables notifications
	choreStore     chore.Store
	householdStore household.Store
	scheduleStore  schedule.Store
}

func NewLogHandler(service *log.Service) *LogHandler {
	return &LogHandler{service: service}
}

// WithNotification attaches the services required to fan out chore-logged
// notifications to other household members after a successful log creation.
func (h *LogHandler) WithNotification(ns *notification.Service, cs chore.Store, hs household.Store) *LogHandler {
	h.notifService = ns
	h.choreStore = cs
	h.householdStore = hs
	return h
}

// WithScheduleStore attaches a schedule store so the handler can manage
// follow-up schedules when logs are created.
func (h *LogHandler) WithScheduleStore(ss schedule.Store) *LogHandler {
	h.scheduleStore = ss
	return h
}

// fanOutNotification creates notifications for all household members except
// the one attributed on the log and the one who performed the action.
// It is always called in a goroutine so that push / DB latency never delays
// the HTTP response.
func (h *LogHandler) fanOutNotification(householdID, loggerID, actorID, choreID int64) {
	if h.notifService == nil {
		return
	}
	ctx := context.Background()

	c, err := h.choreStore.GetChore(ctx, choreID)
	if err != nil {
		return
	}
	members, err := h.householdStore.GetMembers(ctx, householdID)
	if err != nil {
		return
	}
	mi := make([]notification.MemberInfo, len(members))
	for i, m := range members {
		mi[i] = notification.MemberInfo{UserID: m.UserID, DisplayName: m.DisplayName}
	}
	h.notifService.NotifyChoreLogged(ctx, mi, loggerID, actorID, c.Name, c.Icon)
}

func (h *LogHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	var req struct {
		ChoreID          int64          `json:"choreId"`
		Note             string         `json:"note"`
		Indicators       []string       `json:"indicators"`
		IndicatorVolumes map[string]int `json:"indicatorVolumes"`
		Date             string         `json:"date"`        // optional ISO date "YYYY-MM-DD"; defaults to today
		Hour             *int           `json:"hour"`        // optional calendar slot hour (0-23)
		CompletedAt      string         `json:"completedAt"` // optional RFC3339 timestamp for backdating
		VolumeML         *int           `json:"volumeML"`    // optional volume in mL
		UserID           *int64         `json:"userId"`      // optional: log on behalf of another household member
		FollowUpMinutes  int            `json:"followUpMinutes"`
		FollowUpTime     string         `json:"followUpTime"` // local ISO datetime for schedule placement
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

	// Verify the chore belongs to this household (defense in depth).
	if h.choreStore != nil {
		chore, err := h.choreStore.GetChore(r.Context(), req.ChoreID)
		if err != nil {
			writeError(w, http.StatusNotFound, "chore not found")
			return
		}
		if chore.HouseholdID != *user.HouseholdID {
			writeError(w, http.StatusForbidden, "chore does not belong to your household")
			return
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

	entry, err := h.service.LogChore(r.Context(), *user.HouseholdID, logUserID, req.ChoreID, req.Note, req.Indicators, req.IndicatorVolumes, logDate, req.Hour, logCompletedAt, req.VolumeML)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	if h.scheduleStore != nil {
		if err := h.scheduleStore.DeleteFollowUpSchedulesByChore(r.Context(), req.ChoreID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if req.FollowUpMinutes > 0 && req.FollowUpTime != "" {
			t, err := time.Parse("2006-01-02T15:04", req.FollowUpTime)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid followUpTime format")
				return
			}
			specificTime := t.Format("15:04")
			startDate := schedule.DateOnly{Time: t.Truncate(24 * time.Hour)}
			_, err = h.scheduleStore.Create(r.Context(), schedule.ChoreSchedule{
				HouseholdID:   *user.HouseholdID,
				ChoreID:       req.ChoreID,
				FrequencyType: "once",
				TimePeriod:    schedule.PeriodAnytime,
				SpecificTime:  specificTime,
				StartDate:     &startDate,
				IsActive:      true,
				IsFollowUp:    true,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if h.choreStore != nil {
			c, err := h.choreStore.GetChore(r.Context(), req.ChoreID)
			if err == nil && c.HouseholdID == *user.HouseholdID {
				c.LastFollowUpMinutes = req.FollowUpMinutes
				_ = h.choreStore.UpdateChore(r.Context(), c)
			}
		}
	}

	// Fire-and-forget: notify other household members.
	if h.notifService != nil {
		hhID := *user.HouseholdID
		loggerID := logUserID
		choreID := req.ChoreID
		go h.fanOutNotification(hhID, loggerID, user.ID, choreID)
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
		Note             string         `json:"note"`
		Indicators       []string       `json:"indicators"`
		IndicatorVolumes map[string]int `json:"indicatorVolumes"`
		VolumeML         *int           `json:"volumeML"`
		UserID           *int64         `json:"userId"`      // optional: change who the log is attributed to
		CompletedAt      string         `json:"completedAt"` // optional: new completion timestamp
		Hour             *int           `json:"hour"`        // optional: new slot hour
		Date             string         `json:"date"`        // optional: new log date
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// If changing userId, verify the target user is a household member.
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

	if err := h.service.UpdateLog(r.Context(), id, *user.HouseholdID, req.Note, req.Indicators, req.IndicatorVolumes, req.VolumeML, userID, logCompletedAt, req.Hour, logDate); err != nil {
		if errors.Is(err, log.ErrNotFound) {
			writeError(w, http.StatusNotFound, "log not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
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

	dateStr := r.URL.Query().Get("date")
	date := today()
	if dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err == nil {
			date = parsed
		}
	}

	logs, err := h.service.GetDayLogs(r.Context(), *user.HouseholdID, date)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if logs == nil {
		logs = []log.ChoreLog{}
	}

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

	startStr := r.URL.Query().Get("start")
	start := today()
	if startStr != "" {
		parsed, err := time.Parse("2006-01-02", startStr)
		if err == nil {
			start = parsed
		}
	}

	logs, err := h.service.GetWeekLogs(r.Context(), *user.HouseholdID, start)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

func (h *LogHandler) Month(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
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

	logs, err := h.service.GetMonthLogs(r.Context(), *user.HouseholdID, year, time.Month(month))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

func (h *LogHandler) History(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
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

	logs, hasMore, err := h.service.GetHistoryLogs(r.Context(), *user.HouseholdID, start, end)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

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

func (h *LogHandler) LatestPerChore(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}
	result, err := h.service.LatestPerChore(r.Context(), *user.HouseholdID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"latestLogs": result})
}
