package household_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/household"
)

// The auth service resolves canonical membership instead of keeping a copy.
type stubAuthStore struct{ resolve auth.MembershipResolver }

func (s *stubAuthStore) SetMembershipResolver(resolve auth.MembershipResolver) { s.resolve = resolve }

func newSvc() (*household.Service, *household.MemoryStore, *stubAuthStore) {
	store := household.NewMemoryStore()
	auth := &stubAuthStore{}
	svc := household.NewService(store, auth)
	return svc, store, auth
}

// ─── CreateHousehold ──────────────────────────────────────────────────────────

func TestCreateHousehold_Basic(t *testing.T) {
	svc, _, auth := newSvc()
	ctx := context.Background()

	hh, err := svc.CreateHousehold(ctx, "Smith Family", "", 1)
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	if hh.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if hh.Name != "Smith Family" {
		t.Errorf("Name = %q", hh.Name)
	}
	id, role, err := auth.resolve(ctx, 1)
	if err != nil || id == nil || *id != hh.ID || role != household.RoleOwner {
		t.Fatalf("noncanonical auth profile: %v %s %v", id, role, err)
	}

}

func TestCreateHousehold_EmptyNameError(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.CreateHousehold(context.Background(), "", "", 1)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCreateHousehold_MultipleAllowed(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, err := svc.CreateHousehold(ctx, "First", "", 1)
	if err != nil {
		t.Fatalf("first CreateHousehold: %v", err)
	}
	// Multi-household: the same user can create (and own) a second household.
	hh2, err := svc.CreateHousehold(ctx, "Second", "", 1)
	if err != nil {
		t.Fatalf("second CreateHousehold should succeed in multi-household mode: %v", err)
	}
	if hh2.Name != "Second" {
		t.Errorf("Name = %q, want Second", hh2.Name)
	}
}

// ─── GetHousehold ─────────────────────────────────────────────────────────────

func TestGetHousehold_Basic(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, _ = svc.CreateHousehold(ctx, "Jones", "", 1)

	hh, members, err := svc.GetHousehold(ctx, 1)
	if err != nil {
		t.Fatalf("GetHousehold: %v", err)
	}
	if hh.Name != "Jones" {
		t.Errorf("Name = %q", hh.Name)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0].Role != household.RoleOwner {
		t.Errorf("Role = %q", members[0].Role)
	}
}

func TestGetAdminHouseholdID_Roles(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()
	hh, err := svc.CreateHousehold(ctx, "Export Home", "", 1)
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	if got, err := svc.GetAdminHouseholdID(ctx, 1); err != nil || got != hh.ID {
		t.Fatalf("owner authorization = (%d, %v), want household %d", got, err, hh.ID)
	}
	if err := store.AddMember(ctx, hh.ID, 2, household.RoleAdmin); err != nil {
		t.Fatalf("AddMember admin: %v", err)
	}
	if got, err := svc.GetAdminHouseholdID(ctx, 2); err != nil || got != hh.ID {
		t.Fatalf("admin authorization = (%d, %v), want household %d", got, err, hh.ID)
	}
	if err := store.AddMember(ctx, hh.ID, 3, household.RoleMember); err != nil {
		t.Fatalf("AddMember member: %v", err)
	}
	if _, err := svc.GetAdminHouseholdID(ctx, 3); err != household.ErrNotAuthorized {
		t.Fatalf("member authorization error = %v, want ErrNotAuthorized", err)
	}
}

func TestGetHousehold_NotMember(t *testing.T) {
	svc, _, _ := newSvc()
	_, _, err := svc.GetHousehold(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error for non-member")
	}
}

// ─── UpdateHousehold ──────────────────────────────────────────────────────────

func TestUpdateHousehold_ByOwner(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, _ = svc.CreateHousehold(ctx, "Old Name", "", 1)
	err := svc.UpdateHousehold(ctx, 1, "New Name", "")
	if err != nil {
		t.Fatalf("UpdateHousehold: %v", err)
	}
	hh, _, _ := svc.GetHousehold(ctx, 1)
	if hh.Name != "New Name" {
		t.Errorf("Name = %q", hh.Name)
	}
}

func TestUpdateHousehold_EmptyName(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, _ = svc.CreateHousehold(ctx, "Test", "", 1)
	err := svc.UpdateHousehold(ctx, 1, "", "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestUpdateHousehold_NotAuthorized(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, _ = svc.CreateHousehold(ctx, "Test", "", 1)
	// Add a plain member
	store := household.NewMemoryStore()
	_ = store // use the same svc
	// Create a household and get the real store's AddMember via join
	// For simplicity, create a fresh scenario
	svc2, store2, _ := newSvc()
	hh, _ := svc2.CreateHousehold(ctx, "X", "", 1)
	_ = hh
	_ = store2.AddMember(ctx, hh.ID, 2, household.RoleMember)

	err := svc2.UpdateHousehold(ctx, 2, "Hacked", "")
	if err == nil {
		t.Fatal("expected error: member cannot update household")
	}
}

// ─── Invites ──────────────────────────────────────────────────────────────────

func TestCreateInvite_OwnerCanCreate(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, _ = svc.CreateHousehold(ctx, "Test", "", 1)
	inv, err := svc.CreateInvite(ctx, 1)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if inv.Code == "" {
		t.Error("expected non-empty invite code")
	}
	if inv.HouseholdID == 0 {
		t.Error("expected non-zero HouseholdID")
	}
}

func TestCreateInvite_NonOwnerFails(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	_, err := svc.CreateInvite(ctx, 2)
	if err == nil {
		t.Fatal("expected error: member cannot create invite")
	}
}

func TestGetInvites_ReturnsList(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, _ = svc.CreateHousehold(ctx, "Test", "", 1)
	_, _ = svc.CreateInvite(ctx, 1)
	_, _ = svc.CreateInvite(ctx, 1)

	invites, err := svc.GetInvites(ctx, 1)
	if err != nil {
		t.Fatalf("GetInvites: %v", err)
	}
	if len(invites) != 2 {
		t.Errorf("expected 2 invites, got %d", len(invites))
	}
}

func TestDeleteInvite_RemovesIt(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, _ = svc.CreateHousehold(ctx, "Test", "", 1)
	inv, _ := svc.CreateInvite(ctx, 1)

	err := svc.DeleteInvite(ctx, 1, inv.ID)
	if err != nil {
		t.Fatalf("DeleteInvite: %v", err)
	}

	invites, _ := svc.GetInvites(ctx, 1)
	if len(invites) != 0 {
		t.Errorf("expected 0 invites after delete, got %d", len(invites))
	}
}

// ─── JoinHousehold ────────────────────────────────────────────────────────────

func TestJoinHousehold_ViaOneTimeInvite(t *testing.T) {
	svc, _, auth := newSvc()
	ctx := context.Background()

	_, _ = svc.CreateHousehold(ctx, "Test", "", 1)
	inv, _ := svc.CreateInvite(ctx, 1)

	joined, err := svc.JoinHousehold(ctx, 2, inv.Code)
	if err != nil {
		t.Fatalf("JoinHousehold: %v", err)
	}
	if joined.Name != "Test" {
		t.Errorf("Joined household Name = %q", joined.Name)
	}
	id, role, err := auth.resolve(ctx, 2)
	if err != nil || id == nil || *id != joined.ID || role != household.RoleMember {
		t.Fatalf("noncanonical joined profile: %v %s %v", id, role, err)
	}

}

func TestJoinHousehold_AlreadyMember(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, _ = svc.CreateHousehold(ctx, "Test", "", 1)
	inv, _ := svc.CreateInvite(ctx, 1)
	_, _ = svc.JoinHousehold(ctx, 2, inv.Code)

	// Try joining again
	inv2, _ := svc.CreateInvite(ctx, 1)
	_, err := svc.JoinHousehold(ctx, 2, inv2.Code)
	if err == nil {
		t.Fatal("expected error: already a member")
	}
}

func TestJoinHousehold_InvalidCode(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, _ = svc.CreateHousehold(ctx, "Test", "", 1)
	_, err := svc.JoinHousehold(ctx, 2, "BADCODE")
	if err == nil {
		t.Fatal("expected error for invalid invite code")
	}
}

// TestJoinHousehold_ExhaustedInvite verifies that a consumed one-time invite
// can never add a membership: the join fails and no member is added.
func TestJoinHousehold_ExhaustedInvite(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_, _ = store.CreateInvite(ctx, hh.ID, 1, "EXHAUSTED1", 1)
	// Consume the single allowed use.
	if err := store.UseInvite(ctx, "EXHAUSTED1"); err != nil {
		t.Fatalf("UseInvite: %v", err)
	}

	_, err := svc.JoinHousehold(ctx, 2, "EXHAUSTED1")
	if err != household.ErrInviteNotFound {
		t.Fatalf("JoinHousehold with exhausted invite: err = %v, want ErrInviteNotFound", err)
	}
	// No membership should have been added for user 2.
	_, members, _ := svc.GetHousehold(ctx, 1)
	if len(members) != 1 {
		t.Errorf("expected 1 member after rejected join, got %d", len(members))
	}
}

func TestJoinHousehold_ViaPermanentCode(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	// Use the household's permanent invite code
	joined, err := svc.JoinHousehold(ctx, 2, hh.InviteCode)
	if err != nil {
		t.Fatalf("JoinHousehold via permanent code: %v", err)
	}
	if joined.ID != hh.ID {
		t.Errorf("joined wrong household: %d", joined.ID)
	}
}

// ─── UpdateMemberRole ─────────────────────────────────────────────────────────

func TestUpdateMemberRole_OwnerPromotesMember(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	err := svc.UpdateMemberRole(ctx, 1, 2, household.RoleAdmin)
	if err != nil {
		t.Fatalf("UpdateMemberRole: %v", err)
	}
}

func TestUpdateMemberRole_NonOwnerFails(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)
	_ = store.AddMember(ctx, hh.ID, 3, household.RoleMember)

	err := svc.UpdateMemberRole(ctx, 2, 3, household.RoleAdmin)
	if err == nil {
		t.Fatal("expected error: member cannot change roles")
	}
}

func TestUpdateMemberRole_InvalidRole(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	err := svc.UpdateMemberRole(ctx, 1, 2, "superadmin")
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestUpdateMemberRole_CannotDemoteLastOwner(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	// Try to demote the only owner
	err := svc.UpdateMemberRole(ctx, 1, 1, household.RoleMember)
	if err == nil {
		t.Fatal("expected error: cannot change role of last owner")
	}
}

// ─── RemoveMember ─────────────────────────────────────────────────────────────

func TestRemoveMember_OwnerRemovesMember(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	err := svc.RemoveMember(ctx, 1, 2)
	if err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	_, members, _ := svc.GetHousehold(ctx, 1)
	if len(members) != 1 {
		t.Errorf("expected 1 member after remove, got %d", len(members))
	}
}

func TestRemoveMember_NonOwnerFails(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)
	_ = store.AddMember(ctx, hh.ID, 3, household.RoleMember)

	err := svc.RemoveMember(ctx, 2, 3)
	if err == nil {
		t.Fatal("expected error: non-owner cannot remove members")
	}
}

func TestRemoveMember_CannotRemoveSelf(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, _ = svc.CreateHousehold(ctx, "Test", "", 1)
	err := svc.RemoveMember(ctx, 1, 1)
	if !errors.Is(err, household.ErrNotAuthorized) {
		t.Fatalf("self removal = %v, want ErrNotAuthorized", err)
	}
}

// ─── LeaveHousehold ───────────────────────────────────────────────────────────

func TestLeaveHousehold_MemberLeaves(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	err := svc.LeaveHousehold(ctx, 2)
	if err != nil {
		t.Fatalf("LeaveHousehold: %v", err)
	}
}

func TestLeaveHousehold_LastOwnerCannotLeave(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, _ = svc.CreateHousehold(ctx, "Test", "", 1)
	err := svc.LeaveHousehold(ctx, 1)
	if err == nil {
		t.Fatal("expected error: last owner cannot leave")
	}
}

func TestLeaveHousehold_OwnerLeavesIfAnotherOwnerExists(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleOwner)

	err := svc.LeaveHousehold(ctx, 1)
	if err != nil {
		t.Fatalf("LeaveHousehold with second owner: %v", err)
	}
}

