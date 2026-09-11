package handlers

import (
	"errors"
	"net/http"

	"github.com/HammerMeetNail/nabu/internal/account"
	"github.com/HammerMeetNail/nabu/internal/middleware"
)

// AccountHandler serves account-level operations (currently deletion,
// required in-app by App Store guideline 5.1.1(v)).
type AccountHandler struct {
	service     *account.Service
	authHandler *AuthHandler
}

func NewAccountHandler(service *account.Service, authHandler *AuthHandler) *AccountHandler {
	return &AccountHandler{service: service, authHandler: authHandler}
}

// DeleteMe handles DELETE /api/me. The body must carry the typed
// confirmation {"confirm":"DELETE"} so the account cannot be destroyed by a
// single stray request; CSRF and auth are enforced by middleware like every
// other state-changing endpoint.
func (h *AccountHandler) DeleteMe(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Confirm != "DELETE" {
		writeError(w, http.StatusBadRequest, `confirmation required: send {"confirm":"DELETE"}`)
		return
	}

	if err := h.service.DeleteAccount(r.Context(), user.ID); err != nil {
		var transfer account.ErrMustTransferOwnership
		if errors.As(err, &transfer) {
			writeError(w, http.StatusConflict, transfer.Error())
			return
		}
		writeServerError(w, "account deletion failed", err)
		return
	}

	// The user row is gone (sessions cascade with it); clear the cookie so
	// the client ends up cleanly logged out.
	h.authHandler.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "account deleted"})
}
