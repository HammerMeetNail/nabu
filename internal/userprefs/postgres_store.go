package userprefs

import (
	"context"
	"database/sql"
	"encoding/json"
)

type postgresStore struct {
	db *sql.DB
}

// NewPostgresStore returns a Store backed by a PostgreSQL database.
func NewPostgresStore(db *sql.DB) Store {
	return &postgresStore{db: db}
}

func (s *postgresStore) Get(ctx context.Context, userID int64) (Preferences, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT chore_order FROM user_preferences WHERE user_id = $1`,
		userID,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return Preferences{ChoreOrder: []int64{}}, nil
	}
	if err != nil {
		return Preferences{}, err
	}

	var order []int64
	if err := json.Unmarshal(raw, &order); err != nil {
		return Preferences{}, err
	}
	if order == nil {
		order = []int64{}
	}
	return Preferences{ChoreOrder: order}, nil
}

func (s *postgresStore) Upsert(ctx context.Context, userID int64, p Preferences) error {
	order := p.ChoreOrder
	if order == nil {
		order = []int64{}
	}
	raw, err := json.Marshal(order)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO user_preferences (user_id, chore_order, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id)
		DO UPDATE SET chore_order = EXCLUDED.chore_order,
		              updated_at  = EXCLUDED.updated_at`,
		userID, raw,
	)
	return err
}