// ─── TransferOwnership ────────────────────────────────────────────────────────

func TestTransferOwnership(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	err := svc.TransferOwnership(ctx, 1, 2)
	if err != nil {
		t.Fatalf("TransferOwnership: %v", err)
	}

	_, members, _ := svc.GetHousehold(ctx, 2)
	roleByUser := map[int64]string{}
	for _, m := range members {
		roleByUser[m.UserID] = m.Role
	}
	if roleByUser[2] != household.RoleOwner {
		t.Errorf("user 2 should be owner, got %q", roleByUser[2])
	}
	if roleByUser[1] != household.RoleAdmin {
		t.Errorf("user 1 should be admin after transfer, got %q", roleByUser[1])
	}
}

func TestTransferOwnership_NonOwnerFails(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	err := svc.TransferOwnership(ctx, 2, 1)
	if err == nil {
		t.Fatal("expected error: non-owner cannot transfer")
	}
}

func TestTransferOwnership_TargetNotMember(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, _ = svc.CreateHousehold(ctx, "Test", "", 1)
	err := svc.TransferOwnership(ctx, 1, 999)
	if err == nil {
		t.Fatal("expected error: target is not a member")
	}
}

// ─── GetInvites / DeleteInvite non-owner ─────────────────────────────────────

func TestGetInvites_NonOwnerFails(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	_, err := svc.GetInvites(ctx, 2)
	if err == nil {
		t.Fatal("expected error: member cannot list invites")
	}
}

