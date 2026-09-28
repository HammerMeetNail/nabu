package handlers

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/schedule"
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

// validateQuietHours checks a household-level quiet window on a chore. Either
// both bounds are empty (no window) or both are well-formed "HH:MM" values.
func validateQuietHours(start, end string) (int, string) {
	if start == "" && end == "" {
		return 0, ""
	}
	if !chore.QuietHoursComplete(start, end) {
		return http.StatusBadRequest, "quiet hours require both a start and end time"
	}
	return 0, ""
}

// validateQuietHoursPtr is the PATCH variant: nil means unchanged, an explicit
// value must be empty (clear) or a well-formed "HH:MM".
func validateQuietHoursPtr(start, end *string) (int, string) {
	for _, v := range []*string{start, end} {
		if v != nil && *v != "" && !chore.ValidQuietTime(*v) {
			return http.StatusBadRequest, "quiet hours must be an HH:MM time"
		}
	}
	return 0, ""
}

// validateMetric checks a metric type against the closed allowlist and caps the
// unit label. An empty metricType is allowed (treated as "none" downstream).
func validateMetric(metricType, metricUnit string) (int, string) {
	if metricType != "" && !chore.ValidMetricType(metricType) {
		return http.StatusBadRequest, "invalid metric type"
	}
	if utf8.RuneCountInString(metricUnit) > 12 {
		return http.StatusBadRequest, "metric unit must be 12 characters or fewer"
	}
	if strings.ContainsAny(metricUnit, "\x00\n\r\t") {
		return http.StatusBadRequest, "metric unit contains invalid characters"
	}
	return 0, ""
}

// validateSubjects checks the per-chore subject tags (Phase 5.5).
func validateSubjects(subjects []string) (int, string) {
	if len(subjects) > 8 {
		return http.StatusBadRequest, "too many subjects"
	}
	for _, s := range subjects {
		if utf8.RuneCountInString(s) == 0 || utf8.RuneCountInString(s) > 30 {
			return http.StatusBadRequest, "subjects must be 1-30 characters"
		}
		if strings.ContainsAny(s, "\x00\n\r\t") {
			return http.StatusBadRequest, "subject contains invalid characters"
		}
	}
	return 0, ""
}

type ChoreHandler struct {
	service        *chore.Service
	scheduleStore  scheduleStore
	householdStore householdStore
}

type scheduleStore interface {
	ListByHousehold(ctx context.Context, householdID int64) ([]schedule.ChoreSchedule, error)
	Update(ctx context.Context, s schedule.ChoreSchedule) (schedule.ChoreSchedule, error)
}

type householdStore interface {
	GetMembershipForHousehold(ctx context.Context, userID, householdID int64) (string, error)
}

func NewChoreHandler(service *chore.Service) *ChoreHandler {
	return &ChoreHandler{service: service}
}

func (h *ChoreHandler) WithScheduleStore(s scheduleStore) *ChoreHandler {
	h.scheduleStore = s
	return h
}

func (h *ChoreHandler) WithHouseholdStore(hs householdStore) *ChoreHandler {
	h.householdStore = hs
	return h
}

