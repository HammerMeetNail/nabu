package apikey

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type postgresStore struct {
	db *sql.DB
}

// NewPostgresStore returns a Store backed by PostgreSQL.
func NewPostgresStore(db *sql.DB) Store {
	return &postgresStore{db: db}
}

func nullTime(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// Create locks the owner's user row so the active-key cap holds across
// concurrent requests, including separate app replicas.
func (s *postgresStore) Create(ctx context.Context, rec Record, maxActive int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var version int64
	err = tx.QueryRowContext(ctx, `SELECT auth_version FROM users WHERE id = $1 FOR UPDATE`, rec.UserID).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if version != rec.AuthVersion {
		return ErrInvalidKey
	}
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_api_keys
		WHERE user_id = $1 AND revoked_at IS NULL AND auth_version = $2`, rec.UserID, version).Scan(&active); err != nil {
		return err
	}
	if active >= maxActive {
		return ErrTooManyKeys
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO user_api_keys (id, user_id, name, token_hash, auth_version, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, rec.ID, rec.UserID, rec.Name, rec.TokenHash, rec.AuthVersion, rec.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *postgresStore) List(ctx context.Context, userID int64) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, name, token_hash, auth_version, created_at, last_used_at, revoked_at
		FROM user_api_keys WHERE user_id = $1 ORDER BY created_at DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Record, 0)
	for rows.Next() {
		var rec Record
		var used, revoked sql.NullTime
		if err := rows.Scan(&rec.ID, &rec.UserID, &rec.Name, &rec.TokenHash, &rec.AuthVersion, &rec.CreatedAt, &used, &revoked); err != nil {
			return nil, err
		}
		rec.LastUsedAt, rec.RevokedAt = nullTime(used), nullTime(revoked)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *postgresStore) Get(ctx context.Context, id string) (Record, error) {
	var rec Record
	var used, revoked sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id, name, token_hash, auth_version, created_at, last_used_at, revoked_at
		FROM user_api_keys WHERE id = $1`, id).Scan(&rec.ID, &rec.UserID, &rec.Name, &rec.TokenHash, &rec.AuthVersion, &rec.CreatedAt, &used, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, err
	}
	rec.LastUsedAt, rec.RevokedAt = nullTime(used), nullTime(revoked)
	return rec, nil
}

func (s *postgresStore) Revoke(ctx context.Context, userID int64, id string, now time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE user_api_keys SET revoked_at = COALESCE(revoked_at, $3)
		WHERE id = $1 AND user_id = $2`, id, userID, now)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *postgresStore) Touch(ctx context.Context, id string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE user_api_keys SET last_used_at = $2
		WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < $2::timestamptz - INTERVAL '1 hour')`, id, now)
	return err
}
