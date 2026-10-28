package push

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

// jobDatabase holds user 1 in household 1 with chore 1.
func jobDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db := testdb.New(t)
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash,display_name) VALUES (1,'one@example.invalid','','One');
		INSERT INTO households(id,name,invite_code) VALUES (1,'Home','invite-1');
		INSERT INTO chores(id,household_id,name,visibility) VALUES (1,1,'Dishes','household')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func countJobs(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM push_jobs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPostgresJobStoreClaimsLeasesAndFinishes(t *testing.T) {
	db := jobDatabase(t)
	jobs := NewPostgresJobStore(db)
	ctx := context.Background()
	// Any clock: a new job is due at once. Times are whole microseconds,
	// PostgreSQL's precision.
	now := time.Date(2020, 1, 1, 9, 0, 0, 0, time.UTC)
	logged := Job{UserID: 1, HouseholdID: 1, ChoreID: 1, Type: "chore_logged", Title: "🍽️ Dishes", Body: "Alice logged this",
		Data: map[string]any{"choreId": 1, "type": "chore_logged"}, DedupeKey: "event:1:1", ExpiresAt: now.Add(2 * time.Hour)}
	for range 2 {
		if err := jobs.EnqueuePush(ctx, logged); err != nil {
			t.Fatal(err)
		}
	}
	joined := Job{UserID: 1, HouseholdID: 1, Type: "household_joined", Title: "New Member", ExpiresAt: now.Add(2 * time.Hour)}
	if err := jobs.EnqueuePush(ctx, joined); err != nil {
		t.Fatal(err)
	}
	if n := countJobs(t, db); n != 2 {
		t.Fatalf("jobs = %d, want the duplicate dropped", n)
	}

	first, err := jobs.ClaimPush(ctx, now, now.Add(jobLease), "lease-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Title != logged.Title || first.Body != logged.Body || first.ChoreID != 1 || first.DedupeKey != "event:1:1" ||
		first.Attempts != 1 || first.LeaseID != "lease-1" || len(first.Done) != 0 ||
		first.Data["choreId"] != float64(1) || first.Data["type"] != "chore_logged" || !first.ExpiresAt.Equal(logged.ExpiresAt) {
		t.Fatalf("claimed %+v", first)
	}
	second, err := jobs.ClaimPush(ctx, now, now.Add(jobLease), "lease-2")
	if err != nil || second.ChoreID != 0 || second.Type != "household_joined" {
		t.Fatalf("second claim = %+v %v", second, err)
	}
	if _, err := jobs.ClaimPush(ctx, now, now.Add(jobLease), "lease-3"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("leased jobs claimed again: %v", err)
	}

	// Progress kept for a retry, under the lease that holds the job.
	first.Done = []string{"web:a"}
	stale := first
	stale.LeaseID = "someone-else"
	if err := jobs.FinishPush(ctx, stale, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := jobs.FinishPush(ctx, first, now.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.ClaimPush(ctx, now.Add(10*time.Second-time.Microsecond), now.Add(time.Minute), "early"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("claimed before its retry: %v", err)
	}
	retried, err := jobs.ClaimPush(ctx, now.Add(10*time.Second), now.Add(time.Minute), "lease-4")
	if err != nil || retried.ID != first.ID || retried.Attempts != 2 || !slices.Equal(retried.Done, []string{"web:a"}) {
		t.Fatalf("retry claim = %+v %v", retried, err)
	}

	// A finished job is never claimed again, and keeps its key until it
	// expires.
	if err := jobs.FinishPush(ctx, retried, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := jobs.FinishPush(ctx, second, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.ClaimPush(ctx, now.Add(time.Hour), now.Add(time.Hour+time.Minute), "late"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("finished job claimed: %v", err)
	}
	if err := jobs.EnqueuePush(ctx, logged); err != nil || countJobs(t, db) != 2 {
		t.Fatalf("finished job's duplicate queued: %v", err)
	}
	if _, err := jobs.ClaimPush(ctx, now.Add(2*time.Hour), now.Add(3*time.Hour), "expired"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expired job claimed: %v", err)
	}
	if n := countJobs(t, db); n != 0 {
		t.Fatalf("expired jobs kept: %d", n)
	}
}

// A claim skips a job another worker is claiming instead of waiting for it.
func TestPostgresJobClaimSkipsLockedJobs(t *testing.T) {
	db := jobDatabase(t)
	jobs := NewPostgresJobStore(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := jobs.EnqueuePush(ctx, Job{UserID: 1, HouseholdID: 1, Type: "household_joined", Title: "t", ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT id FROM push_jobs FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.ClaimPush(ctx, now.Add(time.Second), now.Add(time.Minute), "other"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("claim of a locked job: %v", err)
	}
}

// A job goes with its recipient, household or activity.
func TestPostgresJobsFollowTheirReferences(t *testing.T) {
	for _, deletion := range []string{`DELETE FROM chores WHERE id=1`, `DELETE FROM users WHERE id=1`, `DELETE FROM households WHERE id=1`} {
		t.Run(deletion, func(t *testing.T) {
			db := jobDatabase(t)
			if err := NewPostgresJobStore(db).EnqueuePush(context.Background(), Job{UserID: 1, HouseholdID: 1, ChoreID: 1,
				Type: "chore_logged", Title: "t", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(deletion); err != nil {
				t.Fatal(err)
			}
			if n := countJobs(t, db); n != 0 {
				t.Fatalf("jobs left = %d", n)
			}
		})
	}
}
