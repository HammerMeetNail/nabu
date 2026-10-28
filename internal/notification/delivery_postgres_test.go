package notification

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/push"
	"github.com/HammerMeetNail/nabu/internal/testdb"
	"github.com/HammerMeetNail/nabu/internal/testsync"
)

// householdDatabase holds Alice (1, owner) and Bob (2, member) in household 1,
// with chore 1.
func householdDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db := testdb.New(t)
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash,display_name) VALUES
			(1,'alice@example.invalid','','Alice'),(2,'bob@example.invalid','','Bob');
		INSERT INTO households(id,name,invite_code) VALUES (1,'Home','invite-1');
		INSERT INTO user_households(user_id,household_id,role) VALUES (1,1,'owner'),(2,1,'member');
		INSERT INTO chores(id,household_id,name,icon,visibility) VALUES (1,1,'Dishes','🍽️','household')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPostgresEventStoreLeasesRetriesAndExpires(t *testing.T) {
	db := householdDatabase(t)
	events := NewPostgresEventStore(db)
	ctx := context.Background()
	// Any clock: a new event is due at once.
	now := time.Date(2020, 1, 1, 9, 0, 0, 0, time.UTC)
	logged := Event{Kind: EventChoreLogged, HouseholdID: 1, ChoreID: 1, SubjectID: 1, ActorID: 1, ExpiresAt: now.Add(eventTTL)}
	joined := Event{Kind: EventHouseholdJoined, HouseholdID: 1, SubjectID: 2, ActorID: 2, Title: "New Member",
		Body: "Bob joined Home", ExpiresAt: now.Add(eventTTL)}
	for _, ev := range []Event{logged, joined} {
		if err := events.EnqueueEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	first, err := events.ClaimEvent(ctx, now, now.Add(eventLease), "lease-1")
	if err != nil || first.Kind != EventChoreLogged || first.ChoreID != 1 || first.SubjectID != 1 || first.ActorID != 1 || first.Attempts != 1 {
		t.Fatalf("first claim = %+v %v", first, err)
	}
	second, err := events.ClaimEvent(ctx, now, now.Add(eventLease), "lease-2")
	if err != nil || second.Kind != EventHouseholdJoined || second.ChoreID != 0 || second.Body != "Bob joined Home" {
		t.Fatalf("second claim = %+v %v", second, err)
	}
	if _, err := events.ClaimEvent(ctx, now, now.Add(eventLease), "lease-3"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("leased event claimed again: %v", err)
	}
	if err := events.FinishEvent(ctx, first, now.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := events.FinishEvent(ctx, second, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := events.ClaimEvent(ctx, now.Add(10*time.Second-time.Microsecond), now.Add(time.Minute), "early"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("claimed before its retry: %v", err)
	}
	retried, err := events.ClaimEvent(ctx, now.Add(10*time.Second), now.Add(time.Minute), "lease-4")
	if err != nil || retried.ID != first.ID || retried.Attempts != 2 {
		t.Fatalf("retry = %+v %v", retried, err)
	}
	// The lease ran out and another worker retook it; the first lease's
	// finish no longer counts.
	if err := events.FinishEvent(ctx, first, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := events.ClaimEvent(ctx, now.Add(eventTTL), now.Add(eventTTL+time.Minute), "expired"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expired event claimed: %v", err)
	}
	var left int
	if err := db.QueryRow(`SELECT count(*) FROM notification_events`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("events left = %d %v", left, err)
	}
}

func TestPostgresNotificationDedupeKeepsTheFirst(t *testing.T) {
	db := householdDatabase(t)
	store := NewPostgresStore(db)
	ctx := context.Background()
	first, err := store.CreateNotification(ctx, Notification{UserID: 2, Type: "chore_logged", Title: "t", DedupeKey: "event:1:2"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.CreateNotification(ctx, Notification{UserID: 2, Type: "chore_logged", Title: "t", DedupeKey: "event:1:2"})
	if err != nil || again.ID != first.ID {
		t.Fatalf("duplicate = %+v %v", again, err)
	}
	for range 2 {
		if _, err := store.CreateNotification(ctx, Notification{UserID: 2, Type: "chore_logged", Title: "keyless"}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := store.GetUnreadCount(ctx, 2); err != nil || n != 3 {
		t.Fatalf("notifications = %d %v", n, err)
	}
}

// heldChannel blocks its first send until released.
type heldChannel struct {
	mu      sync.Mutex
	sent    []string
	entered chan struct{}
	release chan struct{}
}

type deviceKey string

func (k deviceKey) Key() string { return string(k) }

func (c *heldChannel) Targets(context.Context, int64) ([]push.Target, error) {
	return []push.Target{deviceKey("first"), deviceKey("second")}, nil
}

func (c *heldChannel) Deliver(ctx context.Context, target push.Target, _ push.Message) push.Outcome {
	if target.Key() == "first" {
		close(c.entered)
		testsync.Pause(ctx, c.release)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, target.Key())
	return push.Delivered
}

// Each send holds its recipient's membership: removing them waits for the
// send in progress, and none starts after the removal commits.
func TestPostgresRemovalWaitsForOneSendAndStopsTheRest(t *testing.T) {
	db := householdDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	households := household.NewPostgresStore(db)
	svc := NewService(NewPostgresStore(db)).WithAuthorization(households, chore.NewPostgresStore(db))
	jobs := push.NewMemoryJobStore()
	if err := jobs.EnqueuePush(ctx, push.Job{UserID: 2, HouseholdID: 1, ChoreID: 1, Type: "chore_logged", Title: "🍽️ Dishes",
		ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	channel := &heldChannel{entered: make(chan struct{}), release: make(chan struct{})}
	delivered := make(chan struct{})
	go func() { push.NewDeliverer(jobs, svc.PushGuard(), channel).DeliverPending(ctx); close(delivered) }()
	testsync.Receive(t, ctx, channel.entered)

	var pid int
	if err := db.QueryRowContext(ctx, `SELECT pid FROM pg_stat_activity WHERE application_name=current_setting('application_name')
		AND state='idle in transaction' AND query LIKE '%FROM user_households%' LIMIT 1`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	removed := make(chan error, 1)
	go func() { removed <- households.RemoveMember(ctx, 1, 2) }()
	testdb.WaitBlocked(t, db, pid, "pg_advisory_xact_lock")
	close(channel.release)
	if err := testsync.Receive(t, ctx, removed); err != nil {
		t.Fatal(err)
	}
	testsync.Receive(t, ctx, delivered)
	if !slices.Equal(channel.sent, []string{"first"}) {
		t.Fatalf("sent to %v after the removal", channel.sent)
	}
	if pending := jobs.Pending(); len(pending) != 0 {
		t.Fatalf("job kept for a former member: %+v", pending)
	}
}
