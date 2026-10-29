package middleware

import (
	"context"
	"database/sql"
	"time"
)

// PostgresCounter keeps rate-limit windows in PostgreSQL, so every instance
// that shares the database counts a client's requests in one window.
type PostgresCounter struct{ db *sql.DB }

func NewPostgresCounter(db *sql.DB) *PostgresCounter { return &PostgresCounter{db: db} }

// Hit counts one request from client in scope and returns the window's count
// so far and when it ends. A request after the window ends starts a new one.
func (c *PostgresCounter) Hit(ctx context.Context, scope, client string, window time.Duration) (int, time.Time, error) {
	var count int
	var end time.Time
	err := c.db.QueryRowContext(ctx, `INSERT INTO rate_limits AS r (scope, client, window_end, count)
 VALUES ($1, $2, now() + make_interval(secs => $3), 1)
 ON CONFLICT (scope, client) DO UPDATE SET
  count = CASE WHEN r.window_end <= now() THEN 1 ELSE r.count + 1 END,
  window_end = CASE WHEN r.window_end <= now() THEN now() + make_interval(secs => $3) ELSE r.window_end END
 RETURNING count, window_end`, scope, client, window.Seconds()).Scan(&count, &end)
	return count, end, err
}

// Expire deletes windows that have ended.
func (c *PostgresCounter) Expire(ctx context.Context) error {
	_, err := c.db.ExecContext(ctx, `DELETE FROM rate_limits WHERE window_end <= now()`)
	return err
}
