package notification

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/HammerMeetNail/nabu/internal/lifecycle"
)

const (
	EventChoreLogged     = "chore_logged"
	EventHouseholdJoined = "household_joined"

	// A notification later than this is noise.
	eventTTL = 15 * time.Minute
	// A fan-out writes a few rows per member.
	eventLease = time.Minute
)

// Event is a log or a join waiting to fan out to the household: each other
// member gets an in-app notification and, if they allow it, a push job.
type Event struct {
	ID          int64
	Kind        string
	HouseholdID int64
	ChoreID     int64
	// SubjectID is the member a log is for, or who joined; ActorID did it.
	// Neither is notified.
	SubjectID int64
	ActorID   int64
	// Title and Body are a join's message, written when it happened. A log's
	// is written when it fans out.
	Title     string
	Body      string
	Attempts  int
	LeaseID   string
	ExpiresAt time.Time
}

type EventStore interface {
	EnqueueEvent(ctx context.Context, ev Event) error
	// ClaimEvent leases the next due event, or returns sql.ErrNoRows.
	ClaimEvent(ctx context.Context, now, leaseUntil time.Time, leaseID string) (Event, error)
	// FinishEvent deletes the event, or with a non-zero retryAt releases it
	// to be tried again then.
	FinishEvent(ctx context.Context, ev Event, retryAt time.Time) error
}

type PostgresEventStore struct {
	db *sql.DB
}

func NewPostgresEventStore(db *sql.DB) *PostgresEventStore {
	return &PostgresEventStore{db: db}
}

func (s *PostgresEventStore) EnqueueEvent(ctx context.Context, ev Event) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO notification_events
		(kind, household_id, chore_id, subject_id, actor_id, title, body, expires_at)
		VALUES ($1, $2, NULLIF($3, 0), NULLIF($4, 0), NULLIF($5, 0), $6, $7, $8)`,
		ev.Kind, ev.HouseholdID, ev.ChoreID, ev.SubjectID, ev.ActorID, ev.Title, ev.Body, ev.ExpiresAt)
	return err
}

func (s *PostgresEventStore) ClaimEvent(ctx context.Context, now, leaseUntil time.Time, leaseID string) (Event, error) {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM notification_events WHERE expires_at <= $1`, now); err != nil {
		return Event{}, err
	}
	var ev Event
	err := s.db.QueryRowContext(ctx, `UPDATE notification_events SET lease_until = $2, lease_id = $3, attempts = attempts + 1
		WHERE id = (SELECT id FROM notification_events WHERE next_attempt <= $1 AND expires_at > $1
		AND (lease_until IS NULL OR lease_until <= $1) ORDER BY next_attempt, id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id, kind, household_id, COALESCE(chore_id, 0), COALESCE(subject_id, 0), COALESCE(actor_id, 0),
		title, body, attempts, lease_id, expires_at`, now, leaseUntil, leaseID).Scan(&ev.ID, &ev.Kind, &ev.HouseholdID,
		&ev.ChoreID, &ev.SubjectID, &ev.ActorID, &ev.Title, &ev.Body, &ev.Attempts, &ev.LeaseID, &ev.ExpiresAt)
	return ev, err
}

func (s *PostgresEventStore) FinishEvent(ctx context.Context, ev Event, retryAt time.Time) error {
	if retryAt.IsZero() {
		_, err := s.db.ExecContext(ctx, `DELETE FROM notification_events WHERE id = $1 AND lease_id = $2`, ev.ID, ev.LeaseID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE notification_events SET lease_until = NULL, lease_id = '', next_attempt = $3
		WHERE id = $1 AND lease_id = $2`, ev.ID, ev.LeaseID, retryAt)
	return err
}

// MemoryEventStore backs tests and the zero-DB development mode. The fan-out
// worker claims events while HTTP handlers add them, so every method takes mu.
type MemoryEventStore struct {
	mu      sync.Mutex
	deleted lifecycle.Tombstones
	seq     int64
	events  []memoryEvent
}

type memoryEvent struct {
	ev          Event
	nextAttempt time.Time
	leaseUntil  time.Time
}

func NewMemoryEventStore() *MemoryEventStore {
	return &MemoryEventStore{}
}

func (s *MemoryEventStore) EnqueueEvent(_ context.Context, ev Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.deleted.Check(ev.SubjectID, ev.HouseholdID, ev.ChoreID, 0, 0); err != nil {
		return err
	}
	if err := s.deleted.Check(ev.ActorID, 0, 0, 0, 0); err != nil {
		return err
	}
	s.seq++
	ev.ID, ev.Attempts, ev.LeaseID = s.seq, 0, ""
	s.events = append(s.events, memoryEvent{ev: ev})
	return nil
}

func (s *MemoryEventStore) ClaimEvent(_ context.Context, now, leaseUntil time.Time, leaseID string) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.events[:0]
	for _, queued := range s.events {
		if queued.ev.ExpiresAt.After(now) {
			kept = append(kept, queued)
		}
	}
	s.events = kept
	for i := range s.events {
		queued := &s.events[i]
		if queued.nextAttempt.After(now) || queued.leaseUntil.After(now) {
			continue
		}
		queued.leaseUntil = leaseUntil
		queued.ev.LeaseID = leaseID
		queued.ev.Attempts++
		return queued.ev, nil
	}
	return Event{}, sql.ErrNoRows
}

func (s *MemoryEventStore) FinishEvent(_ context.Context, ev Event, retryAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.events {
		queued := &s.events[i]
		if queued.ev.ID != ev.ID || queued.ev.LeaseID != ev.LeaseID {
			continue
		}
		if retryAt.IsZero() {
			s.events = append(s.events[:i], s.events[i+1:]...)
			return nil
		}
		queued.ev.LeaseID = ""
		queued.leaseUntil = time.Time{}
		queued.nextAttempt = retryAt
		return nil
	}
	// The lease expired and another worker took the event, as in PostgreSQL.
	return nil
}

// Pending returns the events not yet fanned out, for tests.
func (s *MemoryEventStore) Pending() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Event, 0, len(s.events))
	for _, queued := range s.events {
		out = append(out, queued.ev)
	}
	return out
}

func (s *MemoryEventStore) CleanupAccount(d *lifecycle.Deletion) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.events[:0]
	for _, queued := range s.events {
		ev := queued.ev
		if ev.SubjectID != d.UserID && ev.ActorID != d.UserID && !d.Households[ev.HouseholdID] && !d.Chores[ev.ChoreID] {
			kept = append(kept, queued)
		}
	}
	s.events = kept
	s.deleted.Mark(d)
}
