package handlers

import (
	"errors"
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

// List returns a bounded newest-first cursor page and the current unread count.
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	page, err := h.service.ListPage(r.Context(), user.ID, r.URL.Query().Get("cursor"))
	if errors.Is(err, notification.ErrInvalidCursor) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeServerError(w, "failed to load notifications", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, page)
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
		writeServerError(w, "failed to mark notification read", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// MarkAllRead marks every notification for the current user as read.
func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if err := h.service.MarkAllRead(r.Context(), user.ID); err != nil {
		writeServerError(w, "failed to mark notifications read", err)
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
		writeServerError(w, "failed to delete notification", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
