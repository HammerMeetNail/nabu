package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dave/choresy/internal/auth"
	"github.com/dave/choresy/internal/household"
	"github.com/dave/choresy/internal/mail"
)

func setupHouseholdTest(t *testing.T) (*HouseholdHandler, string, *auth.Service) {
	t.Helper()
	authStore := auth.NewMemoryStore()
	authService := auth.NewService(authStore)
	mailer := mail.NewMemorySender()
	authService.SetMailer(mailer, "http://localhost:8080")
	authService.SetAuditLogger(nil)

	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)
	handler := NewHouseholdHandler(householdService)

	user, session := quickRegister(authService, "alice@example.com")
	_ = user

	return handler, session.ID, authService
}

func TestHouseholdGetNoHousehold(t *testing.T) {
	handler, sessionID, authService := setupHouseholdTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/household", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHouseholdCreate(t *testing.T) {
	handler, sessionID, authService := setupHouseholdTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/household", strings.NewReader(
		`{"name":"My Home"}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"My Home"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestHouseholdGet(t *testing.T) {
	handler, sessionID, authService := setupHouseholdTest(t)
	// Create household first
	createReq := withUser(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		`{"name":"Test Home"}`,
	)), authService, sessionID)
	createReq.Header.Set("Content-Type", "application/json")
	handler.Create(httptest.NewRecorder(), createReq)

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/household", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"Test Home"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestHouseholdLeave(t *testing.T) {
	handler, sessionID, authService := setupHouseholdTest(t)
	// Create household
	createReq := withUser(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		`{"name":"Leave Me"}`,
	)), authService, sessionID)
	createReq.Header.Set("Content-Type", "application/json")
	handler.Create(httptest.NewRecorder(), createReq)

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/household/leave", nil), authService, sessionID)
	rec := httptest.NewRecorder()

	handler.Leave(rec, req)

	// Sole owner cannot leave — that's correct behavior (ErrLastOwner)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestHouseholdRequiresAuth(t *testing.T) {
	handler, _, _ := setupHouseholdTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/household", nil)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// setupHouseholdWithHome creates a household and returns a session for the owner.
func setupHouseholdWithHome(t *testing.T) (*HouseholdHandler, string, *auth.Service) {
	t.Helper()
	handler, sessionID, authService := setupHouseholdTest(t)
	createReq := withUser(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		`{"name":"My Home"}`,
	)), authService, sessionID)
	createReq.Header.Set("Content-Type", "application/json")
	handler.Create(httptest.NewRecorder(), createReq)
	return handler, sessionID, authService
}

func TestHouseholdUpdate(t *testing.T) {
	handler, sessionID, authService := setupHouseholdWithHome(t)

	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/household", strings.NewReader(
		`{"name":"Renamed Home"}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestHouseholdUpdateNoAuth(t *testing.T) {
	handler, _, _ := setupHouseholdTest(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/household", strings.NewReader(`{"name":"X"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHouseholdCreateAndListAndDeleteInvite(t *testing.T) {
	handler, sessionID, authService := setupHouseholdWithHome(t)

	// CreateInvite
	createReq := withUser(httptest.NewRequest(http.MethodPost, "/api/household/invites", nil), authService, sessionID)
	createRec := httptest.NewRecorder()
	handler.CreateInvite(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("CreateInvite: status = %d, body=%s", createRec.Code, createRec.Body.String())
	}
	if !strings.Contains(createRec.Body.String(), `"invite"`) {
		t.Fatalf("CreateInvite body = %s", createRec.Body.String())
	}

	// ListInvites
	listReq := withUser(httptest.NewRequest(http.MethodGet, "/api/household/invites", nil), authService, sessionID)
	listRec := httptest.NewRecorder()
	handler.ListInvites(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListInvites: status = %d, body=%s", listRec.Code, listRec.Body.String())
	}

	// DeleteInvite (id=1 — the first invite created)
	delReq := withUser(httptest.NewRequest(http.MethodDelete, "/api/household/invites/1", nil), authService, sessionID)
	delRec := httptest.NewRecorder()
	handler.DeleteInvite(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("DeleteInvite: status = %d, body=%s", delRec.Code, delRec.Body.String())
	}
}

func TestHouseholdCreateInviteNoAuth(t *testing.T) {
	handler, _, _ := setupHouseholdTest(t)
	rec := httptest.NewRecorder()
	handler.CreateInvite(rec, httptest.NewRequest(http.MethodPost, "/api/household/invites", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHouseholdListInvitesNoAuth(t *testing.T) {
	handler, _, _ := setupHouseholdTest(t)
	rec := httptest.NewRecorder()
	handler.ListInvites(rec, httptest.NewRequest(http.MethodGet, "/api/household/invites", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHouseholdDeleteInviteInvalidID(t *testing.T) {
	handler, sessionID, authService := setupHouseholdWithHome(t)
	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/household/invites/abc", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.DeleteInvite(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHouseholdJoin(t *testing.T) {
	// Create owner with household
	ownerHandler, ownerSession, ownerAuth := setupHouseholdWithHome(t)

	// Create invite
	invReq := withUser(httptest.NewRequest(http.MethodPost, "/", nil), ownerAuth, ownerSession)
	invRec := httptest.NewRecorder()
	ownerHandler.CreateInvite(invRec, invReq)
	if invRec.Code != http.StatusCreated {
		t.Fatalf("CreateInvite: %d %s", invRec.Code, invRec.Body.String())
	}
	// Parse invite code
	body := invRec.Body.String()
	codeStart := strings.Index(body, `"code":"`) + len(`"code":"`)
	codeEnd := strings.Index(body[codeStart:], `"`)
	inviteCode := body[codeStart : codeStart+codeEnd]

	// Register a second user
	authStore := auth.NewMemoryStore()
	authService2 := auth.NewService(authStore)
	mailer2 := mail.NewMemorySender()
	authService2.SetMailer(mailer2, "http://localhost:8080")
	authService2.SetAuditLogger(nil)
	householdStore2 := household.NewMemoryStore()
	householdService2 := household.NewService(householdStore2, authService2)
	handler2 := NewHouseholdHandler(householdService2)

	// The invite lookup is against owner's service — reuse ownerHandler
	_, session2 := quickRegister(ownerAuth, "bob@example.com")

	joinReq := withUser(httptest.NewRequest(http.MethodPost, "/api/household/join", strings.NewReader(
		`{"inviteCode":"`+inviteCode+`"}`,
	)), ownerAuth, session2.ID)
	joinReq.Header.Set("Content-Type", "application/json")
	joinRec := httptest.NewRecorder()
	ownerHandler.Join(joinRec, joinReq)

	_ = handler2 // created but used only to show the pattern
	if joinRec.Code != http.StatusOK {
		t.Fatalf("Join: status = %d, body=%s", joinRec.Code, joinRec.Body.String())
	}
}

func TestHouseholdJoinNoAuth(t *testing.T) {
	handler, _, _ := setupHouseholdTest(t)
	req := httptest.NewRequest(http.MethodPost, "/api/household/join", strings.NewReader(`{"inviteCode":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Join(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHouseholdTransferNoAuth(t *testing.T) {
	handler, _, _ := setupHouseholdTest(t)
	req := httptest.NewRequest(http.MethodPost, "/api/household/transfer", strings.NewReader(`{"newOwnerId":2}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Transfer(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHouseholdTransferBadTarget(t *testing.T) {
	handler, sessionID, authService := setupHouseholdWithHome(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/household/transfer", strings.NewReader(
		`{"newOwnerId":9999}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Transfer(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestHouseholdUpdateMemberRoleNoAuth(t *testing.T) {
	handler, _, _ := setupHouseholdTest(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/household/members/2", strings.NewReader(`{"role":"member"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.UpdateMemberRole(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHouseholdUpdateMemberRoleInvalidID(t *testing.T) {
	handler, sessionID, authService := setupHouseholdWithHome(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/household/members/abc", strings.NewReader(
		`{"role":"member"}`,
	)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.UpdateMemberRole(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHouseholdRemoveMemberNoAuth(t *testing.T) {
	handler, _, _ := setupHouseholdTest(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/household/members/2", nil)
	rec := httptest.NewRecorder()
	handler.RemoveMember(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHouseholdRemoveMemberInvalidID(t *testing.T) {
	handler, sessionID, authService := setupHouseholdWithHome(t)
	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/household/members/abc", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.RemoveMember(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// setupTwoMemberHousehold creates a household with owner (ownerSession) and member (memberSession).
func setupTwoMemberHousehold(t *testing.T) (*HouseholdHandler, string, string, *auth.Service) {
	t.Helper()
	handler, ownerSession, authService := setupHouseholdWithHome(t)

	_, user2Session := quickRegister(authService, "carol@example.com")

	invReq := withUser(httptest.NewRequest(http.MethodPost, "/api/household/invites", nil), authService, ownerSession)
	invRec := httptest.NewRecorder()
	handler.CreateInvite(invRec, invReq)
	if invRec.Code != http.StatusCreated {
		t.Fatalf("CreateInvite: %d %s", invRec.Code, invRec.Body.String())
	}

	body := invRec.Body.String()
	codeStart := strings.Index(body, `"code":"`) + len(`"code":"`)
	codeEnd := strings.Index(body[codeStart:], `"`)
	inviteCode := body[codeStart : codeStart+codeEnd]

	joinReq := withUser(httptest.NewRequest(http.MethodPost, "/api/household/join", strings.NewReader(
		`{"inviteCode":"`+inviteCode+`"}`,
	)), authService, user2Session.ID)
	joinReq.Header.Set("Content-Type", "application/json")
	joinRec := httptest.NewRecorder()
	handler.Join(joinRec, joinReq)
	if joinRec.Code != http.StatusOK {
		t.Fatalf("Join: %d %s", joinRec.Code, joinRec.Body.String())
	}

	return handler, ownerSession, user2Session.ID, authService
}

// --- Create ---

func TestHouseholdCreateNoAuth(t *testing.T) {
	handler, _, _ := setupHouseholdTest(t)
	req := httptest.NewRequest(http.MethodPost, "/api/household", strings.NewReader(`{"name":"X"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHouseholdCreateInvalidBody(t *testing.T) {
	handler, sessionID, authService := setupHouseholdTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/household", strings.NewReader(`not-json`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHouseholdCreateSecondHousehold(t *testing.T) {
	// Multi-household: a user can create more than one household.
	handler, sessionID, authService := setupHouseholdWithHome(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/household", strings.NewReader(`{"name":"Second Home"}`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
}

// --- Update ---

func TestHouseholdUpdateInvalidBody(t *testing.T) {
	handler, sessionID, authService := setupHouseholdWithHome(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/household", strings.NewReader(`not-json`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHouseholdUpdateNoHousehold(t *testing.T) {
	handler, sessionID, authService := setupHouseholdTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/household", strings.NewReader(`{"name":"X"}`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// --- CreateInvite ---

func TestHouseholdCreateInviteNoHousehold(t *testing.T) {
	handler, sessionID, authService := setupHouseholdTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/household/invites", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.CreateInvite(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// --- ListInvites ---

func TestHouseholdListInvitesNoHousehold(t *testing.T) {
	handler, sessionID, authService := setupHouseholdTest(t)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/household/invites", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.ListInvites(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// --- DeleteInvite ---

func TestHouseholdDeleteInviteNoAuth(t *testing.T) {
	handler, _, _ := setupHouseholdTest(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/household/invites/1", nil)
	rec := httptest.NewRecorder()
	handler.DeleteInvite(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHouseholdDeleteInviteNotFound(t *testing.T) {
	// Member (non-owner) trying to delete an invite → service error → 403
	handler, _, memberSession, authService := setupTwoMemberHousehold(t)
	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/household/invites/1", nil), authService, memberSession)
	rec := httptest.NewRecorder()
	handler.DeleteInvite(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// --- Join ---

func TestHouseholdJoinInvalidBody(t *testing.T) {
	handler, sessionID, authService := setupHouseholdTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/household/join", strings.NewReader(`not-json`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Join(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHouseholdJoinBadCode(t *testing.T) {
	handler, sessionID, authService := setupHouseholdTest(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/household/join", strings.NewReader(`{"inviteCode":"badcode"}`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Join(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// --- UpdateMemberRole ---

func TestHouseholdUpdateMemberRoleInvalidBody(t *testing.T) {
	handler, sessionID, authService := setupHouseholdWithHome(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/household/members/2", strings.NewReader(`not-json`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.UpdateMemberRole(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHouseholdUpdateMemberRoleNoHousehold(t *testing.T) {
	handler, sessionID, authService := setupHouseholdTest(t)
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/household/members/2", strings.NewReader(`{"role":"admin"}`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.UpdateMemberRole(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestHouseholdUpdateMemberRoleSuccess(t *testing.T) {
	handler, ownerSession, memberSession, authService := setupTwoMemberHousehold(t)
	// Look up member's user ID by authenticating them
	memberUser, err := authService.Authenticate(httptest.NewRequest(http.MethodGet, "/", nil).Context(), memberSession)
	if err != nil {
		t.Fatalf("Authenticate member: %v", err)
	}
	req := withUser(httptest.NewRequest(http.MethodPatch,
		"/api/household/members/"+strings.TrimSpace(fmt.Sprintf("%d", memberUser.ID)),
		strings.NewReader(`{"role":"admin"}`),
	), authService, ownerSession)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.UpdateMemberRole(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

// --- RemoveMember ---

func TestHouseholdRemoveMemberNoHousehold(t *testing.T) {
	handler, sessionID, authService := setupHouseholdTest(t)
	req := withUser(httptest.NewRequest(http.MethodDelete, "/api/household/members/2", nil), authService, sessionID)
	rec := httptest.NewRecorder()
	handler.RemoveMember(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestHouseholdRemoveMemberSuccess(t *testing.T) {
	handler, ownerSession, memberSession, authService := setupTwoMemberHousehold(t)
	memberUser, err := authService.Authenticate(httptest.NewRequest(http.MethodGet, "/", nil).Context(), memberSession)
	if err != nil {
		t.Fatalf("Authenticate member: %v", err)
	}
	req := withUser(httptest.NewRequest(http.MethodDelete,
		"/api/household/members/"+strings.TrimSpace(fmt.Sprintf("%d", memberUser.ID)),
		nil,
	), authService, ownerSession)
	rec := httptest.NewRecorder()
	handler.RemoveMember(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

// --- Leave ---

func TestHouseholdLeaveNoAuth(t *testing.T) {
	handler, _, _ := setupHouseholdTest(t)
	req := httptest.NewRequest(http.MethodPost, "/api/household/leave", nil)
	rec := httptest.NewRecorder()
	handler.Leave(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHouseholdLeaveSuccess(t *testing.T) {
	handler, _, memberSession, authService := setupTwoMemberHousehold(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/household/leave", nil), authService, memberSession)
	rec := httptest.NewRecorder()
	handler.Leave(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

// --- Transfer ---

func TestHouseholdTransferInvalidBody(t *testing.T) {
	handler, sessionID, authService := setupHouseholdWithHome(t)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/household/transfer", strings.NewReader(`not-json`)), authService, sessionID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Transfer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHouseholdTransferSuccess(t *testing.T) {
	handler, ownerSession, memberSession, authService := setupTwoMemberHousehold(t)
	memberUser, err := authService.Authenticate(httptest.NewRequest(http.MethodGet, "/", nil).Context(), memberSession)
	if err != nil {
		t.Fatalf("Authenticate member: %v", err)
	}
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/household/transfer", strings.NewReader(
		fmt.Sprintf(`{"newOwnerId":%d}`, memberUser.ID),
	)), authService, ownerSession)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Transfer(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}
