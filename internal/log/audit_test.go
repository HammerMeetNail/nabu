package log_test

import (
	"context"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/audit"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
)

// newSvcWithAudit builds a log service wired with an audit recorder and a
// context carrying the actor (the authenticated user performing the action).
func newSvcWithAudit() (*chorelog.Service, *audit.Recorder, context.Context) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	rec := audit.NewRecorder()
	svc.SetAuditLogger(rec)
	ctx := audit.WithActor(context.Background(), audit.Actor{UserID: 7, HouseholdID: 5, Role: "member"})
	return svc, rec, ctx
}

func TestAudit_LogCreated(t *testing.T) {
	svc, rec, ctx := newSvcWithAudit()

	// User 7 logs chore 100 attributed to user 9 (logging on behalf of another
	// household member). The audit actor must be user 7 (who did it).
	l, err := svc.LogChore(ctx, 5, 9, 100, nil, "done", nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("LogChore: %v", err)
	}
	ev, ok := rec.Find("log.created")
	if !ok {
		t.Fatalf("missing log.created; events=%#v", rec.Events())
	}
	if ev.Attrs["household_id"] != "5" {
		t.Errorf("household_id = %q, want 5", ev.Attrs["household_id"])
	}
	if ev.Attrs["chore_id"] != "100" {
		t.Errorf("chore_id = %q, want 100", ev.Attrs["chore_id"])
	}
	if ev.Attrs["log_id"] != id64(l.ID) {
		t.Errorf("log_id = %q, want %s", ev.Attrs["log_id"], id64(l.ID))
	}
	if ev.Attrs["user_id"] != "7" {
		t.Errorf("user_id (actor) = %q, want 7 (the logger, not the attributed user)", ev.Attrs["user_id"])
	}
}

func TestAudit_LogUpdated(t *testing.T) {
	svc, rec, ctx := newSvcWithAudit()

	l, _ := svc.LogChore(ctx, 5, 9, 100, nil, "first", nil, nil, nil, nil, nil, nil, nil)
	rec.Reset()

	if err := svc.UpdateLog(ctx, l.ID, 5, nil, "edited", nil, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("UpdateLog: %v", err)
	}
	ev, ok := rec.Find("log.updated")
	if !ok {
		t.Fatalf("missing log.updated; events=%#v", rec.Events())
	}
	if ev.Attrs["log_id"] != id64(l.ID) {
		t.Errorf("log_id = %q", ev.Attrs["log_id"])
	}
	if ev.Attrs["household_id"] != "5" {
		t.Errorf("household_id = %q", ev.Attrs["household_id"])
	}
	if ev.Attrs["user_id"] != "7" {
		t.Errorf("user_id (actor) = %q, want 7", ev.Attrs["user_id"])
	}
}

func TestAudit_LogUpdated_CrossHouseholdNotAudited(t *testing.T) {
	svc, rec, ctx := newSvcWithAudit()

	l, _ := svc.LogChore(ctx, 5, 9, 100, nil, "x", nil, nil, nil, nil, nil, nil, nil)
	// Try to update from a different household — must fail and not be audited.
	if err := svc.UpdateLog(ctx, l.ID, 99, nil, "hacked", nil, nil, nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("expected cross-household update to fail")
	}
	if ev, ok := rec.Find("log.updated"); ok {
		t.Fatalf("unexpected event for cross-household update: %#v", ev)
	}
}

func TestAudit_LogDeleted(t *testing.T) {
	svc, rec, ctx := newSvcWithAudit()

	l, _ := svc.LogChore(ctx, 5, 9, 100, nil, "x", nil, nil, nil, nil, nil, nil, nil)
	rec.Reset()

	if err := svc.UndoLog(ctx, 5, l.ID); err != nil {
		t.Fatalf("UndoLog: %v", err)
	}
	ev, ok := rec.Find("log.deleted")
	if !ok {
		t.Fatalf("missing log.deleted; events=%#v", rec.Events())
	}
	if ev.Attrs["log_id"] != id64(l.ID) {
		t.Errorf("log_id = %q", ev.Attrs["log_id"])
	}
	if ev.Attrs["user_id"] != "7" {
		t.Errorf("user_id (actor) = %q, want 7", ev.Attrs["user_id"])
	}
}

func TestAudit_LogDeleted_CrossHouseholdNotAudited(t *testing.T) {
	svc, rec, ctx := newSvcWithAudit()

	l, _ := svc.LogChore(ctx, 5, 9, 100, nil, "x", nil, nil, nil, nil, nil, nil, nil)
	if err := svc.UndoLog(ctx, 99, l.ID); err == nil {
		t.Fatal("expected cross-household undo to fail")
	}
	if ev, ok := rec.Find("log.deleted"); ok {
		t.Fatalf("unexpected event for cross-household delete: %#v", ev)
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
