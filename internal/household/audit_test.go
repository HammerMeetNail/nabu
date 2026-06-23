package household_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/household"
)

// newSvcWithAudit builds a service wired with an in-memory audit recorder so
// tests can assert which household events were emitted and with what attrs.
func newSvcWithAudit() (*household.Service, *household.MemoryStore, *stubAuthStore, *audit.Recorder) {
	store := household.NewMemoryStore()
	auth := &stubAuthStore{}
	svc := household.NewService(store, auth)
	rec := audit.NewRecorder()
	svc.SetAuditLogger(rec)
	return svc, store, auth, rec
}

func assertEvent(t *testing.T, rec *audit.Recorder, want string, wantAttrs map[string]string) {
	t.Helper()
	ev, ok := rec.Find(want)
	if !ok {
		t.Fatalf("missing audit event %q; recorded events = %#v", want, rec.Events())
	}
	for k, v := range wantAttrs {
		if got := ev.Attrs[k]; got != v {
			t.Errorf("event %q attr %q = %q, want %q", want, k, got, v)
		}
	}
}

func TestAudit_HouseholdCreated(t *testing.T) {
	svc, _, _, rec := newSvcWithAudit()
	ctx := context.Background()

	hh, err := svc.CreateHousehold(ctx, "Smith", "", 1)
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	assertEvent(t, rec, "household.created", map[string]string{
		"user_id":      "1",
		"household_id": idStr(hh.ID),
		"name":         "Smith",
	})
}

func TestAudit_HouseholdUpdated(t *testing.T) {
	svc, _, _, rec := newSvcWithAudit()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Old", "", 1)
	if err := svc.UpdateHousehold(ctx, 1, "New", ""); err != nil {
		t.Fatalf("UpdateHousehold: %v", err)
	}
	assertEvent(t, rec, "household.updated", map[string]string{
		"user_id":      "1",
		"household_id": idStr(hh.ID),
		"name":         "New",
	})
}

func TestAudit_HouseholdUpdated_UnauthorizedNotAudited(t *testing.T) {
	svc, store, _, rec := newSvcWithAudit()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	// Member (non-owner/admin) must not be allowed to update, and no event emitted.
	if err := svc.UpdateHousehold(ctx, 2, "Hacked", ""); err == nil {
		t.Fatal("expected authorization error")
	}
	if ev, ok := rec.Find("household.updated"); ok {
		t.Fatalf("unexpected audit event for unauthorized update: %#v", ev)
	}
}

func TestAudit_InviteCreatedAndDeleted(t *testing.T) {
	svc, _, _, rec := newSvcWithAudit()
	ctx := context.Background()

	if _, err := svc.CreateHousehold(ctx, "Test", "", 1); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	inv, err := svc.CreateInvite(ctx, 1)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	assertEvent(t, rec, "household.invite_created", map[string]string{
		"user_id":   "1",
		"invite_id": idStr(inv.ID),
	})

	if err := svc.DeleteInvite(ctx, 1, inv.ID); err != nil {
		t.Fatalf("DeleteInvite: %v", err)
	}
	assertEvent(t, rec, "household.invite_deleted", map[string]string{
		"user_id":   "1",
		"invite_id": idStr(inv.ID),
	})
}

func TestAudit_InviteCreated_NotOwnerNotAudited(t *testing.T) {
	svc, store, _, rec := newSvcWithAudit()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	if _, err := svc.CreateInvite(ctx, 2); err == nil {
		t.Fatal("expected non-owner to be rejected")
	}
	if ev, ok := rec.Find("household.invite_created"); ok {
		t.Fatalf("unexpected audit event: %#v", ev)
	}
}

func TestAudit_MemberJoined_InviteCode(t *testing.T) {
	svc, _, _, rec := newSvcWithAudit()
	ctx := context.Background()

	if _, err := svc.CreateHousehold(ctx, "Test", "", 1); err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	inv, _ := svc.CreateInvite(ctx, 1)

	if _, err := svc.JoinHousehold(ctx, 2, inv.Code); err != nil {
		t.Fatalf("JoinHousehold: %v", err)
	}
	assertEvent(t, rec, "household.member_joined", map[string]string{
		"user_id":       "2",
		"invite_method": "invite_code",
	})
}

