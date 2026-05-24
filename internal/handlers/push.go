package handlers

import (
	"net/http"

	"github.com/dave/choresy/internal/middleware"
	"github.com/dave/choresy/internal/push"
)

type PushHandler struct {
	store push.Store
}

func NewPushHandler(store push.Store) *PushHandler {
	return &PushHandler{store: store}
}

// Subscribe saves a Web Push subscription for the current user.
func (h *PushHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())

	var req struct {
		Subscription struct {
			Endpoint string `json:"endpoint"`
			Keys     struct {
				P256DH string `json:"p256dh"`
				Auth   string `json:"auth"`
			} `json:"keys"`
		} `json:"subscription"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sub := push.Subscription{
		Endpoint: req.Subscription.Endpoint,
		P256DH:   req.Subscription.Keys.P256DH,
		Auth:     req.Subscription.Keys.Auth,
	}
	if err := h.store.SaveSubscription(r.Context(), user.ID, sub); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "subscribed"})
}

// Unsubscribe removes a Web Push subscription for the current user.
func (h *PushHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())

	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.store.DeleteSubscription(r.Context(), user.ID, req.Endpoint); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}
