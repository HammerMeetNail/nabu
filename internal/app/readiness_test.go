package app

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/config"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func TestBuildServerDoesNotServeAfterMigrationFailure(t *testing.T) {
	db := testdb.New(t)
	if _, err := db.Exec(`CREATE TABLE schema_migrations (invalid integer)`); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.DatabaseURL = "isolated migration fixture"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h, closer, err := buildServer(ctx, cfg, func(string, database.PoolOptions) (*sql.DB, *database.QueryMetrics, error) { return db, nil, nil })
	if err == nil || h != nil || closer != nil {
		t.Fatal("failed migration published a server")
	}
}

func TestBuildServerValidatesBeforeOpeningDatabase(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.TrustedProxyCIDRs = "invalid"
	cfg.DatabaseURL = "invalid database that must never be opened"
	h, closer, err := BuildServer(context.Background(), cfg)
	if err == nil || h != nil || closer != nil {
		t.Fatal("invalid configuration started a server")
	}
}
