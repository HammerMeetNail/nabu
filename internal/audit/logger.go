package audit

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"sync"
)

// Logger is the interface every audit sink implements. Implementations must be
// safe for concurrent use.
type Logger interface {
	Log(ctx context.Context, event string, attrs map[string]string)
}

// NopLogger discards every event. It is the zero-value default used by services
// that have not been wired with a real sink.
type NopLogger struct{}

func (NopLogger) Log(context.Context, string, map[string]string) {}

// StdLogger writes each event as a single JSON object to an *log.Logger.
type StdLogger struct {
	logger *log.Logger
}

func NewStdLogger(logger *log.Logger) StdLogger {
	if logger == nil {
		logger = log.Default()
	}
	return StdLogger{logger: logger}
}

func (l StdLogger) Log(_ context.Context, event string, attrs map[string]string) {
	payload := map[string]any{"event": event}
	for key, value := range attrs {
		payload[key] = value
	}
	encoded, _ := json.Marshal(payload)
	l.logger.Println(string(encoded))
}

// Actor describes the authenticated principal that performed an action. It is
// carried through request context by the session middleware so that service-
// layer audit calls can attribute events to a user without each service method
// taking an explicit actor parameter (and without any service depending on the
// HTTP middleware package).
type Actor struct {
	UserID      int64
	HouseholdID int64 // 0 when the user has no active household
	Role        string
}

type actorKey struct{}

// WithActor returns a copy of ctx that carries the given audit Actor. The
// session middleware calls this so downstream services can recover the actor
// via ActorFromContext.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// ActorFromContext returns the audit Actor stashed in ctx, if any.
func ActorFromContext(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}

// Emit writes an audit event via logger, first enriching attrs with the actor
// (user_id, household_id, role) pulled from ctx for any field not already
// present. A nil logger is a no-op. Callers should pass an attrs map that is
// safe to mutate (Emit clones it before enriching).
//
// This is the single place actor enrichment happens, so every service/handler
// audit call records who did it and in which household without duplicating the
// context-lookup logic.
func Emit(ctx context.Context, logger Logger, event string, attrs map[string]string) {
	if logger == nil {
		return
	}
	out := make(map[string]string, len(attrs)+3)
	for k, v := range attrs {
		out[k] = v
	}
	if a, ok := ActorFromContext(ctx); ok {
		if _, exists := out["user_id"]; !exists && a.UserID != 0 {
			out["user_id"] = strconv.FormatInt(a.UserID, 10)
		}
		if _, exists := out["household_id"]; !exists && a.HouseholdID != 0 {
			out["household_id"] = strconv.FormatInt(a.HouseholdID, 10)
		}
		if _, exists := out["role"]; !exists && a.Role != "" {
			out["role"] = a.Role
		}
	}
	logger.Log(ctx, event, out)
}

// RecordedEvent is a single audit event captured by a Recorder.
type RecordedEvent struct {
	Event string
	Attrs map[string]string
}

// Recorder is an in-memory audit.Logger intended for tests. It is safe for
// concurrent use. Services under test can take a Recorder via SetAuditLogger
// and tests assert on Events().
type Recorder struct {
	mu     sync.Mutex
	events []RecordedEvent
}

// NewRecorder returns a fresh Recorder.
func NewRecorder() *Recorder {
	return &Recorder{}
}

func (r *Recorder) Log(_ context.Context, event string, attrs map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := make(map[string]string, len(attrs))
	for k, v := range attrs {
		copy[k] = v
	}
	r.events = append(r.events, RecordedEvent{Event: event, Attrs: copy})
}

// Events returns a snapshot of all recorded audit events in emission order.
func (r *Recorder) Events() []RecordedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RecordedEvent, len(r.events))
	copy(out, r.events)
	return out
}

// Reset clears all recorded events.
func (r *Recorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
}

// Find returns the first recorded event matching the given event name, or false.
func (r *Recorder) Find(event string) (RecordedEvent, bool) {
	for _, e := range r.Events() {
		if e.Event == event {
			return e, true
		}
	}
	return RecordedEvent{}, false
}
