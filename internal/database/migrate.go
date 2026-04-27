package database

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	migrationassets "github.com/dave/choresy/migrations"
)

func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[string]struct{}{}
	rows, err := db.QueryContext(ctx, `SELECT name FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate schema_migrations: %w", err)
	}

	names, err := migrationassets.Names()
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(names)

	for _, name := range names {
		if _, ok := applied[name]; ok {
			continue
		}
		body, err := migrationassets.Assets.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(name) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}

	return nil
}
