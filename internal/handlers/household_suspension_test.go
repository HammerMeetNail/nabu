package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/household"
)

// setupSuspendTest: owner creates "Sitter House"; sitter joins as a member.
// Returns the handler, auth service, both sessions, and the sitter's user ID.
func setupSuspendTest(t *testing.T) (*HouseholdHandler, *auth.Service, auth.Session, auth.Session, int64) {
	t.Helper()
	authService := auth.NewService(auth.NewMemoryStore())
	householdStore := household.NewMemoryStore()
	householdService := household.NewService(householdStore, authService)
	handler := NewHouseholdHandler(householdService)

	owner, ownerSession := quickRegister(authService, "owner@example.com")
	sitter, sitterSession := quickRegister(authService, "sitter@example.com")
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	hh, err := householdService.CreateHousehold(ctx, "Sitter House", "", owner.ID)
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	if err := householdStore.AddMember(ctx, hh.ID, sitter.ID, household.RoleMember); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	return handler, authService, ownerSession, sitterSession, sitter.ID
}

func suspendMember(handler *HouseholdHandler, authService *auth.Service, actorSession string, targetUserID int64, suspended string) *httptest.ResponseRecorder {
	req := withUser(httptest.NewRequest(http.MethodPatch, "/api/household/members/x/access", strings.NewReader(suspended)), authService, actorSession)
	req.URL.Path = "/api/household/members/" + strconv.FormatInt(targetUserID, 10) + "/access"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.UpdateMemberAccess(rec, req)
	return rec
}

func TestUpdateMemberAccess_OwnerSuspendsSitter(t *testing.T) {
	handler, authService, ownerSession, sitterSession, sitterID := setupSuspendTest(t)

	if rec := suspendMember(handler, authService, ownerSession.ID, sitterID, `{"suspended":true}`); rec.Code != http.StatusOK {
		t.Fatalf("suspend status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Force sign-out: the sitter's existing session is now invalid.
	closedReq := withUser(httptest.NewRequest(http.MethodGet, "/api/household", nil), authService, sitterSession.ID)
	closedRec := httptest.NewRecorder()
	handler.Get(closedRec, closedReq)
	if closedRec.Code != http.StatusUnauthorized {
		t.Fatalf("old session after suspend = %d, want 401 (sessions must be revoked)", closedRec.Code)
	}

	// Signing in again shows the suspension message, not the household.
	freshUser, freshSession, err := authService.Login(context.Background(), "sitter@example.com", "password123")
	if err != nil {
		t.Fatalf("re-login after suspend: %v", err)
	}
	_ = freshUser
	getReq := withUser(httptest.NewRequest(http.MethodGet, "/api/household", nil), authService, freshSession.ID)
	getRec := httptest.NewRecorder()
	handler.Get(getRec, getReq)
	if getRec.Code != http.StatusForbidden {
		t.Fatalf("suspended GET = %d, want 403, body=%s", getRec.Code, getRec.Body.String())
	}
	body := getRec.Body.String()
	for _, want := range []string{`"access_suspended"`, "Sitter House", "temporarily disabled"} {
		if !strings.Contains(body, want) {
			t.Errorf("suspended GET body missing %q: %s", want, body)
		}
	}

	// Re-enable restores access.
	if rec := suspendMember(handler, authService, ownerSession.ID, sitterID, `{"suspended":false}`); rec.Code != http.StatusOK {
		t.Fatalf("unsuspend status = %d, body=%s", rec.Code, rec.Body.String())
	}
	getRec2 := httptest.NewRecorder()
	handler.Get(getRec2, withUser(httptest.NewRequest(http.MethodGet, "/api/household", nil), authService, freshSession.ID))
	if getRec2.Code != http.StatusOK {
		t.Fatalf("re-enabled GET = %d, want 200", getRec2.Code)
	}
}

func TestUpdateMemberAccess_SitterCannotSuspend(t *testing.T) {
	handler, authService, _, sitterSession, _ := setupSuspendTest(t)

	// The sitter targeting the owner (owner ID is sitter ID - 1 in this
	// fixture, but target identity is irrelevant: the actor must be admin+).
	rec := suspendMember(handler, authService, sitterSession.ID, 1, `{"suspended":true}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sitter suspend = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateMemberAccess_RequiresSuspendedField(t *testing.T) {
	handler, authService, ownerSession, _, sitterID := setupSuspendTest(t)

	if rec := suspendMember(handler, authService, ownerSession.ID, sitterID, `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing field = %d, want 400", rec.Code)
	}
}

func TestHouseholdGet_ReportsSuspendedFlagToOwner(t *testing.T) {
	handler, authService, ownerSession, _, sitterID := setupSuspendTest(t)

	if rec := suspendMember(handler, authService, ownerSession.ID, sitterID, `{"suspended":true}`); rec.Code != http.StatusOK {
		t.Fatalf("suspend = %d, body=%s", rec.Code, rec.Body.String())
	}

	rec := httptest.NewRecorder()
	handler.Get(rec, withUser(httptest.NewRequest(http.MethodGet, "/api/household", nil), authService, ownerSession.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("owner GET = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"suspended":true`) {
		t.Fatalf("member list missing suspended flag: %s", rec.Body.String())
	}
}
