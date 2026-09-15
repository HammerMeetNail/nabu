package auth

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/mail"
)

func TestPostgresAuthMailDueLeaseAndExpiryBoundaries(t *testing.T) {
	store := postgresClaimStore(t).(*PostgresStore)
	ctx := context.Background()
	user, err := store.CreateUser(ctx, "outbox-boundaries@example.invalid", "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	// PostgreSQL persists microseconds, so one microsecond is the exact
	// representable boundary before each due, lease, and expiry timestamp.
	due := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	expires := due.Add(2 * time.Minute)
	if err := store.QueueAuthMail(ctx, user.ID, mail.Message{To: user.Email, Subject: "Synthetic", Body: "Synthetic fixture"}, expires); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE auth_mail_outbox SET next_attempt=$1 WHERE user_id=$2`, due, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimAuthMail(ctx, due.Add(-time.Microsecond), due.Add(time.Minute), "early"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("claimed before due time: %v", err)
	}
	first, err := store.ClaimAuthMail(ctx, due, due.Add(time.Minute), "first")
	if err != nil {
		t.Fatal(err)
	}
	if first.Attempts != 1 || !first.NextAttempt.Equal(due) || !first.LeaseUntil.Equal(due.Add(time.Minute)) {
		t.Fatal("first claim did not preserve the due time and acquire its lease")
	}
	if _, err := store.ClaimAuthMail(ctx, first.LeaseUntil.Add(-time.Microsecond), expires, "early-reclaim"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("reclaimed before lease expiry: %v", err)
	}
	second, err := store.ClaimAuthMail(ctx, first.LeaseUntil, expires, "second")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Attempts != 2 || second.LeaseID != "second" {
		t.Fatal("expired lease was not reclaimed as a new attempt")
	}
	for _, delivered := range []bool{false, true} {
		if err := store.FinishAuthMail(ctx, first, delivered, expires.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		var leaseID string
		var leaseUntil, nextAttempt time.Time
		var attempts int
		if err := store.db.QueryRow(`SELECT lease_id, lease_until, next_attempt, attempts
            FROM auth_mail_outbox WHERE id=$1`, second.ID).Scan(&leaseID, &leaseUntil, &nextAttempt, &attempts); err != nil {
			t.Fatal(err)
		}
		if leaseID != second.LeaseID || !leaseUntil.Equal(second.LeaseUntil) || !nextAttempt.Equal(due) || attempts != 2 {
			t.Fatal("stale lease completion changed the current claim")
		}
	}
	lastDue := expires.Add(-time.Microsecond)
	if err := store.FinishAuthMail(ctx, second, false, lastDue); err != nil {
		t.Fatal(err)
	}
	last, err := store.ClaimAuthMail(ctx, lastDue, expires.Add(time.Minute), "last")
	if err != nil {
		t.Fatalf("could not claim immediately before message expiry: %v", err)
	}
	if last.ID != first.ID || last.Attempts != 3 {
		t.Fatal("last eligible claim did not retain the email and increment attempts")
	}
	if _, err := store.ClaimAuthMail(ctx, expires, expires.Add(time.Minute), "expired"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("claimed at message expiry: %v", err)
	}
	var queued int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM auth_mail_outbox WHERE user_id=$1`, user.ID).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("expired email was not purged: count=%d error=%v", queued, err)
	}
}
