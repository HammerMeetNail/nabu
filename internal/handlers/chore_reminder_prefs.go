package handlers

import (
	"net/http"
	"strconv"

	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/reminder"
)

type ChoreReminderPrefsHandler struct {
	store reminder.Store
}

func NewChoreReminderPrefsHandler(store reminder.Store) *ChoreReminderPrefsHandler {
	return &ChoreReminderPrefsHandler{store: store}
}

func (h *ChoreReminderPrefsHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	prefs, err := h.store.GetChoreReminderPrefs(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if prefs == nil {
		prefs = []reminder.ChoreReminderPref{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"prefs": prefs,
	})
}

func (h *ChoreReminderPrefsHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	choreIDStr := r.PathValue("choreId")
	choreID, err := strconv.ParseInt(choreIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid choreId")
		return
	}

	var req struct {
		Enabled     *bool `json:"enabled"`
		LeadMinutes *int  `json:"leadMinutes"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	current, err := h.store.GetChoreReminderPref(r.Context(), user.ID, choreID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if req.Enabled != nil {
		current.Enabled = *req.Enabled
	}
	if req.LeadMinutes != nil {
		current.LeadMinutes = *req.LeadMinutes
	}
	current.UserID = user.ID
	current.ChoreID = choreID

	if err := h.store.UpdateChoreReminderPref(r.Context(), current); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"pref": current,
	})
}
