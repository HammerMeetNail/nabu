package handlers

import (
	"net/http"

	"github.com/dave/choresy/internal/middleware"
	"github.com/dave/choresy/internal/userprefs"
)

// PreferencesHandler handles GET /api/preferences and PATCH /api/preferences.
type PreferencesHandler struct {
	service *userprefs.Service
}

// NewPreferencesHandler constructs a PreferencesHandler.
func NewPreferencesHandler(service *userprefs.Service) *PreferencesHandler {
	return &PreferencesHandler{service: service}
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"preferences": prefs})
}

// Update patches the current user's preferences.  Only fields present in the
// request body are updated; choreOrder and hiddenHomeChoreIds are supported.
func (h *PreferencesHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		ChoreOrder         *[]int64 `json:"choreOrder"`
		HiddenHomeChoreIDs *[]int64 `json:"hiddenHomeChoreIds"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.ChoreOrder != nil {
		if err := h.service.UpdateChoreOrder(r.Context(), user.ID, *req.ChoreOrder); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if req.HiddenHomeChoreIDs != nil {
		if err := h.service.UpdateHiddenHomeChores(r.Context(), user.ID, *req.HiddenHomeChoreIDs); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	prefs, err := h.service.GetPreferences(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"preferences": prefs})
}
