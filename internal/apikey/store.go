package apikey

import (
	"context"
	"time"
)

// Record is a stored key. TokenHash is the SHA-256 digest of the full token.
type Record struct {
	ID          string
	UserID      int64
	Name        string
	TokenHash   []byte
	AuthVersion int64
	CreatedAt   time.Time
	LastUsedAt  *time.Time
	RevokedAt   *time.Time
}

// Store persists personal API keys.
type Store interface {
	// Create inserts rec unless the user already has maxActive keys that are
	// unrevoked and bound to rec.AuthVersion, in which case it returns
	// ErrTooManyKeys. The count and insert are atomic per user.
	Create(ctx context.Context, rec Record, maxActive int) error
	// List returns the user's keys, newest first.
	List(ctx context.Context, userID int64) ([]Record, error)
	// Get returns the key with the given public ID or ErrNotFound.
	Get(ctx context.Context, id string) (Record, error)
	// Revoke marks the user's key revoked. It returns ErrNotFound when the key
	// does not exist or belongs to someone else. Revoking twice is a no-op.
	Revoke(ctx context.Context, userID int64, id string, now time.Time) error
	// Touch records use, at most once per hour per key.
	Touch(ctx context.Context, id string, now time.Time) error
}
