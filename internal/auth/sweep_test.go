package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func TestMemorySweepEnded(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	u, err := store.CreateUser(ctx, "sweep@example.invalid", "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for hash, expires := range map[string]time.Time{"live": now.Add(time.Hour), "lapsed": now.Add(-time.Second), "idle": now.Add(time.Hour), "away": now.Add(time.Hour)} {
		if _, err := store.CreateSession(ctx, u.ID, u.AuthVersion, hash, expires, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.TouchSession(ctx, "idle", now.Add(-lifecycle.SessionIdleTimeout-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.TouchSession(ctx, "away", now.Add(-29*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for hash, expires := range map[string]time.Time{"live-token": now.Add(time.Hour), "used-token": now.Add(time.Hour), "expired-token": now.Add(-time.Second)} {
		if _, err := store.CreateAuthToken(ctx, &u.ID, u.Email, hash, "magic", expires); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ConsumeAuthToken(ctx, "used-token", "magic"); err != nil {
		t.Fatal(err)
	}

	var sessions, tokens int64
	for range 2 {
		n, m, err := store.SweepEnded(ctx, 1)
		if err != nil || n != 1 || m != 1 {
			t.Fatalf("batch of one = %d sessions, %d tokens, %v; want 1, 1", n, m, err)
		}
		sessions, tokens = sessions+n, tokens+m
	}
	if n, m, err := store.SweepEnded(ctx, 1); err != nil || n != 0 || m != 0 {
		t.Fatalf("after everything ended was swept: %d sessions, %d tokens, %v", n, m, err)
	}
	for _, hash := range []string{"live", "away"} {
		if _, err := store.GetSession(ctx, hash); err != nil {
			t.Fatalf("live session %q swept: %v", hash, err)
		}
	}
	for _, hash := range []string{"lapsed", "idle"} {
		if _, err := store.GetSession(ctx, hash); err == nil {
			t.Fatalf("ended session %q kept", hash)
		}
	}
	if _, err := store.GetAuthToken(ctx, "live-token", "magic"); err != nil {
		t.Fatalf("live token swept: %v", err)
	}
	if len(store.tokensByHash) != 1 {
		t.Fatalf("tokens left = %d, want 1", len(store.tokensByHash))
	}
}

func postgresSweepStore(t *testing.T) (*PostgresStore, *sql.DB) {
	t.Helper()
	db := testdb.New(t)
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return NewPostgresStore(db), db
}

// Fixture times are set from PostgreSQL's clock, which the sweep compares
// against, never from this process's clock.
func TestPostgresSweepEnded(t *testing.T) {
	store, db := postgresSweepStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	u, err := store.CreateUser(ctx, "sweep@example.invalid", "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	for _, hash := range []string{"live", "lapsed", "idle", "away"} {
		if _, err := store.CreateSession(ctx, u.ID, u.AuthVersion, hash, time.Now().Add(time.Hour), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	for _, hash := range []string{"live-token", "used-token", "expired-token"} {
		if _, err := store.CreateAuthToken(ctx, &u.ID, u.Email, hash, "magic", time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ConsumeAuthToken(ctx, "used-token", "magic"); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`UPDATE sessions SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE token_hash='lapsed'`,
		`UPDATE sessions SET last_seen_at=clock_timestamp()-` + lifecycle.SessionIdleSQL + `-INTERVAL '1 minute' WHERE token_hash='idle'`,
		`UPDATE sessions SET last_seen_at=clock_timestamp()-INTERVAL '29 days' WHERE token_hash='away'`,
		`UPDATE auth_tokens SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE token_hash='expired-token'`,
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	// The idle session's devices: their registrations end with it.
	if _, err := db.ExecContext(ctx, `INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth, session_hash, binding_id, registration_id)
 VALUES ($1, 'https://push.example.invalid/idle', 'key', 'key', 'idle', 'binding', 'registration')`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO mobile_device_tokens (user_id, token, environment, bundle_id, session_hash, registration_id)
 VALUES ($1, 'idle-device', 'sandbox', 'com.nabu.test', 'idle', 'registration')`, u.ID); err != nil {
		t.Fatal(err)
	}

	// Another instance holds the sweep: this one deletes nothing.
	other, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, sweepLockKey); err != nil {
		t.Fatal(err)
	}
	if n, m, err := store.SweepEnded(ctx, 10); err != nil || n != 0 || m != 0 {
		t.Fatalf("second instance swept %d sessions, %d tokens, %v", n, m, err)
	}
	_ = other.Rollback()

	// A transaction is using the lapsed session, as a push send would: the
	// sweep skips it rather than waiting, and gets it next time.
	sending, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sending.ExecContext(ctx, `SELECT 1 FROM sessions WHERE token_hash='lapsed' FOR SHARE`); err != nil {
		t.Fatal(err)
	}
	quick, stop := context.WithTimeout(ctx, 5*time.Second)
	n, m, err := store.SweepEnded(quick, 10)
	stop()
	if err != nil || n != 1 || m != 2 {
		t.Fatalf("sweep beside a send = %d sessions, %d tokens, %v; want 1, 2", n, m, err)
	}
	_ = sending.Rollback()
	if n, m, err := store.SweepEnded(ctx, 10); err != nil || n != 1 || m != 0 {
		t.Fatalf("next sweep = %d sessions, %d tokens, %v; want 1, 0", n, m, err)
	}
	if n, m, err := store.SweepEnded(ctx, 10); err != nil || n != 0 || m != 0 {
		t.Fatalf("after everything ended was swept: %d sessions, %d tokens, %v", n, m, err)
	}

	var left []string
	rows, err := db.QueryContext(ctx, `SELECT token_hash FROM sessions ORDER BY token_hash`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			t.Fatal(err)
		}
		left = append(left, hash)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(left) != 2 || left[0] != "away" || left[1] != "live" {
		t.Fatalf("sessions left = %v, want [away live]", left)
	}
	var tokensLeft, devicesLeft int
	if err := db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM auth_tokens), (SELECT COUNT(*) FROM push_subscriptions) + (SELECT COUNT(*) FROM mobile_device_tokens)`).Scan(&tokensLeft, &devicesLeft); err != nil {
		t.Fatal(err)
	}
	if tokensLeft != 1 || devicesLeft != 0 {
		t.Fatalf("tokens left = %d, devices left = %d; want 1, 0", tokensLeft, devicesLeft)
	}
}

type countingSweeper struct {
	batches [][2]int64
	calls   int
}

func (c *countingSweeper) SweepEnded(context.Context, int) (int64, int64, error) {
	batch := c.batches[c.calls]
	c.calls++
	return batch[0], batch[1], nil
}

func TestSweepDeletesInBatchesUntilOneComesBackShort(t *testing.T) {
	sw := &countingSweeper{batches: [][2]int64{{sweepBatch, 2}, {7, sweepBatch}, {0, 3}}}
	sessions, tokens := sweep(context.Background(), sw)
	if sw.calls != 3 || sessions != sweepBatch+7 || tokens != sweepBatch+5 {
		t.Fatalf("calls = %d, sessions = %d, tokens = %d", sw.calls, sessions, tokens)
	}
}

func TestRunSweepsStopsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		NewService(NewMemoryStore()).RunSweeps(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunSweeps kept running after its context ended")
	}
}
