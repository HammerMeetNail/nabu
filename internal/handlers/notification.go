package handlers

import (
	"net/http"
	"strconv"

	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/notification"
)

type NotificationHandler struct {
	service *notification.Service
}

func NewNotificationHandler(service *notification.Service) *NotificationHandler {
	return &NotificationHandler{service: service}
}

// List returns the 50 most-recent notifications and the current unread count.
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	notifs, unread, err := h.service.List(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if notifs == nil {
		notifs = []notification.Notification{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"notifications": notifs,
		"unreadCount":   unread,
	})
}

// MarkRead marks a single notification as read.
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid notification id")
		return
	}
	if err := h.service.MarkRead(r.Context(), id, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// MarkAllRead marks every notification for the current user as read.
func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if err := h.service.MarkAllRead(r.Context(), user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Delete removes a single notification belonging to the current user.
func (h *NotificationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid notification id")
		return
	}
	if err := h.service.Delete(r.Context(), id, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
