package chore

import (
	"context"
	"database/sql"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) CreateChore(ctx context.Context, chore Chore) (Chore, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO chores (household_id, name, icon, color, sort_order, category, is_predefined, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at
	`, chore.HouseholdID, chore.Name, chore.Icon, chore.Color, chore.SortOrder, chore.Category, chore.IsPredefined, chore.CreatedBy).Scan(&chore.ID, &chore.CreatedAt)
	return chore, err
}

func (s *PostgresStore) GetChore(ctx context.Context, id int64) (Chore, error) {
	var c Chore
	err := s.db.QueryRowContext(ctx, `SELECT id, household_id, name, icon, color, sort_order, category, is_predefined, created_by, created_at FROM chores WHERE id = $1`, id).Scan(&c.ID, &c.HouseholdID, &c.Name, &c.Icon, &c.Color, &c.SortOrder, &c.Category, &c.IsPredefined, &c.CreatedBy, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return Chore{}, ErrNotFound
	}
	return c, err
}

func (s *PostgresStore) ListChores(ctx context.Context, householdID int64) ([]Chore, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, household_id, name, icon, color, sort_order, category, is_predefined, created_by, created_at FROM chores WHERE household_id = $1 ORDER BY sort_order`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chores []Chore
	for rows.Next() {
		var c Chore
		if err := rows.Scan(&c.ID, &c.HouseholdID, &c.Name, &c.Icon, &c.Color, &c.SortOrder, &c.Category, &c.IsPredefined, &c.CreatedBy, &c.CreatedAt); err != nil {
			return nil, err
		}
		chores = append(chores, c)
	}
	return chores, rows.Err()
}

func (s *PostgresStore) UpdateChore(ctx context.Context, chore Chore) error {
	_, err := s.db.ExecContext(ctx, `UPDATE chores SET name=$1, icon=$2, color=$3, category=$4 WHERE id=$5`, chore.Name, chore.Icon, chore.Color, chore.Category, chore.ID)
	return err
}

func (s *PostgresStore) DeleteChore(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chores WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) ReorderChores(ctx context.Context, householdID int64, choreIDs []int64) error {
	for i, id := range choreIDs {
		if _, err := s.db.ExecContext(ctx, `UPDATE chores SET sort_order = $1 WHERE id = $2 AND household_id = $3`, i, id, householdID); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) SeedPredefinedChores(ctx context.Context, householdID int64) error {
	for _, pc := range PredefinedChores {
		s.db.ExecContext(ctx, `INSERT INTO chores (household_id, name, icon, color, sort_order, category, is_predefined) VALUES ($1,$2,$3,$4,$5,$6,TRUE) ON CONFLICT (household_id, name) DO NOTHING`,
			householdID, pc.Name, pc.Icon, pc.Color, pc.SortOrder, pc.Category)
	}
	return nil
}
