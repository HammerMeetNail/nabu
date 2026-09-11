package handlers

import (
	"net/http"
	"strconv"
	"strings"
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
	var rangeStart, rangeEnd time.Time

	loc := h.userLocation(r)
	now := nowInLoc(loc)
	ctx := stats.WithViewer(r.Context(), user.ID)

	switch period {
	case "day":
		y, m, d := now.Date()
		rangeStart = time.Date(y, m, d, 0, 0, 0, 0, loc)
		rangeEnd = rangeStart.AddDate(0, 0, 1)
		board, err = h.service.GetDailyLeaderboard(ctx, *user.HouseholdID, loc)
	case "month":
		rangeStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		rangeEnd = rangeStart.AddDate(0, 1, 0)
		board, err = h.service.GetMonthlyLeaderboard(ctx, *user.HouseholdID, now.Year(), now.Month(), loc)
	case "all":
		// No time bound: fetch every log ever recorded for this household.
		rangeStart = time.Time{}
		rangeEnd = time.Time{}
		board, err = h.service.GetAllTimeLeaderboard(ctx, *user.HouseholdID, loc)
	default:
		weekday := int(now.Weekday()) - int(time.Sunday)
		if weekday < 0 {
			weekday += 7
		}
		y, m, d := now.Date()
		rangeStart = time.Date(y, m, d-weekday, 0, 0, 0, 0, loc)
		rangeEnd = rangeStart.AddDate(0, 0, 7)
		board, err = h.service.GetWeeklyLeaderboard(ctx, *user.HouseholdID, loc)
	}

	if err != nil {
		writeServerError(w, "failed to load leaderboard", err)
		return
	}

	resp := map[string]any{
		"leaderboard": board,
		"period":      period,
	}
	if !rangeStart.IsZero() {
		resp["start"] = rangeStart.Format("2006-01-02")
	}
	if !rangeEnd.IsZero() {
		resp["end"] = rangeEnd.Format("2006-01-02")
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *StatsHandler) Streaks(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	ctx := stats.WithViewer(r.Context(), user.ID)
	streaks, err := h.service.GetUserStreaks(ctx, *user.HouseholdID, user.ID, h.userLocation(r))
	if err != nil {
		writeServerError(w, "failed to load streaks", err)
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

	ctx := stats.WithViewer(r.Context(), user.ID)
	cells, err := h.service.GetHeatmap(ctx, *user.HouseholdID, start, end, loc)
	if err != nil {
		writeServerError(w, "failed to load heatmap", err)
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
	periodStr := r.URL.Query().Get("period")

	loc := h.userLocation(r)
	now := nowInLoc(loc)

	var start, end time.Time

	if periodStr != "" {
		y, m, d := now.Date()
		midnight := time.Date(y, m, d, 0, 0, 0, 0, loc)
		switch periodStr {
		case "day":
			start = midnight
			end = midnight.AddDate(0, 0, 1)
		case "week":
			weekday := int(now.Weekday()) - int(time.Sunday)
			if weekday < 0 {
				weekday += 7
			}
			start = time.Date(y, m, d-weekday, 0, 0, 0, 0, loc)
			end = start.AddDate(0, 0, 7)
		case "month":
			start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
			end = start.AddDate(0, 1, 0)
		default:
			writeError(w, http.StatusBadRequest, "invalid period")
			return
		}
	} else if startStr != "" || endStr != "" {
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		start = midnight.AddDate(0, 0, -7)
		end = midnight.AddDate(0, 0, 1)
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
	} else {
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		weekday := int(now.Weekday()) - int(time.Sunday)
		if weekday < 0 {
			weekday += 7
		}
		start = midnight.AddDate(0, 0, -weekday)
		end = start.AddDate(0, 0, 7)
	}

	ctx := stats.WithViewer(r.Context(), user.ID)
	breakdown, err := h.service.GetCategoryBreakdown(ctx, *user.HouseholdID, start, end, loc)
	if err != nil {
		writeServerError(w, "failed to load breakdown", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"breakdown": breakdown,
		"start":     start.Format("2006-01-02"),
		"end":       end.Format("2006-01-02"),
	})
}

func (h *StatsHandler) Recap(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	ctx := stats.WithViewer(r.Context(), user.ID)
	recap, err := h.service.GetWeeklyRecap(ctx, *user.HouseholdID, h.userLocation(r))
	if err != nil {
		writeServerError(w, "failed to load recap", err)
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

	ctx := stats.WithViewer(r.Context(), user.ID)
	overview, err := h.service.GetWeeklyOverview(ctx, *user.HouseholdID, user.ID, h.userLocation(r))
	if err != nil {
		writeServerError(w, "failed to load overview", err)
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

	var choreID, userID *int64
	if cidStr := r.URL.Query().Get("choreId"); cidStr != "" {
		if cid, err := strconv.ParseInt(cidStr, 10, 64); err == nil {
			choreID = &cid
		}
	}
	if uidStr := r.URL.Query().Get("userId"); uidStr != "" {
		if uid, err := strconv.ParseInt(uidStr, 10, 64); err == nil {
			userID = &uid
		}
	}

	ctx := stats.WithViewer(r.Context(), user.ID)
	hours, err := h.service.GetBusyHours(ctx, *user.HouseholdID, start, end, loc, choreID, userID)
	if err != nil {
		if strings.Contains(err.Error(), "chore not found") {
			writeError(w, http.StatusNotFound, "chore not found")
			return
		}
		writeServerError(w, "failed to load busy hours", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"busyHours": hours,
		"start":     start.Format("2006-01-02"),
		"end":       end.Format("2006-01-02"),
	})
}

func (h *StatsHandler) ChoreStats(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	loc := h.userLocation(r)
	now := nowInLoc(loc)

	var customStart, customEnd *time.Time

	periodStr := r.URL.Query().Get("period")
	if periodStr != "" {
		y, m, d := now.Date()
		midnight := time.Date(y, m, d, 0, 0, 0, 0, loc)
		var s, e time.Time
		switch periodStr {
		case "day":
			s = midnight
			e = midnight.AddDate(0, 0, 1)
		case "week":
			weekday := int(now.Weekday()) - int(time.Sunday)
			if weekday < 0 {
				weekday += 7
			}
			s = time.Date(y, m, d-weekday, 0, 0, 0, 0, loc)
			e = s.AddDate(0, 0, 7)
		case "month":
			s = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
			e = s.AddDate(0, 1, 0)
		default:
			writeError(w, http.StatusBadRequest, "invalid period")
			return
		}
		customStart = &s
		customEnd = &e
	} else {
		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")
		if startStr != "" && endStr != "" {
			if parsed, err := time.ParseInLocation("2006-01-02", startStr, loc); err == nil {
				customStart = &parsed
			}
			if parsed, err := time.ParseInLocation("2006-01-02", endStr, loc); err == nil {
				customEnd = &parsed
			}
		}
	}

	ctx := stats.WithViewer(r.Context(), user.ID)
	choreStats, err := h.service.GetChoreStats(ctx, *user.HouseholdID, loc, customStart, customEnd)
	if err != nil {
		writeServerError(w, "failed to load chore stats", err)
		return
	}

	fetchStart := now.AddDate(0, 0, -29)
	fetchEnd := now.AddDate(0, 0, 1)
	if customStart != nil {
		fetchStart = *customStart
	}
	if customEnd != nil {
		fetchEnd = *customEnd
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"choreStats": choreStats,
		"start":      fetchStart.Format("2006-01-02"),
		"end":        fetchEnd.Format("2006-01-02"),
	})
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

	ctx := stats.WithViewer(r.Context(), user.ID)
	allStats, err := h.service.GetChoreStats(ctx, *user.HouseholdID, h.userLocation(r), nil, nil)
	if err != nil {
		writeServerError(w, "failed to load chore stats", err)
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

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}

	ctx := stats.WithViewer(r.Context(), user.ID)
	entries, err := h.service.GetTopChores(ctx, *user.HouseholdID, userID, 5, period, h.userLocation(r))
	if err != nil {
		writeServerError(w, "failed to load top chores", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"topChores": entries,
		"period":    period,
	})
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

	ctx := stats.WithViewer(r.Context(), user.ID)
	ts, err := h.service.GetChoreTimeSeries(ctx, *user.HouseholdID, choreID, period, h.userLocation(r))
	if err != nil {
		if strings.Contains(err.Error(), "chore not found") {
			writeError(w, http.StatusNotFound, "chore not found")
			return
		}
		writeServerError(w, "failed to load chore time series", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"timeSeries": ts})
}

func (h *StatsHandler) ChoreSummary(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	idStr := r.PathValue("id")
	choreID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chore id")
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "week"
	}

	ctx := stats.WithViewer(r.Context(), user.ID)
	summary, err := h.service.GetChoreSummary(ctx, *user.HouseholdID, choreID, period, h.userLocation(r))
	if err != nil {
		if strings.Contains(err.Error(), "chore not found") {
			writeError(w, http.StatusNotFound, "chore not found")
			return
		}
		writeServerError(w, "failed to load chore summary", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": summary})
}

func (h *StatsHandler) FeedingGaps(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	loc := h.userLocation(r)

	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	var start, end time.Time
	if startStr != "" && endStr != "" {
		var err error
		start, err = time.ParseInLocation("2006-01-02", startStr, loc)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid start date")
			return
		}
		end, err = time.ParseInLocation("2006-01-02", endStr, loc)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid end date")
			return
		}
	} else {
		now := nowInLoc(loc)
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -14)
		end = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	}

	var choreID *int64
	if cidStr := r.URL.Query().Get("choreId"); cidStr != "" {
		if cid, err := strconv.ParseInt(cidStr, 10, 64); err == nil {
			choreID = &cid
		}
	}

	ctx := stats.WithViewer(r.Context(), user.ID)
	gaps, err := h.service.GetFeedingGaps(ctx, *user.HouseholdID, choreID, start, end, loc)
	if err != nil {
		if strings.Contains(err.Error(), "chore not found") {
			writeError(w, http.StatusNotFound, "chore not found")
			return
		}
		writeServerError(w, "failed to load feeding gaps", err)
		return
	}
	if gaps == nil {
		gaps = []stats.FeedingGap{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"feedingGaps": gaps})
}

func nowInLoc(loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	return time.Now().In(loc)
}
