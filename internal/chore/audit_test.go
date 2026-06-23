package chore_test

import (
	"context"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/chore"
)

// newSvcWithAudit builds a chore service wired with an in-memory audit recorder,
// and returns a context carrying the actor so service-layer audit (which reads
// the actor from context, like the session middleware does in production) can
// attribute events to a user.
func newSvcWithAudit() (*chore.Service, *audit.Recorder, context.Context) {
	svc := chore.NewService(chore.NewMemoryStore())
	rec := audit.NewRecorder()
	svc.SetAuditLogger(rec)
	ctx := audit.WithActor(context.Background(), audit.Actor{UserID: 10, HouseholdID: 1, Role: "member"})
	return svc, rec, ctx
}

func TestAudit_ChoreCreated(t *testing.T) {
	svc, rec, ctx := newSvcWithAudit()

	c, err := svc.CreateChore(ctx, 1, 10, "Wash Dishes", "🍽️", "#3B82F6", "cleaning", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateChore: %v", err)
	}
	ev, ok := rec.Find("chore.created")
	if !ok {
		t.Fatalf("missing chore.created; events=%#v", rec.Events())
	}
	if ev.Attrs["chore_id"] != id64(c.ID) {
		t.Errorf("chore_id = %q, want %s", ev.Attrs["chore_id"], id64(c.ID))
	}
	if ev.Attrs["household_id"] != "1" {
		t.Errorf("household_id = %q, want 1", ev.Attrs["household_id"])
	}
	if ev.Attrs["name"] != "Wash Dishes" {
		t.Errorf("name = %q", ev.Attrs["name"])
	}
	// Actor must be enriched from context — this is who did it, even though the
	// service method does not take an actor parameter.
	if ev.Attrs["user_id"] != "10" {
		t.Errorf("user_id (actor) = %q, want 10", ev.Attrs["user_id"])
	}
}

func TestAudit_ChoreCreated_EmptyNameNotAudited(t *testing.T) {
	svc, rec, ctx := newSvcWithAudit()
	if _, err := svc.CreateChore(ctx, 1, 10, "", "", "", "", nil, nil, nil); err == nil {
		t.Fatal("expected error for empty name")
	}
	if ev, ok := rec.Find("chore.created"); ok {
		t.Fatalf("expected no event on failure; got %#v", ev)
	}
}

func TestAudit_ChoreUpdated(t *testing.T) {
	svc, rec, ctx := newSvcWithAudit()
	c, _ := svc.CreateChore(ctx, 1, 10, "Old", "🐱", "", "", nil, nil, nil)

	if err := svc.UpdateChore(ctx, c.ID, 1, "New", "", "", "", nil, nil, nil); err != nil {
		t.Fatalf("UpdateChore: %v", err)
	}
	ev, ok := rec.Find("chore.updated")
	if !ok {
		t.Fatalf("missing chore.updated; events=%#v", rec.Events())
	}
	if ev.Attrs["chore_id"] != id64(c.ID) {
		t.Errorf("chore_id = %q", ev.Attrs["chore_id"])
	}
	if ev.Attrs["household_id"] != "1" {
		t.Errorf("household_id = %q", ev.Attrs["household_id"])
	}
	if ev.Attrs["user_id"] != "10" {
		t.Errorf("user_id (actor) = %q, want 10", ev.Attrs["user_id"])
	}
}

func TestAudit_ChoreUpdated_CrossHouseholdNotAudited(t *testing.T) {
	svc, rec, ctx := newSvcWithAudit()
	c, _ := svc.CreateChore(ctx, 1, 10, "Mine", "", "", "", nil, nil, nil)

	// Attempt to update chore 1 from household 2 — must fail and not be audited.
	if err := svc.UpdateChore(ctx, c.ID, 2, "Hacked", "", "", "", nil, nil, nil); err == nil {
		t.Fatal("expected cross-household update to fail")
	}
	if ev, ok := rec.Find("chore.updated"); ok {
		t.Fatalf("unexpected event for cross-household update: %#v", ev)
	}
}

func TestAudit_ChoreDeleted(t *testing.T) {
	svc, rec, ctx := newSvcWithAudit()
	c, _ := svc.CreateChore(ctx, 1, 10, "Temp", "", "", "", nil, nil, nil)

	if err := svc.DeleteChore(ctx, c.ID, 1); err != nil {
		t.Fatalf("DeleteChore: %v", err)
	}
	ev, ok := rec.Find("chore.deleted")
	if !ok {
		t.Fatalf("missing chore.deleted; events=%#v", rec.Events())
	}
	if ev.Attrs["chore_id"] != id64(c.ID) {
		t.Errorf("chore_id = %q", ev.Attrs["chore_id"])
	}
	if ev.Attrs["user_id"] != "10" {
		t.Errorf("user_id (actor) = %q, want 10", ev.Attrs["user_id"])
	}
}

