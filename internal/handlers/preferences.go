package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/userprefs"
)

// PreferencesHandler handles GET /api/preferences and PATCH /api/preferences.
type PreferencesHandler struct {
	service        *userprefs.Service
	choreStore     chore.Store // optional; used to ownership-check widget choreIds
	householdStore household.Store
}

// NewPreferencesHandler constructs a PreferencesHandler.
func NewPreferencesHandler(service *userprefs.Service) *PreferencesHandler {
	return &PreferencesHandler{service: service}
}

// WithChoreStore attaches a chore store so widget choreIds can be
// ownership-checked against the caller's household.
func (h *PreferencesHandler) WithChoreStore(cs chore.Store) *PreferencesHandler {
	h.choreStore = cs
	return h
}

func (h *PreferencesHandler) WithHouseholdStore(hs household.Store) *PreferencesHandler {
	h.householdStore = hs
	return h
}

func (h *PreferencesHandler) visibleChoreSet(userID, householdID int64, ctx context.Context) (map[int64]struct{}, error) {
	if householdID == 0 {
		return map[int64]struct{}{}, nil
	}
	if h.choreStore == nil {
		// An explicitly unwired internal handler has no chore-backed preferences.
		return nil, nil
	}
	return chore.NewService(h.choreStore).WithMemberships(h.householdStore).VisibleChoreIDs(ctx, userID, householdID)
}

