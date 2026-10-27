package database

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func migratedDB(t *testing.T) *sql.DB {
	t.Helper()
	db := testdb.New(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func queryStrings(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// Every foreign key has an index leading with its columns, so deleting or
// updating the referenced row never scans the referencing table. A new
// foreign key needs its index in the same migration.
func TestEveryForeignKeyIsIndexed(t *testing.T) {
	missing := queryStrings(t, migratedDB(t), `
		SELECT c.conrelid::regclass::text || ' (' || string_agg(a.attname, ', ' ORDER BY k.n) || ')'
		FROM pg_constraint c
		CROSS JOIN LATERAL unnest(c.conkey) WITH ORDINALITY AS k(attnum, n)
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum
		WHERE c.contype = 'f' AND c.connamespace = 'public'::regnamespace
		  AND NOT EXISTS (
		      SELECT 1 FROM pg_index i
		      WHERE i.indrelid = c.conrelid
		        AND (string_to_array(i.indkey::text, ' ')::int2[])[1:array_length(c.conkey, 1)] @> c.conkey
		        AND (string_to_array(i.indkey::text, ' ')::int2[])[1:array_length(c.conkey, 1)] <@ c.conkey)
		GROUP BY c.conrelid, c.conname
		ORDER BY 1`)
	if len(missing) > 0 {
		t.Fatalf("foreign keys without an index on their columns: %v", missing)
	}
}

// No index repeats another's columns, expressions, predicate and operator
// classes on the same table: the copy costs every write and buys nothing.
func TestNoIndexDuplicatesAnother(t *testing.T) {
	duplicates := queryStrings(t, migratedDB(t), `
		SELECT string_agg(indexrelid::regclass::text, ' = ' ORDER BY indexrelid::regclass::text)
		FROM pg_index
		WHERE indrelid IN (SELECT oid FROM pg_class WHERE relnamespace = 'public'::regnamespace)
		GROUP BY indrelid, indkey::text, indclass::text,
		         coalesce(pg_get_expr(indexprs, indrelid), ''), coalesce(pg_get_expr(indpred, indrelid), '')
		HAVING count(*) > 1
		ORDER BY 1`)
	if len(duplicates) > 0 {
		t.Fatalf("duplicate indexes: %v", duplicates)
	}
}

// Like the history and notification indexes, a foreign key index prebuilt
// CONCURRENTLY and interrupted must stop startup, not be skipped by IF NOT
// EXISTS.
func TestForeignKeyIndexesRejectInvalidPrebuild(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO users(id,email,password_hash,display_name) VALUES(1,'synthetic@example.invalid','','Synthetic')`,
		`INSERT INTO sessions(id,user_id,token_hash,expires_at) VALUES('a',1,'hash-a',now()+interval '1 day'),('b',1,'hash-b',now()+interval '1 day')`,
		`DROP INDEX idx_sessions_user`,
		`DELETE FROM schema_migrations WHERE name='058_foreign_key_indexes.sql'`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	// Two sessions for one user fail a unique build, leaving it invalid.
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX CONCURRENTLY idx_sessions_user ON sessions(user_id)`); err == nil {
		t.Fatal("expected duplicate failure")
	}
	if err := Migrate(ctx, db); err == nil {
		t.Fatal("invalid foreign key index accepted")
	}
	var applied bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name='058_foreign_key_indexes.sql')`).Scan(&applied); err != nil || applied {
		t.Fatal("failed index migration recorded")
	}
	// Dropped and retried, it completes.
	if _, err := db.ExecContext(ctx, `DROP INDEX idx_sessions_user`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
}
