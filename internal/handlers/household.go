package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/notification"
)

type HouseholdHandler struct {
	service        *household.Service
	notifService   *notification.Service
	householdStore household.Store
}

func NewHouseholdHandler(service *household.Service) *HouseholdHandler {
	return &HouseholdHandler{service: service}
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
		writeError(w, http.StatusNotFound, "no household found")
		return
	}

	invites, _ := h.service.GetInvites(r.Context(), user.ID)
	if invites == nil {
		invites = []household.Invite{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"household": hh,
		"members":   members,
		"invites":   invites,
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
		writeError(w, http.StatusConflict, err.Error())
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
		writeError(w, http.StatusForbidden, err.Error())
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
		writeError(w, http.StatusForbidden, err.Error())
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
		writeError(w, http.StatusNotFound, err.Error())
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
		writeError(w, http.StatusForbidden, err.Error())
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
		writeError(w, http.StatusBadRequest, err.Error())
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
		go func() {
			members, err := h.householdStore.GetMembers(context.Background(), hhID)
			if err != nil {
				return
			}
			mi := make([]notification.MemberInfo, len(members))
			for i, m := range members {
				mi[i] = notification.MemberInfo{UserID: m.UserID, DisplayName: m.DisplayName}
			}
			h.notifService.NotifyHouseholdJoined(context.Background(), mi, joinerID, joinerName, householdName)
		}()
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
		writeError(w, http.StatusForbidden, err.Error())
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
		writeError(w, http.StatusForbidden, err.Error())
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
		writeError(w, http.StatusForbidden, err.Error())
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
		writeError(w, http.StatusForbidden, err.Error())
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
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "transferred"})
}

func extractID(path, prefix string) string {
	return strings.TrimPrefix(path, prefix)
}
