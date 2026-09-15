package auth

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/mail"
)

func failSessionInsert(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`CREATE FUNCTION reject_session() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'synthetic session write failure'; END; $$ LANGUAGE plpgsql;
		CREATE TRIGGER reject_session BEFORE INSERT ON sessions FOR EACH ROW EXECUTE FUNCTION reject_session()`)
	if err != nil {
		t.Fatal(err)
	}
}

// Deliver a fixture's first registration email at its persisted due time.
// PostgreSQL NOW() and the Go test process may use different machine clocks.
// This helper must not bypass a retry delay, an active lease, or expiry.
func deliverInitialAuthMail(t *testing.T, svc *Service, userID int64) time.Time {
	t.Helper()
	now := svc.now()
	if store, ok := svc.store.(*PostgresStore); ok {
		var expiresAt time.Time
		var leaseUntil sql.NullTime
		var attempts, rows int
		err := store.db.QueryRow(`SELECT next_attempt, expires_at, lease_until, attempts, COUNT(*) OVER ()
            FROM auth_mail_outbox WHERE user_id=$1`, userID).Scan(&now, &expiresAt, &leaseUntil, &attempts, &rows)
		if err != nil {
			t.Fatal(err)
		}
		if rows != 1 || attempts != 0 || leaseUntil.Valid || !now.Before(expiresAt) {
			t.Fatal("expected one unattempted, unleased, unexpired registration email")
		}
	}
	originalClock := svc.now
	svc.now = func() time.Time { return now }
	defer func() { svc.now = originalClock }()
	svc.DeliverPendingMail(context.Background())
	return now
}

func TestPostgresRegistrationRollsBackAndCanRetry(t *testing.T) {
	for _, clockSkew := range []time.Duration{-5 * time.Second, 0, 5 * time.Second} {
		t.Run(clockSkew.String(), func(t *testing.T) {
			checkPostgresRegistrationRollback(t, clockSkew)
		})
	}
}

func checkPostgresRegistrationRollback(t *testing.T, clockSkew time.Duration) {
	t.Helper()
	store := postgresClaimStore(t).(*PostgresStore)
	svc := NewService(store)
	applicationTime := time.Now().UTC().Add(clockSkew)
	svc.now = func() time.Time { return applicationTime }
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")
	failSessionInsert(t, store.db)
	ctx := context.Background()
	if _, _, err := svc.RegisterWithHash(ctx, "retry@example.invalid", "synthetic"); err == nil {
		t.Fatal("expected failure")
	}
	for _, table := range []string{"users", "sessions", "auth_tokens", "auth_mail_outbox"} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("partial registration in %s: %d", table, count)
		}
	}
	if len(mailer.Messages()) != 0 {
		t.Fatal("sent proof for rolled-back account")
	}
	if _, err := store.db.Exec(`DROP TRIGGER reject_session ON sessions`); err != nil {
		t.Fatal(err)
	}
	u, session, err := svc.RegisterWithHash(ctx, "retry@example.invalid", "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	deliverInitialAuthMail(t, svc, u.ID)
	if !svc.now().Equal(applicationTime) {
		t.Fatal("fixture delivery changed the application clock")
	}
	if got, err := svc.Authenticate(ctx, session.ID); err != nil || got.ID != u.ID {
		t.Fatalf("retry not authenticated: %v", err)
	}
	if len(mailer.Messages()) != 1 {
		t.Fatal("retry did not deliver queued verification")
	}
}

func TestPostgresClaimRollsBackCredentialsAndProofOnSessionFailure(t *testing.T) {
	store := postgresClaimStore(t).(*PostgresStore)
	svc, user, old, mailer := claimFixture(t, store)
	token := extractToken(mailer.Messages()[0].Body, "token=")
	failSessionInsert(t, store.db)
	ctx := context.Background()
	if _, _, err := svc.VerifyEmailAndLogin(ctx, token); err == nil {
		t.Fatal("expected claim failure")
	}
	got, err := svc.Authenticate(ctx, old.ID)
	if err != nil || got.EmailVerified || got.AuthVersion != user.AuthVersion {
		t.Fatalf("partial claim: %+v %v", got, err)
	}
	if _, err := store.GetAuthToken(ctx, hashToken(token), "verify"); err != nil {
		t.Fatalf("proof lost on rollback: %v", err)
	}
	if _, err := store.db.Exec(`DROP TRIGGER reject_session ON sessions`); err != nil {
		t.Fatal(err)
	}
	claimed, _, err := svc.VerifyEmailAndLogin(ctx, token)
	if err != nil || !claimed.EmailVerified || claimed.HasPassword {
		t.Fatalf("claim retry failed: %+v %v", claimed, err)
	}
	if _, err := svc.Authenticate(ctx, old.ID); err == nil {
		t.Fatal("old session survived retry")
	}
}

