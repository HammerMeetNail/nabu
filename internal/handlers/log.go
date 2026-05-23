package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/dave/choresy/internal/log"
	"github.com/dave/choresy/internal/middleware"
)

type LogHandler struct {
	service *log.Service
}

func NewLogHandler(service *log.Service) *LogHandler {
	return &LogHandler{service: service}
}

func (h *LogHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	var req struct {
		ChoreID     int64    `json:"choreId"`
		Note        string   `json:"note"`
		Indicators  []string `json:"indicators"`
		Date        string   `json:"date"`        // optional ISO date "YYYY-MM-DD"; defaults to today
		Hour        *int     `json:"hour"`        // optional calendar slot hour (0-23)
		CompletedAt string   `json:"completedAt"` // optional RFC3339 timestamp for backdating
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
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

	entry, err := h.service.LogChore(r.Context(), *user.HouseholdID, user.ID, req.ChoreID, req.Note, req.Indicators, logDate, req.Hour, logCompletedAt)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"log": entry})
}

func (h *LogHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	_ = user

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid log id")
		return
	}

	var req struct {
		Note       string   `json:"note"`
		Indicators []string `json:"indicators"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.UpdateLog(r.Context(), id, req.Note, req.Indicators); err != nil {
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

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid log id")
		return
	}

	if err := h.service.UndoLog(r.Context(), user.ID, id); err != nil {
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