func TestDeleteInvite_NonOwnerFails(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)
	inv, _ := svc.CreateInvite(ctx, 1)

	err := svc.DeleteInvite(ctx, 2, inv.ID)
	if err == nil {
		t.Fatal("expected error: member cannot delete invite")
	}
}

// ─── F11: Cross-household member removal ──────────────────────────────────────

// TestRemoveMemberCrossHouseholdBlocked verifies that an owner of household A
// cannot remove a member of a different household B.
func TestRemoveMemberCrossHouseholdBlocked(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	// Household A: owner is user 1, member is user 2.
	hhA, _ := svc.CreateHousehold(ctx, "Household A", "A", 1)
	_ = store.AddMember(ctx, hhA.ID, 2, household.RoleMember)

	// Household B: owner is user 3, member is user 4.
	hhB, _ := svc.CreateHousehold(ctx, "Household B", "B", 3)
	_ = store.AddMember(ctx, hhB.ID, 4, household.RoleMember)

	// Owner of A tries to remove a member of B — must fail.
	err := svc.RemoveMember(ctx, 1, 4)
	if err != household.ErrNotAuthorized {
		t.Fatalf("RemoveMember cross-household: error = %v, want ErrNotAuthorized", err)
	}
}

// TestUpdateMemberRoleCrossHouseholdBlocked verifies that an owner of household A
// cannot change the role of a member in a different household B.
func TestUpdateMemberRoleCrossHouseholdBlocked(t *testing.T) {
	svc, store, _ := newSvc()
	ctx := context.Background()

	// Household A: owner is user 1.
	_, _ = svc.CreateHousehold(ctx, "Household A", "A", 1)

	// Household B: owner is user 2, member is user 3.
	hhB, _ := svc.CreateHousehold(ctx, "Household B", "B", 2)
	_ = store.AddMember(ctx, hhB.ID, 3, household.RoleMember)

	// Owner of A tries to change role of member in B — must fail.
	err := svc.UpdateMemberRole(ctx, 1, 3, household.RoleAdmin)
	if err != household.ErrNotAuthorized {
		t.Fatalf("UpdateMemberRole cross-household: error = %v, want ErrNotAuthorized", err)
	}
}