func TestAudit_ChoreDeleted_PredefinedNotAudited(t *testing.T) {
	svc, rec, ctx := newSvcWithAudit()
	_ = svc.SeedDefaultChores(ctx, 1)
	chores, _ := svc.ListChores(ctx, 1)
	var predefID int64
	for _, c := range chores {
		if c.IsPredefined {
			predefID = c.ID
			break
		}
	}
	if predefID == 0 {
		t.Fatal("expected a predefined chore after seed")
	}
	if err := svc.DeleteChore(ctx, predefID, 1); err == nil {
		t.Fatal("expected error deleting predefined chore")
	}
	if ev, ok := rec.Find("chore.deleted"); ok {
		t.Fatalf("unexpected event for predefined delete: %#v", ev)
	}
}

func TestAudit_ChoreReordered(t *testing.T) {
	svc, rec, ctx := newSvcWithAudit()
	c1, _ := svc.CreateChore(ctx, 1, 10, "A", "", "", "", nil, nil, nil)
	c2, _ := svc.CreateChore(ctx, 1, 10, "B", "", "", "", nil, nil, nil)

	if err := svc.ReorderChores(ctx, 1, []int64{c2.ID, c1.ID}); err != nil {
		t.Fatalf("ReorderChores: %v", err)
	}
	ev, ok := rec.Find("chore.reordered")
	if !ok {
		t.Fatalf("missing chore.reordered; events=%#v", rec.Events())
	}
	if ev.Attrs["chore_count"] != "2" {
		t.Errorf("chore_count = %q, want 2", ev.Attrs["chore_count"])
	}
	if ev.Attrs["user_id"] != "10" {
		t.Errorf("user_id (actor) = %q, want 10", ev.Attrs["user_id"])
	}
}

func TestAudit_ChoreDefaultRestored(t *testing.T) {
	svc, rec, ctx := newSvcWithAudit()
	_ = svc.SeedDefaultChores(ctx, 1)
	chores, _ := svc.ListChores(ctx, 1)
	var targetID int64
	for _, c := range chores {
		if c.IsPredefined {
			targetID = c.ID
			break
		}
	}
	_ = svc.UpdateChore(ctx, targetID, 1, "Modified", "", "", "", nil, nil, nil)
	rec.Reset()

	if err := svc.RestoreDefaultChore(ctx, targetID, 1); err != nil {
		t.Fatalf("RestoreDefaultChore: %v", err)
	}
	ev, ok := rec.Find("chore.default_restored")
	if !ok {
		t.Fatalf("missing chore.default_restored; events=%#v", rec.Events())
	}
	if ev.Attrs["chore_id"] != id64(targetID) {
		t.Errorf("chore_id = %q", ev.Attrs["chore_id"])
	}
	if ev.Attrs["user_id"] != "10" {
		t.Errorf("user_id (actor) = %q, want 10", ev.Attrs["user_id"])
	}
}

func TestAudit_ChoreDefaultsSeeded(t *testing.T) {
	svc, rec, ctx := newSvcWithAudit()
	if err := svc.SeedDefaultChores(ctx, 1); err != nil {
		t.Fatalf("SeedDefaultChores: %v", err)
	}
	ev, ok := rec.Find("chore.defaults_seeded")
	if !ok {
		t.Fatalf("missing chore.defaults_seeded; events=%#v", rec.Events())
	}
	if ev.Attrs["household_id"] != "1" {
		t.Errorf("household_id = %q", ev.Attrs["household_id"])
	}
	if ev.Attrs["user_id"] != "10" {
		t.Errorf("user_id (actor) = %q, want 10", ev.Attrs["user_id"])
	}
}

// No-actor context must not panic and must simply omit the actor fields.
func TestAudit_Chore_NoActorDoesNotPanic(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	rec := audit.NewRecorder()
	svc.SetAuditLogger(rec)
	// Plain context (no middleware): should still emit without user_id.
	if _, err := svc.CreateChore(context.Background(), 1, 10, "X", "", "", "", nil, nil, nil); err != nil {
		t.Fatalf("CreateChore: %v", err)
	}
	ev, ok := rec.Find("chore.created")
	if !ok {
		t.Fatalf("expected event even without actor")
	}
	if _, has := ev.Attrs["user_id"]; has {
		t.Errorf("did not expect user_id without actor")
	}
}

func id64(id int64) string {
	if id == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for id > 0 {
		pos--
		buf[pos] = byte('0' + id%10)
		id /= 10
	}
	return string(buf[pos:])
}
