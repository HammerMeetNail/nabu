package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/notification"
)

type HouseholdHandler struct {
	service        *household.Service
	notifService   *notification.Service
	householdStore household.Store
	background     context.Context
	dispatch       func(func())
}

func NewHouseholdHandler(service *household.Service) *HouseholdHandler {
	return &HouseholdHandler{service: service, background: context.Background(), dispatch: func(f func()) { f() }}
}

func (h *HouseholdHandler) SetBackground(ctx context.Context, dispatch func(func())) {
	h.background, h.dispatch = ctx, dispatch
}

func (h *HouseholdHandler) WithNotification(notifService *notification.Service, householdStore household.Store) {
	h.notifService = notifService
	h.householdStore = householdStore
}

func (h *HouseholdHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	hh, members, err := h.service.GetHousehold(r.Context(), user.ID)
	if err != nil {
		writeHouseholdError(w, r, err)
		return
	}
	historicalMembers, err := h.service.GetHistoricalMembers(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load household")
		return
	}

	invites, inviteErr := h.service.GetInvites(r.Context(), user.ID)
	if inviteErr != nil && !errors.Is(inviteErr, household.ErrNotAuthorized) && !errors.Is(inviteErr, household.ErrNotFound) {
		writeHouseholdError(w, r, inviteErr)
		return
	}
	if invites == nil {
		invites = []household.Invite{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"household":         hh,
		"members":           members,
		"historicalMembers": historicalMembers,
		"invites":           invites,
	})
}

func (h *HouseholdHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		Name     string `json:"name"`
		Initials string `json:"initials"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	hh, err := h.service.CreateHousehold(r.Context(), req.Name, req.Initials, user.ID)
	if err != nil {
		// Validation failures are the caller's fault: 400 with the clear
		// message. Everything else gets a static 409 so store errors (e.g.
		// constraint/connection failures) never leak pgx internals.
		if errors.Is(err, household.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create household")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"household": hh})
}

func (h *HouseholdHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		Name     string `json:"name"`
		Initials string `json:"initials"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.UpdateHousehold(r.Context(), user.ID, req.Name, req.Initials); err != nil {
		if errors.Is(err, household.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeHouseholdError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *HouseholdHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	invite, err := h.service.CreateInvite(r.Context(), user.ID)
	if err != nil {
		writeHouseholdError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"invite": invite})
}

func (h *HouseholdHandler) ListInvites(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	invites, err := h.service.GetInvites(r.Context(), user.ID)
	if err != nil {
		writeHouseholdError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"invites": invites})
}

func (h *HouseholdHandler) DeleteInvite(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	idStr := extractID(r.URL.Path, "/api/household/invites/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid invite id")
		return
	}

	if err := h.service.DeleteInvite(r.Context(), user.ID, id); err != nil {
		writeHouseholdError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *HouseholdHandler) Join(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		InviteCode string `json:"inviteCode"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	hh, err := h.service.JoinHousehold(r.Context(), user.ID, req.InviteCode)
	if err != nil {
		// Static message: every join failure (unknown/exhausted/expired code,
		// household full, or a store error) reads identically, so the response
		// never leaks invite state or pgx internals.
		writeError(w, http.StatusBadRequest, "invite not found")
		return
	}

	// Fire-and-forget: notify other household members.
	if h.notifService != nil && h.householdStore != nil {
		hhID := hh.ID
		joinerID := user.ID
		joinerName := user.DisplayName
		if joinerName == "" {
			joinerName = user.Email
		}
		householdName := hh.Name
		h.dispatch(func() {
			ctx, cancel := context.WithTimeout(h.background, 20*time.Second)
			defer cancel()
			members, err := h.householdStore.GetMembers(ctx, hhID)
			if err != nil {
				return
			}
			mi := make([]notification.MemberInfo, len(members))
			for i, m := range members {
				mi[i] = notification.MemberInfo{UserID: m.UserID, DisplayName: m.DisplayName}
			}
			h.notifService.NotifyHouseholdJoined(ctx, mi, joinerID, joinerName, householdName, notification.Scope{HouseholdID: hhID})
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"household": hh})
}

// ListAll returns all households the authenticated user belongs to.
func (h *HouseholdHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	households, err := h.service.ListUserHouseholds(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list households")
		return
	}
	if households == nil {
		households = []household.HouseholdWithRole{}
	}

	// Also include the active household ID from the current user
	writeJSON(w, http.StatusOK, map[string]any{
		"households": households,
	})
}

// Activate switches the user's active household.
func (h *HouseholdHandler) Activate(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Extract household ID from path: /api/households/:id/activate
	path := r.URL.Path
	// path is like /api/households/123/activate
	path = strings.TrimPrefix(path, "/api/households/")
	path = strings.TrimSuffix(path, "/activate")
	householdID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid household id")
		return
	}

	if err := h.service.SwitchHousehold(r.Context(), user.ID, householdID); err != nil {
		writeHouseholdError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "activated"})
}

func (h *HouseholdHandler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	idStr := extractID(r.URL.Path, "/api/household/members/")
	targetUserID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.UpdateMemberRole(r.Context(), user.ID, targetUserID, req.Role); err != nil {
		writeHouseholdError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *HouseholdHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	idStr := extractID(r.URL.Path, "/api/household/members/")
	targetUserID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := h.service.RemoveMember(r.Context(), user.ID, targetUserID); err != nil {
		writeHouseholdError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *HouseholdHandler) Leave(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := h.service.LeaveHousehold(r.Context(), user.ID); err != nil {
		writeHouseholdError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "left"})
}

func (h *HouseholdHandler) Transfer(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		NewOwnerID int64 `json:"newOwnerId"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.TransferOwnership(r.Context(), user.ID, req.NewOwnerID); err != nil {
		writeHouseholdError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "transferred"})
}

func extractID(path, prefix string) string {
	return strings.TrimPrefix(path, prefix)
}

func writeHouseholdError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, household.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, household.ErrNotAuthorized), errors.Is(err, household.ErrNotMember):
		writeError(w, http.StatusForbidden, "not authorized for this household")
	case errors.Is(err, household.ErrLastOwner):
		writeError(w, http.StatusForbidden, "transfer ownership before leaving or removing the last owner")
	case errors.Is(err, household.ErrNotFound):
		status := http.StatusForbidden
		if r.Method == http.MethodGet {
			status = http.StatusNotFound
		}
		writeError(w, status, "no household found")
	case errors.Is(err, household.ErrInviteNotFound), errors.Is(err, household.ErrInviteExpired):
		writeError(w, http.StatusNotFound, "invite not found")
	default:
		writeError(w, http.StatusInternalServerError, "could not update household; please try again")
	}
}
