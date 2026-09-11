package handlers

import (
	"context"
	"crypto/subtle"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

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
	chores      chore.Store
}

func (h *PushHandler) WithChores(chores chore.Store) *PushHandler { h.chores = chores; return h }

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
	user, ok := middleware.CurrentUser(r.Context())
	if !ok || user.SessionHash == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		BindingID    string `json:"bindingId"`
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

	log.Printf("push: subscribe user %d endpoint_host=%s", user.ID, endpointHost(req.Subscription.Endpoint))

	// The endpoint is a bearer-style capability URL the server will POST to
	// on every reminder. Without validation it is a blind SSRF primitive
	// against internal addresses; only allowlisted https push-service hosts
	// may be subscribed.
	if !push.EndpointAllowed(req.Subscription.Endpoint) {
		log.Printf("push: subscribe rejected for user %d endpoint_host=%s", user.ID, endpointHost(req.Subscription.Endpoint))
		writeError(w, http.StatusBadRequest, "endpoint must be a valid https push service URL")
		return
	}
	if req.Subscription.Keys.P256DH == "" || req.Subscription.Keys.Auth == "" {
		writeError(w, http.StatusBadRequest, "subscription keys are required")
		return
	}
	if !pushBindingPattern.MatchString(req.BindingID) {
		writeError(w, http.StatusBadRequest, "a browser identity is required")
		return
	}

	sub := push.Subscription{
		Endpoint:    req.Subscription.Endpoint,
		P256DH:      req.Subscription.Keys.P256DH,
		Auth:        req.Subscription.Keys.Auth,
		SessionHash: user.SessionHash,
		BindingID:   req.BindingID,
	}
	if err := h.store.SaveSubscription(r.Context(), user.ID, sub); err != nil {
		writeServerError(w, "failed to subscribe to push notifications", err)
		return
	}
	log.Printf("push: subscribed user %d", user.ID)
	h.logAudit(r.Context(), "push.subscribed", nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "subscribed"})
}

var pushBindingPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{32,128}$`)

// Identity lets a worker reject a queued notification after session revocation
// or subscription transfer. It returns no profile or subscription capabilities.
func (h *PushHandler) Identity(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if raw := r.Header.Get("X-Nabu-Chore-ID"); raw != "" && raw != "0" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || h.chores == nil {
			writeError(w, http.StatusForbidden, "notification is unavailable")
			return
		}
		c, err := h.chores.GetChore(r.Context(), id)
		if err != nil || user.HouseholdID == nil || c.HouseholdID != *user.HouseholdID ||
			(c.Visibility == chore.VisibilityAdmins && user.Role != "owner" && user.Role != "admin") {
			writeError(w, http.StatusForbidden, "notification is unavailable")
			return
		}
	}
	subs, err := h.store.GetSubscriptions(r.Context(), user.ID)
	if err != nil {
		writeServerError(w, "could not check browser identity", err)
		return
	}
	binding := r.Header.Get("X-Nabu-Push-Binding")
	for _, sub := range subs {
		if subtle.ConstantTimeCompare([]byte(sub.BindingID), []byte(binding)) == 1 && subtle.ConstantTimeCompare([]byte(sub.SessionHash), []byte(user.SessionHash)) == 1 && binding != "" {
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	writeError(w, http.StatusForbidden, "browser identity changed")
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

	if err := h.store.DeleteSubscription(r.Context(), user.ID, req.Endpoint, user.SessionHash); err != nil {
		writeServerError(w, "failed to unsubscribe from push notifications", err)
		return
	}
	h.logAudit(r.Context(), "push.unsubscribed", nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}
