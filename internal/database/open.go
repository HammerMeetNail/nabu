package database

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Each push worker holds a membership transaction and a nested session
// transaction through one send. The fan-out worker holds a membership
// transaction while it writes on another connection, and the reminder tick
// reads and claims on one. Reserve one leader connection. This is part of the
// configured process budget, not additional capacity on the database server.
const DeliveryConnections = 8

// PushWorkers deliver push jobs concurrently on each instance.
const PushWorkers = 2

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

// Every statement on the app's connections is bounded, so a runaway query or
// a stuck lock wait fails its own request instead of holding a pooled
// connection while others queue behind it. Migrations lift both for their own
// work. A value given in DATABASE_URL (statement_timeout=…, lock_timeout=…)
// wins.
//
// The lock timeout stays above the 10s one push send may take. A send holds its
// recipient's membership and its device's registration and session for that
// long (push.Deliverer, push.PostgresStore WithSubscription), so a removal or
// sign-out never overlaps it, and one that waits behind it still succeeds.
// Each send is guarded on its own, so no wait is longer than one send.
const (
	DefaultStatementTimeout = 30 * time.Second
	DefaultLockTimeout      = 15 * time.Second
)

type PoolOptions struct {
	MaxOpen int
	MaxIdle int
	// Zero means the default above.
	StatementTimeout time.Duration
	LockTimeout      time.Duration
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
	boundStatements(cfg, options)
	metrics := NewQueryMetrics()
	cfg.Tracer = metrics
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(options.MaxOpen)
	db.SetMaxIdleConns(options.MaxIdle)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)
	return db, metrics, nil
}

// boundStatements sets the session timeouts every connection starts with.
// The delivery pool copies this configuration, so it is bounded too.
func boundStatements(cfg *pgx.ConnConfig, options PoolOptions) {
	if cfg.RuntimeParams == nil {
		cfg.RuntimeParams = map[string]string{}
	}
	set := func(name string, value, fallback time.Duration) {
		if _, given := cfg.RuntimeParams[name]; given {
			return
		}
		if value <= 0 {
			value = fallback
		}
		cfg.RuntimeParams[name] = strconv.FormatInt(value.Milliseconds(), 10)
	}
	set("statement_timeout", options.StatementTimeout, DefaultStatementTimeout)
	set("lock_timeout", options.LockTimeout, DefaultLockTimeout)
}