func (h *PreferencesHandler) filterPreferencesForResponse(prefs userprefs.Preferences, visible map[int64]struct{}) userprefs.Preferences {
	if visible == nil {
		return prefs
	}
	// Filter choreOrder and hiddenHomeChoreIDs
	var filteredOrder []int64
	for _, id := range prefs.ChoreOrder {
		if _, ok := visible[id]; ok {
			filteredOrder = append(filteredOrder, id)
		}
	}
	prefs.ChoreOrder = filteredOrder
	if prefs.ChoreOrder == nil {
		prefs.ChoreOrder = []int64{}
	}
	var filteredHidden []int64
	for _, id := range prefs.HiddenHomeChoreIDs {
		if _, ok := visible[id]; ok {
			filteredHidden = append(filteredHidden, id)
		}
	}
	prefs.HiddenHomeChoreIDs = filteredHidden
	if prefs.HiddenHomeChoreIDs == nil {
		prefs.HiddenHomeChoreIDs = []int64{}
	}
	// Filter stats widgets: omit any widget that references an invisible chore.
	var filteredWidgets []userprefs.StatsWidget
	for _, w := range prefs.StatsWidgets {
		visibleWidget := true
		for _, cid := range w.ChoreIDs {
			if _, ok := visible[cid]; !ok {
				visibleWidget = false
				break
			}
		}
		if visibleWidget {
			filteredWidgets = append(filteredWidgets, w)
		}
	}
	prefs.StatsWidgets = filteredWidgets
	if prefs.StatsWidgets == nil {
		prefs.StatsWidgets = []userprefs.StatsWidget{}
	}
	// Filter stats section keys that reference chore IDs or widget IDs for invisible tasks
	// Section keys like "chore:123" or "widget:abc" where widget's title may leak private name.
	// We already filtered widgets; now filter section order/hidden that reference invisible chores.
	// For simplicity, check "chore:" prefix.
	var filteredOrderSecs []string
	for _, k := range prefs.StatsSectionOrder {
		if strings.HasPrefix(k, "chore:") {
			var cid int64
			if _, err := parseChoreSection(k, &cid); err == nil {
				if _, ok := visible[cid]; !ok {
					continue
				}
			}
		}
		if strings.HasPrefix(k, "widget:") {
			// widget IDs already filtered, but keep only if widget still exists
			found := false
			for _, w := range prefs.StatsWidgets {
				if k == "widget:"+w.ID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		filteredOrderSecs = append(filteredOrderSecs, k)
	}
	prefs.StatsSectionOrder = filteredOrderSecs
	if prefs.StatsSectionOrder == nil {
		prefs.StatsSectionOrder = []string{}
	}
	var filteredHiddenSecs []string
	for _, k := range prefs.StatsSectionHidden {
		if strings.HasPrefix(k, "chore:") {
			var cid int64
			if _, err := parseChoreSection(k, &cid); err == nil {
				if _, ok := visible[cid]; !ok {
					continue
				}
			}
		}
		filteredHiddenSecs = append(filteredHiddenSecs, k)
	}
	prefs.StatsSectionHidden = filteredHiddenSecs
	if prefs.StatsSectionHidden == nil {
		prefs.StatsSectionHidden = []string{}
	}
	return prefs
}

func parseChoreSection(k string, out *int64) (int64, error) {
	parts := strings.SplitN(k, ":", 2)
	if len(parts) != 2 {
		return 0, errors.New("invalid")
	}
	v, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, err
	}
	*out = v
	return v, nil
}

// Get returns the current user's preferences.
func (h *PreferencesHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	prefs, err := h.service.GetPreferences(r.Context(), user.ID)
	if err != nil {
		writeServerError(w, "failed to load preferences", err)
		return
	}
	if user.HouseholdID != nil {
		visible, err := h.visibleChoreSet(user.ID, *user.HouseholdID, r.Context())
		if err != nil {
			writeServerError(w, "failed to verify chore access", err)
			return
		}
		prefs = h.filterPreferencesForResponse(prefs, visible)
	} else {
		prefs = h.filterPreferencesForResponse(prefs, map[int64]struct{}{})
	}

	writeJSON(w, http.StatusOK, map[string]any{"preferences": prefs})
}

// Update patches the current user's preferences.  Only fields present in the
// request body are updated; choreOrder, hiddenHomeChoreIds, and timezone are
// supported.
func (h *PreferencesHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		ChoreOrder            *[]int64                 `json:"choreOrder"`
		HiddenHomeChoreIDs    *[]int64                 `json:"hiddenHomeChoreIds"`
		Timezone              *string                  `json:"timezone"`
		VolumeUnit            *string                  `json:"volumeUnit"`
		StatsSectionOrder     *[]string                `json:"statsSectionOrder"`
		StatsSectionHidden    *[]string                `json:"statsSectionHidden"`
		StatsWidgets          *[]userprefs.StatsWidget `json:"statsWidgets"`
		HideNotificationBadge *bool                    `json:"hideNotificationBadge"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	visible := map[int64]struct{}{}
	if user.HouseholdID != nil {
		var err error
		visible, err = h.visibleChoreSet(user.ID, *user.HouseholdID, r.Context())
		if err != nil {
			writeServerError(w, "failed to verify chore access", err)
			return
		}
	}

	if req.ChoreOrder != nil {
		if visible != nil {
			for _, cid := range *req.ChoreOrder {
				if _, ok := visible[cid]; !ok {
					writeError(w, http.StatusNotFound, "chore not found")
					return
				}
			}
		}
		if err := h.service.UpdateChoreOrder(r.Context(), user.ID, *req.ChoreOrder); err != nil {
			writeServerError(w, "failed to update preferences", err)
			return
		}
	}

	if req.HiddenHomeChoreIDs != nil {
		if visible != nil {
			for _, cid := range *req.HiddenHomeChoreIDs {
				if _, ok := visible[cid]; !ok {
					writeError(w, http.StatusNotFound, "chore not found")
					return
				}
			}
		}
		if err := h.service.UpdateHiddenHomeChores(r.Context(), user.ID, *req.HiddenHomeChoreIDs); err != nil {
			writeServerError(w, "failed to update preferences", err)
			return
		}
	}

	if req.Timezone != nil {
		if err := h.service.UpdateTimezone(r.Context(), user.ID, *req.Timezone); err != nil {
			if errors.Is(err, userprefs.ErrInvalidInput) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeServerError(w, "failed to update preferences", err)
			return
		}
	}

	if req.VolumeUnit != nil {
		if err := h.service.UpdateVolumeUnit(r.Context(), user.ID, *req.VolumeUnit); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	if req.StatsSectionOrder != nil {
		if err := h.service.UpdateStatsSectionOrder(r.Context(), user.ID, *req.StatsSectionOrder); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	if req.StatsSectionHidden != nil {
		if err := h.service.UpdateStatsSectionHidden(r.Context(), user.ID, *req.StatsSectionHidden); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	if req.HideNotificationBadge != nil {
		if err := h.service.UpdateHideNotificationBadge(r.Context(), user.ID, *req.HideNotificationBadge); err != nil {
			writeServerError(w, "failed to update preferences", err)
			return
		}
	}

	if req.StatsWidgets != nil {
		// Ownership + visibility check: every referenced chore must be visible to caller.
		for _, wdg := range *req.StatsWidgets {
			for _, cid := range wdg.ChoreIDs {
				if visible != nil {
					if _, ok := visible[cid]; !ok {
						writeError(w, http.StatusNotFound, "chore not found")
						return
					}
				}
			}
		}
		if _, err := h.service.UpdateStatsWidgets(r.Context(), user.ID, *req.StatsWidgets); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	prefs, err := h.service.GetPreferences(r.Context(), user.ID)
	if err != nil {
		writeServerError(w, "failed to load preferences", err)
		return
	}
	prefs = h.filterPreferencesForResponse(prefs, visible)

	writeJSON(w, http.StatusOK, map[string]any{"preferences": prefs})
}
