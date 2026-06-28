package reminder

import (
	"context"
	"database/sql"
	"sync"
)

// LeaderLockKey is the Postgres advisory-lock key used to guard the reminder
// scheduler. It is an arbitrary application-chosen constant; it only needs to be
// stable and not collide with any other advisory lock the app uses.
const LeaderLockKey int64 = 0x6e6162755f726d64 // "nabu_rmd"

// LeaderLock is an optional single-runner guard for the scheduler. When set on a
// Scheduler, ticks only run while this instance holds leadership, preventing
// duplicate reminder notifications when multiple app instances run concurrently.
type LeaderLock interface {
	// TryAcquire reports whether this instance currently holds leadership,
	// attempting to acquire it if not already held.
	TryAcquire(ctx context.Context) (bool, error)
	// Release relinquishes leadership (best effort).
	Release(ctx context.Context) error
}

// PostgresAdvisoryLock implements LeaderLock using a Postgres session-level
// advisory lock held on a dedicated connection. The lock is automatically
// released by Postgres when the holding connection (or the whole process) dies,
// so a crashed leader is transparently replaced by a follower on its next
// acquisition attempt — no lease table, heartbeat, or migration required.
type PostgresAdvisoryLock struct {
	db  *sql.DB
	key int64

	mu   sync.Mutex
	conn *sql.Conn // non-nil while leadership is held
}

func NewPostgresAdvisoryLock(db *sql.DB, key int64) *PostgresAdvisoryLock {
	return &PostgresAdvisoryLock{db: db, key: key}
}

func (l *PostgresAdvisoryLock) TryAcquire(ctx context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn != nil {
		// Already leader — confirm the underlying connection (and thus the
		// session lock) is still alive. If it dropped, Postgres has already
		// released the lock, so discard the conn and re-attempt acquisition.
		if err := l.conn.PingContext(ctx); err == nil {
			return true, nil
		}
		_ = l.conn.Close()
		l.conn = nil
	}

	conn, err := l.db.Conn(ctx)
	if err != nil {
		return false, err
	}
	var acquired bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", l.key).Scan(&acquired); err != nil {
		_ = conn.Close()
		return false, err
	}
	if !acquired {
		// Another instance holds leadership; return the connection to the pool.
		_ = conn.Close()
		return false, nil
	}
	l.conn = conn
	return true, nil
}

func (l *PostgresAdvisoryLock) Release(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return nil
	}
	_, unlockErr := l.conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", l.key)
	closeErr := l.conn.Close()
	l.conn = nil
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
