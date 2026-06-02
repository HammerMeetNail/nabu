package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/stats"
	"github.com/HammerMeetNail/nabu/internal/userprefs"
)

type StatsHandler struct {
	service    *stats.Service
	prefsStore userprefs.Store
}

func NewStatsHandler(service *stats.Service, prefsStore userprefs.Store) *StatsHandler {
	return &StatsHandler{service: service, prefsStore: prefsStore}
}

func (h *StatsHandler) userLocation(r *http.Request) *time.Location {
	if h.prefsStore == nil {
		return time.UTC
	}
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		return time.UTC
	}
	prefs, err := h.prefsStore.Get(r.Context(), user.ID)
	if err != nil || prefs.Timezone == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(prefs.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func (h *StatsHandler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "week"
	}

	var board []stats.LeaderboardEntry
	var err error

	switch period {
	case "month":
		loc := h.userLocation(r)
		now := nowInLoc(loc)
		board, err = h.service.GetMonthlyLeaderboard(r.Context(), *user.HouseholdID, now.Year(), now.Month(), loc)
	default:
		board, err = h.service.GetWeeklyLeaderboard(r.Context(), *user.HouseholdID, h.userLocation(r))
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"leaderboard": board})
}

func (h *StatsHandler) Streaks(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	streaks, err := h.service.GetUserStreaks(r.Context(), *user.HouseholdID, user.ID, h.userLocation(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"streaks": streaks})
}

func (h *StatsHandler) Heatmap(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	loc := h.userLocation(r)
	now := nowInLoc(loc)
	year, month, day := now.Date()
	midnight := time.Date(year, month, day, 0, 0, 0, 0, loc)
	start := midnight.AddDate(0, -3, 0)
	end := midnight.AddDate(0, 0, 1)

	if startStr != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", startStr, loc); err == nil {
			start = parsed
		}
	}
	if endStr != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", endStr, loc); err == nil {
			end = parsed
		}
	}

	cells, err := h.service.GetHeatmap(r.Context(), *user.HouseholdID, start, end, loc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"heatmap": cells})
}

func (h *StatsHandler) Breakdown(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	loc := h.userLocation(r)
	now := nowInLoc(loc)
	year, month, day := now.Date()
	midnight := time.Date(year, month, day, 0, 0, 0, 0, loc)
	start := midnight.AddDate(0, 0, -7)
	end := midnight.AddDate(0, 0, 1)

	if startStr != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", startStr, loc); err == nil {
			start = parsed
		}
	}
	if endStr != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", endStr, loc); err == nil {
			end = parsed
		}
	}

	breakdown, err := h.service.GetCategoryBreakdown(r.Context(), *user.HouseholdID, start, end, loc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"breakdown": breakdown})
}

func (h *StatsHandler) Recap(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	recap, err := h.service.GetWeeklyRecap(r.Context(), *user.HouseholdID, h.userLocation(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"recap": recap})
}

func (h *StatsHandler) Overview(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	overview, err := h.service.GetWeeklyOverview(r.Context(), *user.HouseholdID, user.ID, h.userLocation(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"overview": overview})
}

func (h *StatsHandler) BusyHours(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	loc := h.userLocation(r)
	now := nowInLoc(loc)
	year, month, day := now.Date()
	midnight := time.Date(year, month, day, 0, 0, 0, 0, loc)
	start := midnight.AddDate(0, 0, -30)
	end := midnight.AddDate(0, 0, 1)

	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	if startStr != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", startStr, loc); err == nil {
			start = parsed
		}
	}
	if endStr != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", endStr, loc); err == nil {
			end = parsed
		}
	}

	hours, err := h.service.GetBusyHours(r.Context(), *user.HouseholdID, start, end, loc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"busyHours": hours})
}

func (h *StatsHandler) ChoreStats(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	choreStats, err := h.service.GetChoreStats(r.Context(), *user.HouseholdID, h.userLocation(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"choreStats": choreStats})
}

func (h *StatsHandler) ChoreStatsByID(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "chore id required")
		return
	}
	choreID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chore id")
		return
	}

	allStats, err := h.service.GetChoreStats(r.Context(), *user.HouseholdID, h.userLocation(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, cs := range allStats {
		if cs.ChoreID == choreID {
			writeJSON(w, http.StatusOK, map[string]any{"choreStats": cs})
			return
		}
	}

	writeError(w, http.StatusNotFound, "chore not found")
}

func (h *StatsHandler) TopChores(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	var userID int64
	if uidStr := r.URL.Query().Get("userId"); uidStr != "" {
		uid, err := strconv.ParseInt(uidStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid userId")
			return
		}
		userID = uid
	}

	entries, err := h.service.GetTopChores(r.Context(), *user.HouseholdID, userID, 5, h.userLocation(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"topChores": entries})
}

func (h *StatsHandler) ChoreTimeSeries(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "chore id required")
		return
	}
	choreID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chore id")
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "daily"
	}

	ts, err := h.service.GetChoreTimeSeries(r.Context(), *user.HouseholdID, choreID, period, h.userLocation(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"timeSeries": ts})
}

func nowInLoc(loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	return time.Now().In(loc)
}
