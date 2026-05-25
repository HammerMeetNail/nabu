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
	var rawOrder []byte
	var rawHidden []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT chore_order, hidden_home_chore_ids FROM user_preferences WHERE user_id = $1`,
		userID,
	).Scan(&rawOrder, &rawHidden)
	if err == sql.ErrNoRows {
		return Preferences{ChoreOrder: []int64{}, HiddenHomeChoreIDs: []int64{}}, nil
	}
	if err != nil {
		return Preferences{}, err
	}

	var order []int64
	if err := json.Unmarshal(rawOrder, &order); err != nil {
		return Preferences{}, err
	}
	if order == nil {
		order = []int64{}
	}

	var hidden []int64
	if err := json.Unmarshal(rawHidden, &hidden); err != nil {
		return Preferences{}, err
	}
	if hidden == nil {
		hidden = []int64{}
	}

	return Preferences{ChoreOrder: order, HiddenHomeChoreIDs: hidden}, nil
}

func (s *postgresStore) Upsert(ctx context.Context, userID int64, p Preferences) error {
	order := p.ChoreOrder
	if order == nil {
		order = []int64{}
	}
	hidden := p.HiddenHomeChoreIDs
	if hidden == nil {
		hidden = []int64{}
	}
	rawOrder, err := json.Marshal(order)
	if err != nil {
		return err
	}
	rawHidden, err := json.Marshal(hidden)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO user_preferences (user_id, chore_order, hidden_home_chore_ids, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id)
		DO UPDATE SET chore_order           = EXCLUDED.chore_order,
		              hidden_home_chore_ids = EXCLUDED.hidden_home_chore_ids,
		              updated_at            = EXCLUDED.updated_at`,
		userID, rawOrder, rawHidden,
	)
	return err
}
