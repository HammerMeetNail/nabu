package handlers

import (
	"net/http"
	"strings"

	"github.com/HammerMeetNail/nabu/internal/apns"
	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/middleware"
)

// APNsHandler serves native device-token registration for the iOS app.
type APNsHandler struct {
	store       apns.Store
	auditLogger audit.Logger
}

func NewAPNsHandler(store apns.Store) *APNsHandler {
	return &APNsHandler{store: store, auditLogger: audit.NopLogger{}}
}

func (h *APNsHandler) SetAuditLogger(logger audit.Logger) {
	if logger != nil {
		h.auditLogger = logger
	}
}

// Register handles POST /api/mobile/apns/register.
func (h *APNsHandler) Register(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		Token       string `json:"token"`
		Environment string `json:"environment"`
		BundleID    string `json:"bundleId"`
		DeviceName  string `json:"deviceName"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" || len(req.Token) > 200 || !isHexToken(req.Token) {
		writeError(w, http.StatusBadRequest, "invalid device token")
		return
	}
	if req.Environment != apns.EnvironmentSandbox && req.Environment != apns.EnvironmentProduction {
		writeError(w, http.StatusBadRequest, "environment must be sandbox or production")
		return
	}
	if req.BundleID == "" || len(req.BundleID) > 200 {
		writeError(w, http.StatusBadRequest, "invalid bundle id")
		return
	}
	if len(req.DeviceName) > 100 {
		req.DeviceName = req.DeviceName[:100]
	}

	err := h.store.RegisterDevice(r.Context(), apns.Device{
		UserID:      user.ID,
		SessionHash: user.SessionHash,
		Token:       req.Token,
		Environment: req.Environment,
		BundleID:    req.BundleID,
		DeviceName:  req.DeviceName,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register device")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "registered"})
}

// Unregister handles POST /api/mobile/apns/unregister. Only the owning
// user's registration is removed; another user's token is untouched.
func (h *APNsHandler) Unregister(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" || len(req.Token) > 200 {
		writeError(w, http.StatusBadRequest, "invalid device token")
		return
	}

	if err := h.store.UnregisterDevice(r.Context(), user.ID, req.Token, user.SessionHash); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to unregister device")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unregistered"})
}

// isHexToken reports whether s looks like a hex-encoded APNs device token.
func isHexToken(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
