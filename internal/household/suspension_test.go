package household_test

import (
	"context"
	"errors"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/household"
)

// revokingAuthStore also implements the session-revoker interface the
// household service uses to force-sign-out disabled members.
type revokingAuthStore struct {
	stubAuthStore
	revoked []int64
}

func (s *revokingAuthStore) RevokeSessions(_ context.Context, userID int64) error {
	s.revoked = append(s.revoked, userID)
	return nil
}

// setupSuspension builds: owner(1) + member(2) + admin(3) in household A
// ("Sitter House"), and a second household B ("Other Home") owned by
// member(2) for multi-household checks.
func setupSuspension(t *testing.T) (*household.Service, *household.MemoryStore, *revokingAuthStore, int64, int64) {
	t.Helper()
	ctx := context.Background()
	store := household.NewMemoryStore()
	auth := &revokingAuthStore{}
	svc := household.NewService(store, auth)

	home, err := svc.CreateHousehold(ctx, "Sitter House", "", 1)
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	if err := store.AddMember(ctx, home.ID, 2, household.RoleMember); err != nil {
		t.Fatalf("AddMember sitter: %v", err)
	}
	if err := store.AddMember(ctx, home.ID, 3, household.RoleAdmin); err != nil {
		t.Fatalf("AddMember admin: %v", err)
	}
	other, err := svc.CreateHousehold(ctx, "Other Home", "", 2)
	if err != nil {
		t.Fatalf("CreateHousehold other: %v", err)
	}
	return svc, store, auth, home.ID, other.ID
}

func TestSetMemberAccess_OwnerSuspendsAndRestoresMember(t *testing.T) {
	svc, store, auth, hhID, otherID := setupSuspension(t)
	ctx := context.Background()

	// Make "Sitter House" the member's active household before suspending.
	if err := svc.SwitchHousehold(ctx, 2, hhID); err != nil {
		t.Fatalf("SwitchHousehold: %v", err)
	}
	if err := svc.SetMemberAccess(ctx, 1, 2, true); err != nil {
		t.Fatalf("SetMemberAccess suspend: %v", err)
	}
	// The disabled household drops out; the pointer moves to the remaining
	// usable membership.
	id, _, err := store.GetMembership(ctx, 2)
	if err != nil || id != otherID {
		t.Fatalf("suspended member should fall back to Other Home, got %d %v", id, err)
	}
	if suspended, err := store.IsMemberSuspended(ctx, hhID, 2); err != nil || !suspended {
		t.Fatalf("IsMemberSuspended = %v, %v", suspended, err)
	}
	if len(auth.revoked) != 1 || auth.revoked[0] != 2 {
		t.Fatalf("sessions not revoked for target: %v", auth.revoked)
	}
	// Listing the household still shows the member, flagged as suspended.
	_, members, err := svc.GetHousehold(ctx, 1)
	if err != nil {
		t.Fatalf("GetHousehold: %v", err)
	}
	var found bool
	for _, m := range members {
		if m.UserID == 2 {
			found = true
			if !m.Suspended {
				t.Error("member row missing suspended flag")
			}
		}
	}
	if !found {
		t.Fatal("suspended member missing from member list")
	}

	if err := svc.SetMemberAccess(ctx, 1, 2, false); err != nil {
		t.Fatalf("SetMemberAccess unsuspend: %v", err)
	}
	if suspended, err := store.IsMemberSuspended(ctx, hhID, 2); err != nil || suspended {
		t.Fatalf("still suspended after re-enable: %v %v", suspended, err)
	}
	id, role, err := store.GetMembership(ctx, 2)
	if err != nil || id == 0 || role == "" {
		t.Fatalf("access not restored: %d %s %v", id, role, err)
	}
	// The suspended household is joinable again only by re-enable (already
	// asserted above); its membership stays intact.
	if suspended, err := store.IsMemberSuspended(ctx, hhID, 2); err != nil || suspended {
		t.Fatalf("membership flagged suspended: %v %v", suspended, err)
	}
}

func TestSetMemberAccess_AdminSuspendsMemberButNotAdminOrOwner(t *testing.T) {
	svc, store, _, _, _ := setupSuspension(t)
	ctx := context.Background()

	if err := svc.SetMemberAccess(ctx, 3, 2, true); err != nil {
		t.Fatalf("admin suspending member: %v", err)
	}
	if err := svc.SetMemberAccess(ctx, 3, 3, false); !errors.Is(err, household.ErrNotAuthorized) {
		t.Fatalf("admin targeting self: want ErrNotAuthorized, got %v", err)
	}
	if err := svc.SetMemberAccess(ctx, 3, 1, true); !errors.Is(err, household.ErrNotAuthorized) {
		t.Fatalf("admin targeting owner: want ErrNotAuthorized, got %v", err)
	}
	if suspended, err := store.IsMemberSuspended(ctx, mustActive(t, store, 1), 1); err != nil || suspended {
		t.Fatalf("owner must remain enabled: %v %v", suspended, err)
	}
	if _, _, err := store.GetMembership(ctx, 1); err != nil {
		t.Fatalf("owner lost access: %v", err)
	}
}

func mustActive(t *testing.T, store *household.MemoryStore, userID int64) int64 {
	t.Helper()
	id, _, err := store.GetMembership(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetMembership(%d): %v", userID, err)
	}
	return id
}

func TestSetMemberAccess_OwnerCannotBeSuspendedByAnyone(t *testing.T) {
	svc, _, _, _, _ := setupSuspension(t)
	ctx := context.Background()

	if err := svc.SetMemberAccess(ctx, 3, 1, true); !errors.Is(err, household.ErrNotAuthorized) {
		t.Fatalf("admin suspending owner: want ErrNotAuthorized, got %v", err)
	}
	if err := svc.SetMemberAccess(ctx, 1, 1, true); !errors.Is(err, household.ErrNotAuthorized) {
		t.Fatalf("owner suspending self: want ErrNotAuthorized, got %v", err)
	}
}

