package handlers

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/middleware"
)

var hexColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func validateChoreInput(name, icon, color, category string, indicatorLabels, indicatorDefaults []string) (int, string) {
	if utf8.RuneCountInString(name) == 0 {
		return http.StatusBadRequest, "name must not be empty"
	}
	if utf8.RuneCountInString(name) > 60 {
		return http.StatusBadRequest, "name must be 60 characters or fewer"
	}
	if utf8.RuneCountInString(icon) > 8 {
		return http.StatusBadRequest, "icon must be 8 characters or fewer"
	}
	if color != "" && !hexColorRe.MatchString(color) {
		return http.StatusBadRequest, "color must be a valid hex color (#RRGGBB)"
	}
	if utf8.RuneCountInString(category) > 30 {
		return http.StatusBadRequest, "category must be 30 characters or fewer"
	}
	if strings.ContainsAny(category, "\x00\n\r\t") {
		return http.StatusBadRequest, "category contains invalid characters"
	}
	if len(indicatorLabels) > 8 {
		return http.StatusBadRequest, "too many indicator labels"
	}
	for _, label := range indicatorLabels {
		if utf8.RuneCountInString(label) == 0 || utf8.RuneCountInString(label) > 30 {
			return http.StatusBadRequest, "indicator labels must be 1-30 characters"
		}
		if strings.ContainsAny(label, "\x00\n\r\t") {
			return http.StatusBadRequest, "indicator label contains invalid characters"
		}
	}
	labelSet := map[string]struct{}{}
	for _, label := range indicatorLabels {
		labelSet[label] = struct{}{}
	}
	for _, label := range indicatorDefaults {
		if _, ok := labelSet[label]; !ok {
			return http.StatusBadRequest, "indicator defaults must be a subset of indicator labels"
		}
	}
	return 0, ""
}

type ChoreHandler struct {
	service *chore.Service
}

func NewChoreHandler(service *chore.Service) *ChoreHandler {
	return &ChoreHandler{service: service}
}

func (h *ChoreHandler) List(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	chores, err := h.service.ListChores(r.Context(), *user.HouseholdID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if chores == nil {
		chores = []chore.Chore{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"chores": chores})
}

func (h *ChoreHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	var req struct {
		Name              string   `json:"name"`
		Icon              string   `json:"icon"`
		Color             string   `json:"color"`
		Category          string   `json:"category"`
		IndicatorLabels   []string `json:"indicatorLabels"`
		IndicatorDefaults []string `json:"indicatorDefaults"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if code, msg := validateChoreInput(req.Name, req.Icon, req.Color, req.Category, req.IndicatorLabels, req.IndicatorDefaults); code != 0 {
		writeError(w, code, msg)
		return
	}

	created, err := h.service.CreateChore(r.Context(), *user.HouseholdID, user.ID, req.Name, req.Icon, req.Color, req.Category, req.IndicatorLabels, req.IndicatorDefaults)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"chore": created})
}

func (h *ChoreHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok || user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chore id")
		return
	}

	c, err := h.service.GetChore(r.Context(), id)
	if err != nil || c.HouseholdID != *user.HouseholdID {
		writeError(w, http.StatusNotFound, "chore not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chore": c})
}

func (h *ChoreHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok || user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chore id")
		return
	}

	var req struct {
		Name              string   `json:"name"`
		Icon              string   `json:"icon"`
		Color             string   `json:"color"`
		Category          string   `json:"category"`
		IndicatorLabels   []string `json:"indicatorLabels"`
		IndicatorDefaults []string `json:"indicatorDefaults"`
		FollowUpEnabled   *bool    `json:"followUpEnabled"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if code, msg := validateChoreInput(req.Name, req.Icon, req.Color, req.Category, req.IndicatorLabels, req.IndicatorDefaults); code != 0 {
		writeError(w, code, msg)
		return
	}

	if err := h.service.UpdateChore(r.Context(), id, *user.HouseholdID, req.Name, req.Icon, req.Color, req.Category, req.IndicatorLabels, req.IndicatorDefaults, req.FollowUpEnabled); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *ChoreHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok || user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chore id")
		return
	}

	if err := h.service.DeleteChore(r.Context(), id, *user.HouseholdID); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *ChoreHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	var req struct {
		ChoreIDs []int64 `json:"choreIds"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.ReorderChores(r.Context(), *user.HouseholdID, req.ChoreIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "reordered"})
}

func (h *ChoreHandler) RestoreDefault(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok || user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chore id")
		return
	}

	if err := h.service.RestoreDefaultChore(r.Context(), id, *user.HouseholdID); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}

func (h *ChoreHandler) GetDefaults(w http.ResponseWriter, r *http.Request) {
	defaults := h.service.GetSystemDefaults()
	writeJSON(w, http.StatusOK, map[string]any{"defaults": defaults})
}

func (h *ChoreHandler) SeedDefaults(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	if err := h.service.SeedDefaultChores(r.Context(), *user.HouseholdID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "seeded"})
}
