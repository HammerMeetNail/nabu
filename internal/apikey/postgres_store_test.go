package apikey_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/apikey"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func TestPostgresStoreLifecycle(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO users(id,email,password_hash,display_name) VALUES
		(1,'keys-a@example.invalid','','A'),(2,'keys-b@example.invalid','','B')`); err != nil {
		t.Fatal(err)
	}
	var version int64
	if err := db.QueryRowContext(ctx, `SELECT auth_version FROM users WHERE id = 1`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	store := apikey.NewPostgresStore(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	rec := apikey.Record{ID: "aaaaaaaaaaaaaaaa", UserID: 1, Name: "k", TokenHash: []byte("digest-a"), AuthVersion: version, CreatedAt: now}
	if err := store.Create(ctx, rec, 1); err != nil {
		t.Fatal(err)
	}
	second := rec
	second.ID, second.TokenHash = "bbbbbbbbbbbbbbbb", []byte("digest-b")
	if err := store.Create(ctx, second, 1); !errors.Is(err, apikey.ErrTooManyKeys) {
		t.Fatalf("want ErrTooManyKeys, got %v", err)
	}
	stale := second
	stale.AuthVersion = version + 1
	if err := store.Create(ctx, stale, 5); !errors.Is(err, apikey.ErrInvalidKey) {
		t.Fatalf("stale auth version: want ErrInvalidKey, got %v", err)
	}

	got, err := store.Get(ctx, rec.ID)
	if err != nil || got.UserID != 1 || string(got.TokenHash) != "digest-a" || got.RevokedAt != nil {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := store.Get(ctx, "missingmissingmi"); !errors.Is(err, apikey.ErrNotFound) {
		t.Fatalf("missing key: want ErrNotFound, got %v", err)
	}

	if err := store.Touch(ctx, rec.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Touch(ctx, rec.ID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get(ctx, rec.ID)
	if got.LastUsedAt == nil || !got.LastUsedAt.Equal(now) {
		t.Fatalf("touch should be throttled to hourly, got %v", got.LastUsedAt)
	}

	if err := store.Revoke(ctx, 2, rec.ID, now); !errors.Is(err, apikey.ErrNotFound) {
		t.Fatalf("other user revoke: want ErrNotFound, got %v", err)
	}
	if err := store.Revoke(ctx, 1, rec.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(ctx, 1, rec.ID, now.Add(time.Hour)); err != nil {
		t.Fatalf("second revoke should be a no-op: %v", err)
	}
	list, err := store.List(ctx, 1)
	if err != nil || len(list) != 1 || list[0].RevokedAt == nil || !list[0].RevokedAt.Equal(now) {
		t.Fatalf("list: %+v %v", list, err)
	}
	if err := store.Create(ctx, second, 1); err != nil {
		t.Fatalf("revoked keys must not count toward the cap: %v", err)
	}
	if others, _ := store.List(ctx, 2); len(others) != 0 {
		t.Fatalf("user 2 should see no keys, got %+v", others)
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM users WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, second.ID); !errors.Is(err, apikey.ErrNotFound) {
		t.Fatalf("keys should be deleted with their owner, got %v", err)
	}
}
