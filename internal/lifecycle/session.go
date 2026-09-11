package lifecycle

import (
	"context"
	"database/sql"
)

// SessionGuard serializes a memory-mode device write with session revocation.
// The callback must not call back into authentication or household stores.
type SessionGuard interface {
	WithSession(context.Context, int64, string, func() error) error
}

// WithSQLSession follows credential mutation's user -> session lock order.
// Holding these locks through a bounded delivery makes revocation a boundary:
// it waits for entered sends, and no old snapshot can start after it commits.
func WithSQLSession(ctx context.Context, db *sql.DB, userID int64, hash string, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var epoch int64
	if err := tx.QueryRowContext(ctx, `SELECT auth_version FROM users WHERE id=$1 FOR SHARE`, userID).Scan(&epoch); err != nil {
		return err
	}
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM sessions WHERE token_hash=$1 AND user_id=$2 AND auth_version=$3 FOR SHARE`, hash, userID, epoch).Scan(&id); err != nil {
		return err
	}
	// NOW() is the transaction start, which may precede a long lock wait.
	// Check the wall clock only after both authorization locks are held.
	var valid bool
	if err := tx.QueryRowContext(ctx, `SELECT expires_at>clock_timestamp() AND last_seen_at>clock_timestamp()-INTERVAL '24 hours'
 FROM sessions WHERE id=$1`, id).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return sql.ErrNoRows
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