func TestSetMemberAccess_MemberActorForbidden(t *testing.T) {
	svc, _, _, _, _ := setupSuspension(t)
	ctx := context.Background()

	if err := svc.SetMemberAccess(ctx, 2, 3, true); !errors.Is(err, household.ErrNotAuthorized) {
		t.Fatalf("member actor: want ErrNotAuthorized, got %v", err)
	}
}

func TestSetMemberAccess_CrossHouseholdTargetForbidden(t *testing.T) {
	svc, store, _, _, otherID := setupSuspension(t)
	ctx := context.Background()

	// user 4 belongs only to the "Other Home" household, not "Sitter House".
	if err := store.AddMember(ctx, otherID, 4, household.RoleMember); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if err := svc.SetMemberAccess(ctx, 1, 4, true); !errors.Is(err, household.ErrNotAuthorized) {
		t.Fatalf("cross-household target: want ErrNotAuthorized, got %v", err)
	}
	if suspended, err := store.IsMemberSuspended(ctx, otherID, 4); err != nil || suspended {
		t.Fatalf("cross-household target was suspended: %v %v", suspended, err)
	}
}

func TestSetMemberAccess_SuspendMovesActiveHouseholdAndBlocksSwitch(t *testing.T) {
	svc, store, _, hhID, otherID := setupSuspension(t)
	ctx := context.Background()

	// member(2) is currently active in "Other Home" (created last). Suspend
	// their "Sitter House" membership: the pointer must stay usable, and
	// switching back to the disabled household must fail.
	if id, _, err := store.GetMembership(ctx, 2); err != nil || id != otherID {
		t.Fatalf("expected active Other Home, got %d %v", id, err)
	}
	if err := svc.SetMemberAccess(ctx, 1, 2, true); err != nil {
		t.Fatalf("suspend from owner's active household: %v", err)
	}
	if err := svc.SwitchHousehold(ctx, 2, hhID); !errors.Is(err, household.ErrNotMember) {
		t.Fatalf("switch to suspended household: want ErrNotMember, got %v", err)
	}
	// The listing hides the disabled household.
	list, err := svc.ListUserHouseholds(ctx, 2)
	if err != nil {
		t.Fatalf("ListUserHouseholds: %v", err)
	}
	for _, h := range list {
		if h.ID == hhID {
			t.Fatal("suspended household listed to its disabled member")
		}
	}

	// Re-enable, then switching works again.
	if err := svc.SetMemberAccess(ctx, 1, 2, false); err != nil {
		t.Fatalf("unsuspend: %v", err)
	}
	if err := svc.SwitchHousehold(ctx, 2, hhID); err != nil {
		t.Fatalf("switch after re-enable: %v", err)
	}
}

func TestSetMemberAccess_SuspendOnlyMembershipClearsHouseholdAndReportsMessage(t *testing.T) {
	store := household.NewMemoryStore()
	auth := &revokingAuthStore{}
	svc := household.NewService(store, auth)
	ctx := context.Background()

	hh, err := svc.CreateHousehold(ctx, "Sitter House", "", 1)
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	if err := store.AddMember(ctx, hh.ID, 2, household.RoleMember); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if err := svc.SetMemberAccess(ctx, 1, 2, true); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if id, _, err := store.GetMembership(ctx, 2); !errors.Is(err, household.ErrNotFound) {
		t.Fatalf("expected no usable membership, got %d %v", id, err)
	}
	_, _, err = svc.GetHousehold(ctx, 2)
	var suspendedErr *household.AccessSuspendedError
	if !errors.As(err, &suspendedErr) {
		t.Fatalf("GetHousehold: want AccessSuspendedError, got %v", err)
	}
	if suspendedErr.HouseholdName != "Sitter House" {
		t.Errorf("HouseholdName = %q", suspendedErr.HouseholdName)
	}
	if !errors.Is(err, household.ErrAccessSuspended) {
		t.Errorf("errors.Is(ErrAccessSuspended) = false")
	}
}

func TestSetMemberAccess_JoinWhileSuspendedRejected(t *testing.T) {
	svc, store, _, hhID, _ := setupSuspension(t)
	ctx := context.Background()

	hh, _, err := svc.GetHousehold(ctx, 1)
	if err != nil {
		t.Fatalf("GetHousehold: %v", err)
	}
	if err := svc.SetMemberAccess(ctx, 1, 2, true); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if _, err := svc.JoinHousehold(ctx, 2, hh.InviteCode); !errors.Is(err, household.ErrAlreadyMember) {
		t.Fatalf("join while suspended: want ErrAlreadyMember, got %v", err)
	}
	if suspended, err := store.IsMemberSuspended(ctx, hhID, 2); err != nil || !suspended {
		t.Fatalf("join reactivated the suspended membership: %v %v", suspended, err)
	}
}

func TestRemoveMember_StillWorksOnSuspendedMember(t *testing.T) {
	svc, store, _, hhID, _ := setupSuspension(t)
	ctx := context.Background()

	if err := svc.SetMemberAccess(ctx, 1, 2, true); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := svc.RemoveMember(ctx, 1, 2); err != nil {
		t.Fatalf("remove suspended member: %v", err)
	}
	if _, err := store.GetMembershipForHousehold(ctx, 2, hhID); !errors.Is(err, household.ErrNotMember) {
		t.Fatalf("membership should be gone, got %v", err)
	}
}