// TestDeleteInviteCrossHouseholdBlocked verifies that an owner of household A
// cannot delete an invite belonging to household B.
func TestDeleteInviteCrossHouseholdBlocked(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()

	// Household A: owner is user 1.
	_, _ = svc.CreateHousehold(ctx, "Household A", "A", 1)

	// Household B: owner is user 2.
	_, _ = svc.CreateHousehold(ctx, "Household B", "B", 2)
	invB, _ := svc.CreateInvite(ctx, 2)

	// Owner of A tries to delete an invite from B — must fail.
	err := svc.DeleteInvite(ctx, 1, invB.ID)
	if err != household.ErrNotAuthorized {
		t.Fatalf("DeleteInvite cross-household: error = %v, want ErrNotAuthorized", err)
	}
}

// ─── Finding #10: name/initials caps ─────────────────────────────────────────

func TestCreateHousehold_NameTooLong(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()
	_, err := svc.CreateHousehold(ctx, strings.Repeat("a", 61), "", 1)
	if err == nil {
		t.Fatal("CreateHousehold accepted a 61-rune name")
	}
}

func TestCreateHousehold_InitialsTooLong(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()
	_, err := svc.CreateHousehold(ctx, "My Home", strings.Repeat("a", 9), 1)
	if err == nil {
		t.Fatal("CreateHousehold accepted 9-rune initials")
	}
}

func TestCreateHousehold_AtLimitAccepted(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()
	hh, err := svc.CreateHousehold(ctx, strings.Repeat("a", 60), strings.Repeat("a", 8), 1)
	if err != nil {
		t.Fatalf("CreateHousehold rejected at-limit input: %v", err)
	}
	if hh.Name != strings.Repeat("a", 60) {
		t.Errorf("Name = %q", hh.Name)
	}
}

func TestUpdateHousehold_NameTooLong(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()
	if _, err := svc.CreateHousehold(ctx, "My Home", "MH", 1); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	if err := svc.UpdateHousehold(ctx, 1, strings.Repeat("a", 61), ""); err == nil {
		t.Fatal("UpdateHousehold accepted a 61-rune name")
	}
	// The original name must be untouched.
	hh, _, err := svc.GetHousehold(ctx, 1)
	if err != nil {
		t.Fatalf("GetHousehold: %v", err)
	}
	if hh.Name != "My Home" {
		t.Errorf("Name = %q after rejected update", hh.Name)
	}
}

func TestUpdateHousehold_InitialsTooLong(t *testing.T) {
	svc, _, _ := newSvc()
	ctx := context.Background()
	if _, err := svc.CreateHousehold(ctx, "My Home", "MH", 1); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	if err := svc.UpdateHousehold(ctx, 1, "My Home", strings.Repeat("a", 9)); err == nil {
		t.Fatal("UpdateHousehold accepted 9-rune initials")
	}
}
