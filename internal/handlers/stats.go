package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/dave/choresy/internal/middleware"
	"github.com/dave/choresy/internal/stats"
)

type StatsHandler struct {
	service *stats.Service
}

func NewStatsHandler(service *stats.Service) *StatsHandler {
	return &StatsHandler{service: service}
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
		now := time.Now().UTC()
		board, err = h.service.GetMonthlyLeaderboard(r.Context(), *user.HouseholdID, now.Year(), now.Month())
	default:
		board, err = h.service.GetWeeklyLeaderboard(r.Context(), *user.HouseholdID)
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

	streaks, err := h.service.GetUserStreaks(r.Context(), *user.HouseholdID, user.ID)
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

	now := time.Now().UTC()
	start := now.AddDate(0, -3, 0)
	end := now

	if startStr != "" {
		if parsed, err := time.Parse("2006-01-02", startStr); err == nil {
			start = parsed
		}
	}
	if endStr != "" {
		if parsed, err := time.Parse("2006-01-02", endStr); err == nil {
			end = parsed
		}
	}

	cells, err := h.service.GetHeatmap(r.Context(), *user.HouseholdID, start, end)
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

	now := time.Now().UTC()
	start := now.AddDate(0, 0, -7)
	end := now

	if startStr != "" {
		if parsed, err := time.Parse("2006-01-02", startStr); err == nil {
			start = parsed
		}
	}
	if endStr != "" {
		if parsed, err := time.Parse("2006-01-02", endStr); err == nil {
			end = parsed
		}
	}

	breakdown, err := h.service.GetCategoryBreakdown(r.Context(), *user.HouseholdID, start, end)
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

	recap, err := h.service.GetWeeklyRecap(r.Context(), *user.HouseholdID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"recap": recap})
}

func parseIntQuery(r *http.Request, key string) int {
	v, _ := strconv.Atoi(r.URL.Query().Get(key))
	return v
}
