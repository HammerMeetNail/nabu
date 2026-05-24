package push

import (
	"context"
	"database/sql"
)

type Subscription struct {
	Endpoint string
	P256DH   string
	Auth     string
}

type Store interface {
	SaveSubscription(ctx context.Context, userID int64, sub Subscription) error
	GetSubscriptions(ctx context.Context, userID int64) ([]Subscription, error)
	DeleteSubscription(ctx context.Context, userID int64, endpoint string) error
}

// MemoryStore is an in-memory implementation for tests and the zero-DB fallback.
type MemoryStore struct {
	data map[int64][]Subscription
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: map[int64][]Subscription{}}
}

func (s *MemoryStore) SaveSubscription(_ context.Context, userID int64, sub Subscription) error {
	for i, existing := range s.data[userID] {
		if existing.Endpoint == sub.Endpoint {
			s.data[userID][i] = sub
			return nil
		}
	}
	s.data[userID] = append(s.data[userID], sub)
	return nil
}

func (s *MemoryStore) GetSubscriptions(_ context.Context, userID int64) ([]Subscription, error) {
	return append([]Subscription(nil), s.data[userID]...), nil
}

func (s *MemoryStore) DeleteSubscription(_ context.Context, userID int64, endpoint string) error {
	filtered := s.data[userID][:0]
	for _, sub := range s.data[userID] {
		if sub.Endpoint != endpoint {
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
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, endpoint) DO UPDATE
		 SET p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth`,
		userID, sub.Endpoint, sub.P256DH, sub.Auth,
	)
	return err
}

func (s *PostgresStore) GetSubscriptions(ctx context.Context, userID int64) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT endpoint, p256dh, auth FROM push_subscriptions WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Subscription
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.Endpoint, &sub.P256DH, &sub.Auth); err != nil {
			return nil, err
		}
		result = append(result, sub)
	}
	return result, rows.Err()
}

func (s *PostgresStore) DeleteSubscription(ctx context.Context, userID int64, endpoint string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2`,
		userID, endpoint,
	)
	return err
}
