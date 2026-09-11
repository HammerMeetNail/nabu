package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/middleware"
)

func (h *LogHandler) RecentAmounts(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}
	choreID, err := strconv.ParseInt(r.URL.Query().Get("choreId"), 10, 64)
	if err != nil || choreID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid chore")
		return
	}
	amounts, err := h.service.RecentAmounts(r.Context(), user.ID, *user.HouseholdID, choreID)
	if errors.Is(err, log.ErrNotFound) {
		writeError(w, http.StatusNotFound, "chore not found")
		return
	}
	if err != nil {
		writeServerError(w, "failed to load recent amounts", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"amounts": amounts})
}