func TestAudit_MemberJoined_PermanentCode(t *testing.T) {
	svc, _, _, rec := newSvcWithAudit()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)

	if _, err := svc.JoinHousehold(ctx, 2, hh.InviteCode); err != nil {
		t.Fatalf("JoinHousehold permanent: %v", err)
	}
	assertEvent(t, rec, "household.member_joined", map[string]string{
		"user_id":       "2",
		"invite_method": "permanent_code",
	})
}

func TestAudit_MemberRoleChanged(t *testing.T) {
	svc, store, _, rec := newSvcWithAudit()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	if err := svc.UpdateMemberRole(ctx, 1, 2, household.RoleAdmin); err != nil {
		t.Fatalf("UpdateMemberRole: %v", err)
	}
	assertEvent(t, rec, "household.member_role_changed", map[string]string{
		"user_id":        "1",
		"target_user_id": "2",
		"household_id":   idStr(hh.ID),
		"new_role":       household.RoleAdmin,
	})
}

func TestAudit_MemberRemoved(t *testing.T) {
	svc, store, _, rec := newSvcWithAudit()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	if err := svc.RemoveMember(ctx, 1, 2); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	assertEvent(t, rec, "household.member_removed", map[string]string{
		"user_id":        "1",
		"target_user_id": "2",
		"household_id":   idStr(hh.ID),
	})
}

func TestAudit_MemberRemoved_CrossHouseholdNotAudited(t *testing.T) {
	svc, store, _, rec := newSvcWithAudit()
	ctx := context.Background()

	// Two separate households.
	hh1, _ := svc.CreateHousehold(ctx, "One", "", 1)
	_ = store.AddMember(ctx, hh1.ID, 2, household.RoleMember)
	hh2, _ := svc.CreateHousehold(ctx, "Two", "", 3)
	_ = store.AddMember(ctx, hh2.ID, 4, household.RoleMember)

	// Owner of hh1 attempts to remove a member of hh2 — must be rejected with no event.
	if err := svc.RemoveMember(ctx, 1, 4); err == nil {
		t.Fatal("expected cross-household removal to be rejected")
	}
	if ev, ok := rec.Find("household.member_removed"); ok {
		t.Fatalf("unexpected audit event for cross-household removal: %#v", ev)
	}
}

func TestAudit_MemberLeft_Member(t *testing.T) {
	svc, store, _, rec := newSvcWithAudit()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	if err := svc.LeaveHousehold(ctx, 2); err != nil {
		t.Fatalf("LeaveHousehold: %v", err)
	}
	assertEvent(t, rec, "household.member_left", map[string]string{
		"user_id":      "2",
		"household_id": idStr(hh.ID),
	})
}

func TestAudit_MemberLeft_OwnerWithCoOwner(t *testing.T) {
	svc, store, _, rec := newSvcWithAudit()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	// A second owner so the first may leave.
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleOwner)

	if err := svc.LeaveHousehold(ctx, 1); err != nil {
		t.Fatalf("LeaveHousehold: %v", err)
	}
	assertEvent(t, rec, "household.member_left", map[string]string{
		"user_id":      "1",
		"household_id": idStr(hh.ID),
	})
}

func TestAudit_OwnershipTransferred(t *testing.T) {
	svc, store, _, rec := newSvcWithAudit()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	if err := svc.TransferOwnership(ctx, 1, 2); err != nil {
		t.Fatalf("TransferOwnership: %v", err)
	}
	assertEvent(t, rec, "household.ownership_transferred", map[string]string{
		"user_id":        "1",
		"target_user_id": "2",
		"household_id":   idStr(hh.ID),
	})
}

func TestAudit_OwnershipTransferred_NotOwnerNotAudited(t *testing.T) {
	svc, store, _, rec := newSvcWithAudit()
	ctx := context.Background()

	hh, _ := svc.CreateHousehold(ctx, "Test", "", 1)
	_ = store.AddMember(ctx, hh.ID, 2, household.RoleMember)

	if err := svc.TransferOwnership(ctx, 2, 1); err == nil {
		t.Fatal("expected non-owner transfer to fail")
	}
	if ev, ok := rec.Find("household.ownership_transferred"); ok {
		t.Fatalf("unexpected audit event: %#v", ev)
	}
}

// idStr formats an int64 the same way the service does in audit attrs.
func idStr(id int64) string {
	return strconv.FormatInt(id, 10)
}
