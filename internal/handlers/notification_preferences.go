package handlers

import (
	"net/http"

	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/notification"
)

type NotificationPreferencesHandler struct {
	service *notification.Service
}

func NewNotificationPreferencesHandler(service *notification.Service) *NotificationPreferencesHandler {
	return &NotificationPreferencesHandler{service: service}
}

func (h *NotificationPreferencesHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	prefs, err := h.service.GetNotificationPreferences(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"preferences":    prefs,
		"availableTypes": notification.AvailableNotificationTypes(),
	})
}

func (h *NotificationPreferencesHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		PushEnabled                *bool     `json:"pushEnabled"`
		EmailEnabled               *bool     `json:"emailEnabled"`
		EnabledPushTypes           *[]string `json:"enabledPushTypes"`
		DefaultReminderLeadMinutes *int      `json:"defaultReminderLeadMinutes"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	current, err := h.service.GetNotificationPreferences(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if req.PushEnabled != nil {
		current.PushEnabled = *req.PushEnabled
	}
	if req.EmailEnabled != nil {
		current.EmailEnabled = *req.EmailEnabled
	}
	if req.EnabledPushTypes != nil {
		current.EnabledPushTypes = *req.EnabledPushTypes
	}
	if req.DefaultReminderLeadMinutes != nil {
		current.DefaultReminderLeadMinutes = *req.DefaultReminderLeadMinutes
	}

	if err := h.service.UpdateNotificationPreferences(r.Context(), current); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	prefs, err := h.service.GetNotificationPreferences(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"preferences":    prefs,
		"availableTypes": notification.AvailableNotificationTypes(),
	})
}
