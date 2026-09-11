package database

import (
	"context"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/testdb"
	migrationassets "github.com/HammerMeetNail/nabu/migrations"
)

func TestHistoryIndexesUseExistingExtensionNamespace(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA extensions; CREATE EXTENSION pg_trgm WITH SCHEMA extensions`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var valid int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid WHERE c.relname IN ('idx_chore_logs_household_chore_latest','idx_chore_logs_search_trgm') AND i.indisvalid`).Scan(&valid); err != nil {
		t.Fatal(err)
	}
	if valid != 2 {
		t.Fatalf("valid indexes=%d", valid)
	}
}

func TestHistoryIndexMigrationRejectsFailedConcurrentPrebuild(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO households(id,name,invite_code) VALUES (1,'Fixture','fixture')`,
		`INSERT INTO users(id,email,password_hash,display_name) VALUES(1,'fixture@example.invalid','','Fixture')`,
		`INSERT INTO chores(id,household_id,name) VALUES(1,1,'Fixture')`,
		`INSERT INTO chore_logs(household_id,user_id,chore_id) VALUES(1,1,1),(1,1,1)`,
		`DROP INDEX idx_chore_logs_household_chore_latest`,
		`DELETE FROM schema_migrations WHERE name='048_history_query_indexes.sql'`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	// A real failed concurrent build leaves an invalid catalog entry of the name
	// a startup migration would otherwise skip with IF NOT EXISTS.
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX CONCURRENTLY idx_chore_logs_household_chore_latest ON chore_logs(chore_id)`); err == nil {
		t.Fatal("expected duplicate-key build failure")
	}
	if err := Migrate(ctx, db); err == nil {
		t.Fatal("migration accepted invalid prebuilt index")
	}
	var applied bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name='048_history_query_indexes.sql')`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Fatal("failed migration was recorded as applied")
	}
	t.Run("table resolves after first search path schema", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := tx.ExecContext(ctx, `CREATE SCHEMA first_schema; SET LOCAL search_path=first_schema,public`); err != nil {
			t.Fatal(err)
		}
		body, err := migrationassets.Assets.ReadFile("048_history_query_indexes.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err == nil {
			t.Fatal("invalid index accepted when target table resolves in a later schema")
		}
	})
}

func TestNotificationIndexesRejectInvalidPrebuild(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users(id,email,password_hash,display_name) VALUES(1,'synthetic@example.invalid','','Synthetic')`,
		`INSERT INTO notifications(user_id,type,title) VALUES(1,'test','one'),(1,'test','two')`,
		`DROP INDEX idx_notifications_user_created`,
		`DELETE FROM schema_migrations WHERE name='049_notification_query_indexes.sql'`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX CONCURRENTLY idx_notifications_user_created ON notifications(user_id)`); err == nil {
		t.Fatal("expected duplicate failure")
	}
	if err := Migrate(ctx, db); err == nil {
		t.Fatal("invalid notification index accepted")
	}
	var applied bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name='049_notification_query_indexes.sql')`).Scan(&applied); err != nil || applied {
		t.Fatal("failed index migration recorded")
	}
}
