package handlers

import (
	"net/http"

	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/reminder"
)

// reminderLogLimit caps how many delivery-log entries one request returns.
const reminderLogLimit = 20

// ReminderLogHandler serves the current user's recent schedule-reminder
// delivery log. It is user-scoped: a caller only ever sees rows for their own
// user id (the store query enforces this), which makes it safe to expose and
// useful for verifying that quiet hours actually suppressed a reminder.
type ReminderLogHandler struct {
	store reminder.Store
}

func NewReminderLogHandler(store reminder.Store) *ReminderLogHandler {
	return &ReminderLogHandler{store: store}
}

// List handles GET /api/reminders/log.
func (h *ReminderLogHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	reminders, err := h.store.ListRecentReminders(r.Context(), user.ID, reminderLogLimit)
	if err != nil {
		writeServerError(w, "failed to load reminder log", err)
		return
	}
	if reminders == nil {
		reminders = []reminder.ReminderRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"reminders": reminders})
}
