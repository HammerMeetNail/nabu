package handlers

import (
	"context"
	"log"
	"net/http"
	"net/url"

	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/push"
)

// endpointHost returns just the scheme+host of a push endpoint for logging.
// The full endpoint URL is a bearer-style capability (its path/query authorize
// delivery to a specific browser), so only the host is safe to log.
func endpointHost(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return "unknown"
	}
	return u.Scheme + "://" + u.Host
}

type PushHandler struct {
	store       push.Store
	auditLogger audit.Logger
}

func NewPushHandler(store push.Store) *PushHandler {
	return &PushHandler{store: store, auditLogger: audit.NopLogger{}}
}

// SetAuditLogger attaches a sink for push subscription events. A nil logger is
// a no-op (the handler keeps its default NopLogger).
func (h *PushHandler) SetAuditLogger(logger audit.Logger) {
	if logger != nil {
		h.auditLogger = logger
	}
}

func (h *PushHandler) logAudit(ctx context.Context, event string, attrs map[string]string) {
	audit.Emit(ctx, h.auditLogger, event, attrs)
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
		log.Printf("push: subscribe parse error for user %d: %v", user.ID, err)
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	log.Printf("push: subscribe user %d endpoint_host=%s", user.ID, endpointHost(req.Subscription.Endpoint))

	sub := push.Subscription{
		Endpoint: req.Subscription.Endpoint,
		P256DH:   req.Subscription.Keys.P256DH,
		Auth:     req.Subscription.Keys.Auth,
	}
	if err := h.store.SaveSubscription(r.Context(), user.ID, sub); err != nil {
		log.Printf("push: subscribe save error for user %d: %v", user.ID, err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("push: subscribed user %d", user.ID)
	h.logAudit(r.Context(), "push.subscribed", nil)
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
	h.logAudit(r.Context(), "push.unsubscribed", nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}
