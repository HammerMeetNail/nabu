package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Two notification tasks plus one sequential reminder tick can each hold a
// membership transaction and a nested session transaction. Reserve one leader
// connection and one spare for candidate reads. This is part of the configured
// process budget, not additional capacity on the database server.
const DeliveryConnections = 8

func ForkDeliveryPool(ctx context.Context, db *sql.DB) (*sql.DB, *QueryMetrics, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()
	var cfg *pgx.ConnConfig
	if err := conn.Raw(func(raw any) error {
		pg, ok := raw.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("delivery pool requires PostgreSQL")
		}
		cfg = pg.Conn().Config().Copy()
		return nil
	}); err != nil {
		return nil, nil, err
	}
	metrics := NewQueryMetrics()
	cfg.Tracer = metrics
	background := stdlib.OpenDB(*cfg)
	background.SetMaxOpenConns(DeliveryConnections)
	background.SetMaxIdleConns(2)
	background.SetConnMaxLifetime(5 * time.Minute)
	background.SetConnMaxIdleTime(time.Minute)
	return background, metrics, nil
}

type PoolOptions struct {
	MaxOpen int
	MaxIdle int
}

func Open(databaseURL string) (*sql.DB, error) {
	db, _, err := OpenMeasured(databaseURL, PoolOptions{MaxOpen: 25, MaxIdle: 5})
	return db, err
}

// OpenMeasured records only timing/counts, never SQL, parameters or connection
// strings. One connection may be held by the scheduler's leader lock.
func OpenMeasured(databaseURL string, options PoolOptions) (*sql.DB, *QueryMetrics, error) {
	if options.MaxOpen < 2 || options.MaxOpen > 100 || options.MaxIdle < 0 || options.MaxIdle > options.MaxOpen {
		return nil, nil, fmt.Errorf("invalid database pool limits")
	}
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid database connection configuration")
	}
	metrics := NewQueryMetrics()
	cfg.Tracer = metrics
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(options.MaxOpen)
	db.SetMaxIdleConns(options.MaxIdle)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)
	return db, metrics, nil
}
