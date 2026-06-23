package audit

import (
	"bytes"
	"context"
	"log"
	"testing"
)

func TestStdLoggerWritesJSONEvent(t *testing.T) {
	var output bytes.Buffer
	logger := NewStdLogger(log.New(&output, "", 0))
	logger.Log(context.Background(), "auth.login_succeeded", map[string]string{
		"method":  "password",
		"user_id": "user-123",
	})

	body := output.String()
	if body == "" {
		t.Fatal("expected audit log output")
	}
	if !bytes.Contains([]byte(body), []byte(`"event":"auth.login_succeeded"`)) {
		t.Fatalf("body = %s", body)
	}
	if !bytes.Contains([]byte(body), []byte(`"method":"password"`)) {
		t.Fatalf("body = %s", body)
	}
	if !bytes.Contains([]byte(body), []byte(`"user_id":"user-123"`)) {
		t.Fatalf("body = %s", body)
	}
}

func TestNopLoggerDoesNotPanic(t *testing.T) {
	NopLogger{}.Log(context.Background(), "noop", nil)
}

func TestActorContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	if _, ok := ActorFromContext(ctx); ok {
		t.Fatal("expected no actor in a plain context")
	}
	want := Actor{UserID: 42, HouseholdID: 7, Role: "owner"}
	ctx = WithActor(ctx, want)
	got, ok := ActorFromContext(ctx)
	if !ok {
		t.Fatal("expected actor in context after WithActor")
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestEmitNilLoggerIsNoop(t *testing.T) {
	// Should not panic when no logger is wired.
	Emit(context.Background(), nil, "anything", map[string]string{"x": "y"})
}

func TestEmitEnrichesActorFromContext(t *testing.T) {
	rec := NewRecorder()
	ctx := WithActor(context.Background(), Actor{UserID: 5, HouseholdID: 9, Role: "member"})

	Emit(ctx, rec, "chore.created", map[string]string{"chore_id": "3"})

	events := rec.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Event != "chore.created" {
		t.Errorf("Event = %q", ev.Event)
	}
	if ev.Attrs["chore_id"] != "3" {
		t.Errorf("chore_id missing/incorrect: %q", ev.Attrs["chore_id"])
	}
	if ev.Attrs["user_id"] != "5" {
		t.Errorf("user_id not enriched: %q", ev.Attrs["user_id"])
	}
	if ev.Attrs["household_id"] != "9" {
		t.Errorf("household_id not enriched: %q", ev.Attrs["household_id"])
	}
	if ev.Attrs["role"] != "member" {
		t.Errorf("role not enriched: %q", ev.Attrs["role"])
	}
}

func TestEmitDoesNotOverwriteExplicitAttrs(t *testing.T) {
	rec := NewRecorder()
	ctx := WithActor(context.Background(), Actor{UserID: 5, HouseholdID: 9, Role: "member"})

	// An explicit user_id (e.g. the actor acting on behalf of another member)
	// must win over the context actor.
	Emit(ctx, rec, "log.created", map[string]string{"user_id": "99", "household_id": "9"})

	ev, ok := rec.Find("log.created")
	if !ok {
		t.Fatal("expected log.created event")
	}
	if ev.Attrs["user_id"] != "99" {
		t.Errorf("explicit user_id overwritten: %q", ev.Attrs["user_id"])
	}
}

func TestEmitWithoutActorStillLogs(t *testing.T) {
	rec := NewRecorder()
	Emit(context.Background(), rec, "system.tick", map[string]string{"k": "v"})

	ev, ok := rec.Find("system.tick")
	if !ok {
		t.Fatal("expected event even with no actor")
	}
	if ev.Attrs["k"] != "v" {
		t.Errorf("attr lost: %q", ev.Attrs["k"])
	}
	if _, hasUser := ev.Attrs["user_id"]; hasUser {
		t.Error("did not expect user_id when no actor")
	}
}

func TestRecorderStoresAttrsSnapshot(t *testing.T) {
	rec := NewRecorder()
	attrs := map[string]string{"a": "1"}
	rec.Log(context.Background(), "e.one", attrs)

	// Mutate the caller's map after the call; the recorded copy must be unaffected.
	attrs["a"] = "mutated"

	ev, _ := rec.Find("e.one")
	if ev.Attrs["a"] != "1" {
		t.Errorf("recorder did not snapshot attrs: got %q", ev.Attrs["a"])
	}
}

func TestRecorderFindMissing(t *testing.T) {
	rec := NewRecorder()
	if _, ok := rec.Find("nope"); ok {
		t.Fatal("expected false for missing event")
	}
}

func TestRecorderReset(t *testing.T) {
	rec := NewRecorder()
	rec.Log(context.Background(), "e", nil)
	rec.Reset()
	if len(rec.Events()) != 0 {
		t.Fatalf("expected 0 events after reset, got %d", len(rec.Events()))
	}
}

func TestNewStdLoggerNilDefaultsToDefault(t *testing.T) {
	// Passing nil must not panic and must produce a working logger.
	l := NewStdLogger(nil)
	l.Log(context.Background(), "boot", nil)
}