func TestPostgresMailOutboxSurvivesFailureAndRestart(t *testing.T) {
	store := postgresClaimStore(t).(*PostgresStore)
	svc := NewService(store)
	svc.SetMailer(unavailableTestMailer{}, "http://localhost:8080")
	u, session, err := svc.RegisterWithHash(context.Background(), "pending@example.invalid", "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	firstAttempt := deliverInitialAuthMail(t, svc, u.ID)
	if _, err := svc.Authenticate(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	var nextAttempt time.Time
	var attempts int
	var leaseUntil sql.NullTime
	var leaseID string
	if err := store.db.QueryRow(`SELECT next_attempt, attempts, lease_until, lease_id
        FROM auth_mail_outbox WHERE user_id=$1`, u.ID).Scan(&nextAttempt, &attempts, &leaseUntil, &leaseID); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || leaseUntil.Valid || leaseID != "" || !nextAttempt.Equal(firstAttempt.Add(time.Minute)) {
		t.Fatal("failed delivery did not retain an unleased email with the first retry delay")
	}
	// A new service instance models a restart; persisted attempts and content
	// must be sufficient to resume without the original HTTP request.
	restarted := NewService(NewPostgresStore(store.db))
	mailer := mail.NewMemorySender()
	restarted.SetMailer(mailer, "http://localhost:8080")
	now := nextAttempt.Add(-time.Microsecond)
	restarted.now = func() time.Time { return now }
	restarted.DeliverPendingMail(context.Background())
	if len(mailer.Messages()) != 0 {
		t.Fatal("pending mail delivered before its retry deadline")
	}
	if err := store.db.QueryRow(`SELECT attempts FROM auth_mail_outbox WHERE user_id=$1`, u.ID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatalf("early retry claimed the pending email: attempts=%d error=%v", attempts, err)
	}
	now = nextAttempt
	restarted.DeliverPendingMail(context.Background())
	if len(mailer.Messages()) != 1 {
		t.Fatal("pending mail not delivered after restart")
	}
	if _, err := store.ClaimAuthMail(context.Background(), restarted.now(), restarted.now().Add(time.Minute), "probe"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("delivered capability retained: %v", err)
	}
	var queued int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM auth_mail_outbox WHERE user_id=$1`, u.ID).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("delivered email was not deleted: count=%d error=%v", queued, err)
	}
}

func TestPostgresConcurrentProofConsumption(t *testing.T) {
	store := postgresClaimStore(t)
	svc, _, _, mailer := claimFixture(t, store)
	token := extractToken(mailer.Messages()[0].Body, "token=")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; _, _, err := svc.VerifyEmailAndLogin(ctx, token); results <- err }()
	}
	close(start)
	success := 0
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err == nil {
				success++
			} else if !errors.Is(err, ErrInvalidToken) {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if success != 1 {
		t.Fatalf("proof accepted %d times", success)
	}
}

func TestPostgresDeleteLocksUserBeforeTokens(t *testing.T) {
	store := postgresClaimStore(t).(*PostgresStore)
	svc, user, _, mailer := claimFixture(t, store)
	token := extractToken(mailer.Messages()[0].Body, "token=")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	claimPID, deletePID := make(chan int, 1), make(chan int, 1)
	resume := make(chan struct{})
	claimDone, deleteDone := make(chan error, 1), make(chan error, 1)
	go func() {
		claimDone <- store.InTransaction(ctx, func(tx Store) error {
			pg := tx.(*PostgresStore)
			if _, err := tx.GetUserByID(ctx, user.ID); err != nil {
				return err
			}
			var pid int
			if err := pg.q.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			claimPID <- pid
			select {
			case <-resume:
			case <-ctx.Done():
				return ctx.Err()
			}
			inside := *svc
			inside.store = tx
			_, _, err := inside.VerifyEmailAndLogin(ctx, token)
			return err
		})
	}()
	var claimant int
	select {
	case claimant = <-claimPID:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go func() {
		deleteDone <- store.InTransaction(ctx, func(tx Store) error {
			var pid int
			if err := tx.(*PostgresStore).q.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			deletePID <- pid
			return tx.DeleteUser(ctx, user.ID)
		})
	}()
	var deleting int
	select {
	case deleting = <-deletePID:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Observe the actual DB lock wait before inspecting token ownership.
	for {
		var waiting bool
		if err := store.db.QueryRowContext(ctx, `SELECT $1 = ANY(pg_blocking_pids($2))`, claimant, deleting).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	var id int64
	err := store.db.QueryRowContext(ctx, `SELECT id FROM auth_tokens WHERE token_hash=$1 FOR UPDATE NOWAIT`, hashToken(token)).Scan(&id)
	close(resume)
	if err != nil {
		t.Errorf("deletion locked proof before user: %v", err)
	}
	for _, done := range []chan error{claimDone, deleteDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if _, err := store.GetUserByID(ctx, user.ID); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("deletion did not complete: %v", err)
	}
}