func (h *ChoreHandler) List(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	chores, err := h.service.ListVisible(r.Context(), user.ID, *user.HouseholdID)
	if err != nil {
		writeServerError(w, "failed to load chores", err)
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
		FollowUpEnabled   *bool    `json:"followUpEnabled"`
		MetricType        string   `json:"metricType"`
		MetricUnit        string   `json:"metricUnit"`
		Subjects          []string `json:"subjects"`
		Visibility        *string  `json:"visibility"`
		QuietHoursStart   string   `json:"quietHoursStart"`
		QuietHoursEnd     string   `json:"quietHoursEnd"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if code, msg := validateQuietHours(req.QuietHoursStart, req.QuietHoursEnd); code != 0 {
		writeError(w, code, msg)
		return
	}
	if code, msg := validateChoreInput(req.Name, req.Icon, req.Color, req.Category, req.IndicatorLabels, req.IndicatorDefaults); code != 0 {
		writeError(w, code, msg)
		return
	}
	if code, msg := validateMetric(req.MetricType, req.MetricUnit); code != 0 {
		writeError(w, code, msg)
		return
	}
	if code, msg := validateSubjects(req.Subjects); code != 0 {
		writeError(w, code, msg)
		return
	}
	vis := chore.VisibilityHousehold
	if req.Visibility != nil {
		if !chore.ValidVisibility(*req.Visibility) {
			writeError(w, http.StatusBadRequest, "invalid visibility")
			return
		}
		vis = *req.Visibility
	}

	created, err := h.service.CreateChoreWithVisibility(r.Context(), *user.HouseholdID, user.ID, req.Name, req.Icon, req.Color, req.Category, req.IndicatorLabels, req.IndicatorDefaults, req.FollowUpEnabled, req.MetricType, req.MetricUnit, req.Subjects, vis, req.QuietHoursStart, req.QuietHoursEnd)
	if err != nil {
		if errors.Is(err, chore.ErrNotAdmin) {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		if strings.Contains(err.Error(), "invalid visibility") || strings.Contains(err.Error(), "invalid quiet hours") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
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

	c, err := h.service.GetVisible(r.Context(), user.ID, *user.HouseholdID, id)
	if err != nil {
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
		Name              string    `json:"name"`
		Icon              string    `json:"icon"`
		Color             string    `json:"color"`
		Category          string    `json:"category"`
		IndicatorLabels   []string  `json:"indicatorLabels"`
		IndicatorDefaults []string  `json:"indicatorDefaults"`
		FollowUpEnabled   *bool     `json:"followUpEnabled"`
		MetricType        *string   `json:"metricType"`
		MetricUnit        *string   `json:"metricUnit"`
		Subjects          *[]string `json:"subjects"`
		Visibility        *string   `json:"visibility"`
		QuietHoursStart   *string   `json:"quietHoursStart"`
		QuietHoursEnd     *string   `json:"quietHoursEnd"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if code, msg := validateQuietHoursPtr(req.QuietHoursStart, req.QuietHoursEnd); code != 0 {
		writeError(w, code, msg)
		return
	}

	// For PATCH, name may be empty (no change); validate only if provided.
	if req.Name != "" || req.Icon != "" || req.Color != "" || req.Category != "" || req.IndicatorLabels != nil || req.IndicatorDefaults != nil {
		nameForVal := req.Name
		if nameForVal == "" {
			nameForVal = "x"
		}
		if code, msg := validateChoreInput(nameForVal, req.Icon, req.Color, req.Category, req.IndicatorLabels, req.IndicatorDefaults); code != 0 {
			writeError(w, code, msg)
			return
		}
	}
	mt, mu := "", ""
	if req.MetricType != nil {
		mt = *req.MetricType
	}
	if req.MetricUnit != nil {
		mu = *req.MetricUnit
	}
	if code, msg := validateMetric(mt, mu); code != 0 {
		writeError(w, code, msg)
		return
	}
	if req.Subjects != nil {
		if code, msg := validateSubjects(*req.Subjects); code != 0 {
			writeError(w, code, msg)
			return
		}
	}
	if req.Visibility != nil && !chore.ValidVisibility(*req.Visibility) {
		writeError(w, http.StatusBadRequest, "invalid visibility")
		return
	}

	// Capture old visibility for schedule-assignment cleanup on household→admins transition.
	var oldVis string
	if req.Visibility != nil {
		if existing, err := h.service.GetChore(r.Context(), id); err == nil {
			oldVis = existing.Visibility
			if oldVis == "" {
				oldVis = chore.VisibilityHousehold
			}
		}
	}

	updated, err := h.service.UpdateChoreWithVisibility(r.Context(), id, *user.HouseholdID, req.Name, req.Icon, req.Color, req.Category, req.IndicatorLabels, req.IndicatorDefaults, req.FollowUpEnabled, req.MetricType, req.MetricUnit, req.Subjects, req.Visibility, user.ID, req.QuietHoursStart, req.QuietHoursEnd)
	if err != nil {
		if errors.Is(err, chore.ErrNotAdmin) {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		if err.Error() == "chore not found" || errors.Is(err, chore.ErrNotFound) {
			writeError(w, http.StatusNotFound, "chore not found")
			return
		}
		if strings.Contains(err.Error(), "invalid visibility") || strings.Contains(err.Error(), "invalid metric type") || strings.Contains(err.Error(), "invalid quiet hours") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	cleared := 0
	if req.Visibility != nil && oldVis == chore.VisibilityHousehold && updated.Visibility == chore.VisibilityAdmins && h.scheduleStore != nil && h.householdStore != nil {
		if schedules, err := h.scheduleStore.ListByHousehold(r.Context(), *user.HouseholdID); err == nil {
			for _, s := range schedules {
				if s.ChoreID != id || s.AssignedUserID == nil {
					continue
				}
				role, err := h.householdStore.GetMembershipForHousehold(r.Context(), *s.AssignedUserID, *user.HouseholdID)
				if err != nil {
					continue
				}
				if role != household.RoleOwner && role != household.RoleAdmin {
					// Clear member assignment
					s.AssignedUserID = nil
					if _, uerr := h.scheduleStore.Update(r.Context(), s); uerr == nil {
						cleared++
					}
				}
			}
		}
	}

	resp := map[string]any{"chore": updated, "status": "updated"}
	if cleared > 0 {
		resp["clearedAssignments"] = cleared
	}
	writeJSON(w, http.StatusOK, resp)
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

	if err := h.service.DeleteChoreForUser(r.Context(), id, *user.HouseholdID, user.ID); err != nil {
		if err.Error() == "chore not found" || errors.Is(err, chore.ErrNotFound) {
			writeError(w, http.StatusNotFound, "chore not found")
			return
		}
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

	// Verify each chore is visible to the caller; a member must not be able to reorder a private task.
	for _, cid := range req.ChoreIDs {
		if _, err := h.service.GetVisible(r.Context(), user.ID, *user.HouseholdID, cid); err != nil {
			writeError(w, http.StatusNotFound, "chore not found")
			return
		}
	}

	if err := h.service.ReorderChores(r.Context(), *user.HouseholdID, req.ChoreIDs); err != nil {
		writeServerError(w, "failed to reorder chores", err)
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

	if err := h.service.RestoreDefaultChoreForUser(r.Context(), id, *user.HouseholdID, user.ID); err != nil {
		if err.Error() == "chore not found" || errors.Is(err, chore.ErrNotFound) {
			writeError(w, http.StatusNotFound, "chore not found")
			return
		}
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
		writeServerError(w, "failed to seed default chores", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "seeded"})
}
