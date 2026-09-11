package log_test

import (
	"context"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/database"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/testdb"
	"github.com/HammerMeetNail/nabu/internal/testsync"
)

func TestPostgresConcurrentLogPatchesMergeFields(t *testing.T) {
	db := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO households(id,name,invite_code) VALUES(1,'Synthetic','PATCH')`,
		`INSERT INTO users(id,email,password_hash,display_name) VALUES(1,'patch@example.invalid','','Synthetic')`,
		`INSERT INTO chores(id,household_id,name) VALUES(1,1,'Synthetic')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	store := chorelog.NewPostgresStore(db)
	duration := 90
	original, err := store.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: 1, UserID: 1, ChoreID: 1, CompletedAt: time.Now(), Note: "original", DurationSeconds: &duration})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	var pid int
	if err := tx.QueryRowContext(ctx, `SELECT pg_backend_pid() FROM chore_logs WHERE id=$1 FOR UPDATE`, original.ID).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	noteEdit, durationEdit := original, original
	noteEdit.Note = "changed"
	nextDuration := 135
	durationEdit.DurationSeconds = &nextDuration
	done := make(chan error, 2)
	go func() { done <- store.UpdateLog(ctx, noteEdit, chorelog.LogFields{"note": true}) }()
	go func() { done <- store.UpdateLog(ctx, durationEdit, chorelog.LogFields{"durationSeconds": true}) }()
	testdb.WaitBlocked(t, db, pid, "UPDATE chore_logs SET note=")
	testdb.WaitBlocked(t, db, pid, "UPDATE chore_logs SET duration_seconds=")
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := testsync.Receive(t, ctx, done); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.GetLog(ctx, original.ID)
	if err != nil || got.Note != "changed" || got.DurationSeconds == nil || *got.DurationSeconds != 135 {
		t.Fatalf("concurrent fields lost: %+v %v", got, err)
	}
	foreign := original
	foreign.HouseholdID = 2
	foreign.Note = "foreign"
	if err := store.UpdateLog(ctx, foreign, chorelog.LogFields{"note": true}); err != chorelog.ErrNotFound {
		t.Fatalf("cross-household update: %v", err)
	}
	cleared := original
	cleared.DurationSeconds = nil
	if err := store.UpdateLog(ctx, cleared, chorelog.LogFields{"durationSeconds": true}); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetLog(ctx, original.ID)
	if err != nil || got.DurationSeconds != nil || got.Note != "changed" {
		t.Fatalf("clear changed another field: %+v %v", got, err)
	}
}
