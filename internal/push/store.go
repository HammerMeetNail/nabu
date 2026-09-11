package push

import (
	"context"
	"database/sql"
	"errors"
	"github.com/HammerMeetNail/nabu/internal/diagnostics"
	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	"sync"
)

type Subscription struct {
	Endpoint       string
	P256DH         string
	Auth           string
	SessionHash    string
	BindingID      string
	RegistrationID string
}

type Store interface {
	SaveSubscription(ctx context.Context, userID int64, sub Subscription) error
	GetSubscriptions(ctx context.Context, userID int64) ([]Subscription, error)
	DeleteSubscription(ctx context.Context, userID int64, endpoint, sessionHash string) error
	DeleteSubscriptionIfCurrent(ctx context.Context, userID int64, sub Subscription) error
	WithSubscription(ctx context.Context, userID int64, sub Subscription, fn func() error) error
}

// MemoryStore is an in-memory implementation for tests and the zero-DB fallback.
// The reminder scheduler goroutine reads subscriptions concurrently with HTTP
// handlers writing them, so all access is guarded by mu to avoid a data race
// (a concurrent map write is a fatal panic in Go).
type MemoryStore struct {
	deleted  lifecycle.Tombstones
	mu       sync.Mutex
	sessions lifecycle.SessionGuard
	data     map[int64][]Subscription
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: map[int64][]Subscription{}}
}

func (s *MemoryStore) BindSessions(guard lifecycle.SessionGuard) { s.sessions = guard }

func (s *MemoryStore) SaveSubscription(ctx context.Context, userID int64, sub Subscription) error {
	sub.RegistrationID = diagnostics.RequestID()
	save := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.deleted.Check(userID, 0, 0, 0, 0); err != nil {
			return err
		}
		for owner, subscriptions := range s.data {
			filtered := subscriptions[:0]
			for _, existing := range subscriptions {
				if existing.Endpoint != sub.Endpoint {
					filtered = append(filtered, existing)
				}
			}
			s.data[owner] = filtered
		}
		s.data[userID] = append(s.data[userID], sub)
		return nil
	}
	if s.sessions != nil {
		return s.sessions.WithSession(ctx, userID, sub.SessionHash, save)
	}
	return save()
}

func (s *MemoryStore) GetSubscriptions(ctx context.Context, userID int64) ([]Subscription, error) {
	s.mu.Lock()
	snapshot := append([]Subscription(nil), s.data[userID]...)
	s.mu.Unlock()
	if s.sessions == nil {
		return snapshot, nil
	}
	var current []Subscription
	for _, sub := range snapshot {
		err := s.sessions.WithSession(ctx, userID, sub.SessionHash, func() error {
			s.mu.Lock()
			defer s.mu.Unlock()
			for _, live := range s.data[userID] {
				if live == sub {
					current = append(current, sub)
					break
				}
			}
			return nil
		})
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			_ = s.DeleteSubscriptionIfCurrent(ctx, userID, sub)
		}
	}
	return current, nil
}

func (s *MemoryStore) DeleteSubscriptionIfCurrent(_ context.Context, userID int64, target Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.data[userID][:0]
	for _, sub := range s.data[userID] {
		if sub != target {
			filtered = append(filtered, sub)
		}
	}
	s.data[userID] = filtered
	return nil
}

func (s *MemoryStore) DeleteSubscription(_ context.Context, userID int64, endpoint, sessionHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.data[userID][:0]
	for _, sub := range s.data[userID] {
		if sub.Endpoint != endpoint || sub.SessionHash != sessionHash {
			filtered = append(filtered, sub)
		}
	}
	s.data[userID] = filtered
	return nil
}

// PostgresStore persists subscriptions in Postgres.
type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) SaveSubscription(ctx context.Context, userID int64, sub Subscription) error {
	return lifecycle.WithSQLSession(ctx, s.db, userID, sub.SessionHash, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth, session_hash, binding_id, registration_id)
 SELECT $1::bigint, $2, $3, $4, s.token_hash, $6, $7 FROM sessions s JOIN users u ON u.id=s.user_id
 WHERE s.token_hash=$5 AND s.user_id=$1 AND s.auth_version=u.auth_version
 AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-INTERVAL '24 hours'
 ON CONFLICT (endpoint) DO UPDATE SET user_id=EXCLUDED.user_id, p256dh=EXCLUDED.p256dh,
 auth=EXCLUDED.auth, session_hash=EXCLUDED.session_hash, binding_id=EXCLUDED.binding_id, registration_id=EXCLUDED.registration_id`,
			userID, sub.Endpoint, sub.P256DH, sub.Auth, sub.SessionHash, sub.BindingID, diagnostics.RequestID())
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err == nil && n != 1 {
			return errors.New("push session no longer active")
		}
		return err
	})
}

func (s *PostgresStore) DeleteSubscriptionIfCurrent(ctx context.Context, userID int64, sub Subscription) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE user_id=$1 AND endpoint=$2
 AND registration_id=$3`, userID, sub.Endpoint, sub.RegistrationID)
	return err
}

func (s *PostgresStore) GetSubscriptions(ctx context.Context, userID int64) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.endpoint, p.p256dh, p.auth, p.session_hash, p.binding_id, p.registration_id
 FROM push_subscriptions p JOIN sessions s ON s.token_hash=p.session_hash JOIN users u ON u.id=s.user_id
 WHERE p.user_id=$1 AND s.user_id=$1 AND s.auth_version=u.auth_version
 AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-INTERVAL '24 hours'`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Subscription
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.Endpoint, &sub.P256DH, &sub.Auth, &sub.SessionHash, &sub.BindingID, &sub.RegistrationID); err != nil {
			return nil, err
		}
		result = append(result, sub)
	}
	return result, rows.Err()
}

func (s *PostgresStore) DeleteSubscription(ctx context.Context, userID int64, endpoint, sessionHash string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2 AND session_hash = $3`,
		userID, endpoint, sessionHash,
	)
	return err
}

func (s *MemoryStore) CleanupAccount(d *lifecycle.Deletion) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, d.UserID)
	s.deleted.Mark(d)
}

func (s *MemoryStore) WithSubscription(ctx context.Context, userID int64, target Subscription, fn func() error) error {
	deliver := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, sub := range s.data[userID] {
			if sub == target {
				return fn()
			}
		}
		return errors.New("browser registration changed")
	}
	if s.sessions != nil {
		return s.sessions.WithSession(ctx, userID, target.SessionHash, deliver)
	}
	return deliver()
}

func (s *PostgresStore) WithSubscription(ctx context.Context, userID int64, sub Subscription, fn func() error) error {
	return lifecycle.WithSQLSession(ctx, s.db, userID, sub.SessionHash, func(tx *sql.Tx) error {
		var id string
		if err := tx.QueryRowContext(ctx, `SELECT registration_id FROM push_subscriptions WHERE user_id=$1
  AND endpoint=$2 AND session_hash=$3 AND registration_id=$4 FOR SHARE`, userID, sub.Endpoint, sub.SessionHash, sub.RegistrationID).Scan(&id); err != nil {
			return err
		}
		return fn()
	})
}
