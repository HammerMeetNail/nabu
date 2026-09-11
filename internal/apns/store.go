// Package apns delivers native iOS push notifications through Apple's APNs
// HTTP/2 API and manages the device tokens the app registers.
package apns

import (
	"context"
	"database/sql"
	"errors"
	"github.com/HammerMeetNail/nabu/internal/diagnostics"
	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	"sync"
)

const (
	EnvironmentSandbox    = "sandbox"
	EnvironmentProduction = "production"
)

type Device struct {
	UserID         int64
	Token          string
	Environment    string // "sandbox" | "production"
	BundleID       string
	DeviceName     string
	SessionHash    string
	RegistrationID string
}

type Store interface {
	// RegisterDevice upserts by token: a device token is unique to a physical
	// device, so registration by a different user takes the token over.
	RegisterDevice(ctx context.Context, d Device) error
	// UnregisterDevice removes the token if it belongs to userID.
	UnregisterDevice(ctx context.Context, userID int64, token, sessionHash string) error
	DevicesForUser(ctx context.Context, userID int64) ([]Device, error)
	// DeleteToken removes a token regardless of owner — used when APNs
	// reports it terminally invalid (Unregistered / BadDeviceToken).
	DeleteToken(ctx context.Context, token string) error
	DeleteDeviceIfCurrent(ctx context.Context, d Device) error
	WithDevice(ctx context.Context, d Device, fn func() error) error
}

// MemoryStore backs tests and the zero-DB development mode. The sender reads
// devices from scheduler goroutines while HTTP handlers write them, so all
// access is mutex-guarded.
type MemoryStore struct {
	deleted  lifecycle.Tombstones
	mu       sync.Mutex
	sessions lifecycle.SessionGuard
	byToken  map[string]Device
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{byToken: map[string]Device{}}
}

func (s *MemoryStore) BindSessions(guard lifecycle.SessionGuard) { s.sessions = guard }
func (s *MemoryStore) RegisterDevice(ctx context.Context, d Device) error {
	d.RegistrationID = diagnostics.RequestID()
	save := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.deleted.Check(d.UserID, 0, 0, 0, 0); err != nil {
			return err
		}
		s.byToken[d.Token] = d
		return nil
	}
	if s.sessions != nil {
		return s.sessions.WithSession(ctx, d.UserID, d.SessionHash, save)
	}
	return save()
}
func (s *MemoryStore) DeleteDeviceIfCurrent(_ context.Context, d Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byToken[d.Token] == d {
		delete(s.byToken, d.Token)
	}
	return nil
}

func (s *MemoryStore) UnregisterDevice(_ context.Context, userID int64, token, sessionHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.byToken[token]; ok && d.UserID == userID && d.SessionHash == sessionHash {
		delete(s.byToken, token)
	}
	return nil
}

func (s *MemoryStore) DevicesForUser(ctx context.Context, userID int64) ([]Device, error) {
	s.mu.Lock()
	var snapshot []Device
	for _, d := range s.byToken {
		if d.UserID == userID {
			snapshot = append(snapshot, d)
		}
	}
	s.mu.Unlock()
	if s.sessions == nil {
		return snapshot, nil
	}
	var current []Device
	for _, d := range snapshot {
		err := s.sessions.WithSession(ctx, userID, d.SessionHash, func() error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.byToken[d.Token] == d {
				current = append(current, d)
			}
			return nil
		})
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			_ = s.DeleteDeviceIfCurrent(ctx, d)
		}
	}
	return current, nil
}

func (s *MemoryStore) DeleteToken(_ context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byToken, token)
	return nil
}

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) RegisterDevice(ctx context.Context, d Device) error {
	return lifecycle.WithSQLSession(ctx, s.db, d.UserID, d.SessionHash, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `INSERT INTO mobile_device_tokens
 (user_id, token, environment, bundle_id, device_name, session_hash, registration_id, created_at, last_seen_at)
 SELECT $1::bigint, $2, $3, $4, $5, s.token_hash, $7, NOW(), NOW() FROM sessions s JOIN users u ON u.id=s.user_id
 WHERE s.token_hash=$6 AND s.user_id=$1 AND s.auth_version=u.auth_version
 AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-INTERVAL '24 hours'
 ON CONFLICT (token) DO UPDATE SET user_id=EXCLUDED.user_id, environment=EXCLUDED.environment,
 bundle_id=EXCLUDED.bundle_id, device_name=EXCLUDED.device_name, session_hash=EXCLUDED.session_hash, registration_id=EXCLUDED.registration_id, last_seen_at=NOW()`,
			d.UserID, d.Token, d.Environment, d.BundleID, d.DeviceName, d.SessionHash, diagnostics.RequestID())
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err == nil && n != 1 {
			return errors.New("device session no longer active")
		}
		return err
	})
}

func (s *PostgresStore) DeleteDeviceIfCurrent(ctx context.Context, d Device) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM mobile_device_tokens WHERE user_id=$1 AND token=$2
 AND registration_id=$3`, d.UserID, d.Token, d.RegistrationID)
	return err
}

func (s *PostgresStore) UnregisterDevice(ctx context.Context, userID int64, token, sessionHash string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM mobile_device_tokens WHERE user_id = $1 AND token = $2 AND session_hash=$3`, userID, token, sessionHash)
	return err
}

func (s *PostgresStore) DevicesForUser(ctx context.Context, userID int64) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.user_id, d.token, d.environment, d.bundle_id, d.device_name, d.session_hash, d.registration_id
 FROM mobile_device_tokens d JOIN sessions s ON s.token_hash=d.session_hash JOIN users u ON u.id=s.user_id
 WHERE d.user_id=$1 AND s.user_id=$1 AND s.auth_version=u.auth_version
 AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-INTERVAL '24 hours'
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.UserID, &d.Token, &d.Environment, &d.BundleID, &d.DeviceName, &d.SessionHash, &d.RegistrationID); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *PostgresStore) DeleteToken(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM mobile_device_tokens WHERE token = $1`, token)
	return err
}

func (s *MemoryStore) CleanupAccount(d *lifecycle.Deletion) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, device := range s.byToken {
		if device.UserID == d.UserID {
			delete(s.byToken, token)
		}
	}
	s.deleted.Mark(d)
}

func (s *MemoryStore) WithDevice(ctx context.Context, target Device, fn func() error) error {
	deliver := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.byToken[target.Token] != target {
			return errors.New("device registration changed")
		}
		return fn()
	}
	if s.sessions != nil {
		return s.sessions.WithSession(ctx, target.UserID, target.SessionHash, deliver)
	}
	return deliver()
}

func (s *PostgresStore) WithDevice(ctx context.Context, d Device, fn func() error) error {
	return lifecycle.WithSQLSession(ctx, s.db, d.UserID, d.SessionHash, func(tx *sql.Tx) error {
		var id string
		if err := tx.QueryRowContext(ctx, `SELECT registration_id FROM mobile_device_tokens WHERE user_id=$1
  AND token=$2 AND session_hash=$3 AND registration_id=$4 FOR SHARE`, d.UserID, d.Token, d.SessionHash, d.RegistrationID).Scan(&id); err != nil {
			return err
		}
		return fn()
	})
}
