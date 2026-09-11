package account

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/household"
)

func tomorrow() time.Time { return time.Now().Add(24 * time.Hour) }

func setup(t *testing.T) (*Service, auth.Store, household.Store) {
	t.Helper()
	users := auth.NewMemoryStore()
	households := household.NewMemoryStore()
	return NewService(users, households), users, households
}

func mustCreateUser(t *testing.T, users auth.Store, email string) auth.User {
	t.Helper()
	u, err := users.CreateUser(context.Background(), email, "x")
	if err != nil {
		t.Fatalf("CreateUser(%s): %v", email, err)
	}
	return u
}

func TestDeleteAccount_NoHousehold(t *testing.T) {
	svc, users, _ := setup(t)
	ctx := context.Background()
	u := mustCreateUser(t, users, "solo@test.com")

	if err := svc.DeleteAccount(ctx, u.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if _, err := users.GetUserByID(ctx, u.ID); err == nil {
		t.Fatal("user still exists after deletion")
	}
}

func TestDeleteAccount_SoleMemberDeletesHousehold(t *testing.T) {
	svc, users, households := setup(t)
	ctx := context.Background()
	u := mustCreateUser(t, users, "owner@test.com")
	hh, err := households.CreateHousehold(ctx, "Solo Home", "SH", u.ID)
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	if err := svc.DeleteAccount(ctx, u.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if _, err := households.GetHousehold(ctx, hh.ID); err == nil {
		t.Fatal("sole-member household still exists after deletion")
	}
	if _, err := users.GetUserByID(ctx, u.ID); err == nil {
		t.Fatal("user still exists after deletion")
	}
}

func TestDeleteAccount_OnlyOwnerWithMembersBlocked(t *testing.T) {
	svc, users, households := setup(t)
	ctx := context.Background()
	owner := mustCreateUser(t, users, "owner@test.com")
	member := mustCreateUser(t, users, "member@test.com")
	hh, _ := households.CreateHousehold(ctx, "Family", "F", owner.ID)
	if err := households.AddMember(ctx, hh.ID, member.ID, household.RoleMember); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	err := svc.DeleteAccount(ctx, owner.ID)
	var transfer ErrMustTransferOwnership
	if !errors.As(err, &transfer) {
		t.Fatalf("err = %v, want ErrMustTransferOwnership", err)
	}
	if transfer.HouseholdName != "Family" {
		t.Fatalf("household name = %q, want Family", transfer.HouseholdName)
	}
	// Nothing must have been mutated.
	if _, err := users.GetUserByID(ctx, owner.ID); err != nil {
		t.Fatal("owner was deleted despite rejection")
	}
	if _, err := households.GetHousehold(ctx, hh.ID); err != nil {
		t.Fatal("household was deleted despite rejection")
	}
	if members, _ := households.GetMembers(ctx, hh.ID); len(members) != 2 {
		t.Fatalf("members = %d, want 2", len(members))
	}
}

func TestDeleteAccount_SecondOwnerAllowsDeletion(t *testing.T) {
	svc, users, households := setup(t)
	ctx := context.Background()
	owner := mustCreateUser(t, users, "owner@test.com")
	coOwner := mustCreateUser(t, users, "co@test.com")
	hh, _ := households.CreateHousehold(ctx, "Family", "F", owner.ID)
	if err := households.AddMember(ctx, hh.ID, coOwner.ID, household.RoleOwner); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	if err := svc.DeleteAccount(ctx, owner.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if _, err := households.GetHousehold(ctx, hh.ID); err != nil {
		t.Fatal("household should survive; the co-owner still lives there")
	}
	members, _ := households.GetMembers(ctx, hh.ID)
	if len(members) != 1 || members[0].UserID != coOwner.ID {
		t.Fatalf("members = %+v, want just the co-owner", members)
	}
	if _, err := users.GetUserByID(ctx, owner.ID); err == nil {
		t.Fatal("user still exists after deletion")
	}
}

func TestDeleteAccount_MemberLeavesHousehold(t *testing.T) {
	svc, users, households := setup(t)
	ctx := context.Background()
	owner := mustCreateUser(t, users, "owner@test.com")
	member := mustCreateUser(t, users, "member@test.com")
	hh, _ := households.CreateHousehold(ctx, "Family", "F", owner.ID)
	if err := households.AddMember(ctx, hh.ID, member.ID, household.RoleMember); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	if err := svc.DeleteAccount(ctx, member.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if _, err := households.GetHousehold(ctx, hh.ID); err != nil {
		t.Fatal("household should survive member's account deletion")
	}
	members, _ := households.GetMembers(ctx, hh.ID)
	if len(members) != 1 || members[0].UserID != owner.ID {
		t.Fatalf("members = %+v, want just the owner", members)
	}
}

func TestDeleteAccount_MixedMemberships(t *testing.T) {
	svc, users, households := setup(t)
	ctx := context.Background()
	u := mustCreateUser(t, users, "multi@test.com")
	other := mustCreateUser(t, users, "other@test.com")
	// u's own sole-member household plus membership in other's household.
	solo, _ := households.CreateHousehold(ctx, "Solo", "S", u.ID)
	shared, _ := households.CreateHousehold(ctx, "Shared", "SH", other.ID)
	if err := households.AddMember(ctx, shared.ID, u.ID, household.RoleMember); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	if err := svc.DeleteAccount(ctx, u.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if _, err := households.GetHousehold(ctx, solo.ID); err == nil {
		t.Fatal("sole-member household should be deleted")
	}
	if _, err := households.GetHousehold(ctx, shared.ID); err != nil {
		t.Fatal("shared household should survive")
	}
	members, _ := households.GetMembers(ctx, shared.ID)
	if len(members) != 1 || members[0].UserID != other.ID {
		t.Fatalf("shared members = %+v, want just the other owner", members)
	}
}

func TestDeleteAccount_SessionsInvalidated(t *testing.T) {
	svc, users, _ := setup(t)
	ctx := context.Background()
	u := mustCreateUser(t, users, "sessions@test.com")
	if _, err := users.CreateSession(ctx, u.ID, u.AuthVersion, "hash1", tomorrow()); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := svc.DeleteAccount(ctx, u.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if _, err := users.GetSession(ctx, "hash1"); err == nil {
		t.Fatal("session survived account deletion")
	}
}
