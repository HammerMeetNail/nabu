package chore

import (
	"context"
	"database/sql"
	"encoding/json"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) CreateChore(ctx context.Context, chore Chore) (Chore, error) {
	labels, _ := json.Marshal(nilToEmpty(chore.IndicatorLabels))
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO chores (household_id, name, icon, color, sort_order, category, is_predefined, predefined_key, created_by, indicator_labels)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id, created_at
	`, chore.HouseholdID, chore.Name, chore.Icon, chore.Color, chore.SortOrder, chore.Category, chore.IsPredefined, nullableString(chore.PredefinedKey), chore.CreatedBy, string(labels)).Scan(&chore.ID, &chore.CreatedAt)
	return chore, err
}

func (s *PostgresStore) GetChore(ctx context.Context, id int64) (Chore, error) {
	var c Chore
	var labelsJSON string
	err := s.db.QueryRowContext(ctx, `SELECT id, household_id, name, icon, color, sort_order, category, is_predefined, COALESCE(predefined_key,''), created_by, created_at, indicator_labels FROM chores WHERE id = $1`, id).Scan(&c.ID, &c.HouseholdID, &c.Name, &c.Icon, &c.Color, &c.SortOrder, &c.Category, &c.IsPredefined, &c.PredefinedKey, &c.CreatedBy, &c.CreatedAt, &labelsJSON)
	if err == sql.ErrNoRows {
		return Chore{}, ErrNotFound
	}
	if err == nil {
		_ = json.Unmarshal([]byte(labelsJSON), &c.IndicatorLabels)
		if c.IndicatorLabels == nil {
			c.IndicatorLabels = []string{}
		}
	}
	return c, err
}

func (s *PostgresStore) ListChores(ctx context.Context, householdID int64) ([]Chore, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, household_id, name, icon, color, sort_order, category, is_predefined, COALESCE(predefined_key,''), created_by, created_at, indicator_labels FROM chores WHERE household_id = $1 ORDER BY sort_order`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chores []Chore
	for rows.Next() {
		var c Chore
		var labelsJSON string
		if err := rows.Scan(&c.ID, &c.HouseholdID, &c.Name, &c.Icon, &c.Color, &c.SortOrder, &c.Category, &c.IsPredefined, &c.PredefinedKey, &c.CreatedBy, &c.CreatedAt, &labelsJSON); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(labelsJSON), &c.IndicatorLabels)
		if c.IndicatorLabels == nil {
			c.IndicatorLabels = []string{}
		}
		chores = append(chores, c)
	}
	return chores, rows.Err()
}

func (s *PostgresStore) UpdateChore(ctx context.Context, chore Chore) error {
	labels, _ := json.Marshal(nilToEmpty(chore.IndicatorLabels))
	_, err := s.db.ExecContext(ctx, `UPDATE chores SET name=$1, icon=$2, color=$3, category=$4, indicator_labels=$5 WHERE id=$6`, chore.Name, chore.Icon, chore.Color, chore.Category, string(labels), chore.ID)
	return err
}

func (s *PostgresStore) DeleteChore(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chores WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) ReorderChores(ctx context.Context, householdID int64, choreIDs []int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE chores SET sort_order = v.ord
		FROM (SELECT UNNEST($1::bigint[]) AS id, GENERATE_SERIES(0, $2) AS ord) v
		WHERE chores.id = v.id AND chores.household_id = $3
	`, choreIDs, len(choreIDs)-1, householdID)
	return err
}

func (s *PostgresStore) SeedPredefinedChores(ctx context.Context, householdID int64) error {
	for _, pc := range PredefinedChores {
		labels, _ := json.Marshal(nilToEmpty(pc.IndicatorLabels))
		if _, err := s.db.ExecContext(ctx, `INSERT INTO chores (household_id, name, icon, color, sort_order, category, is_predefined, predefined_key, indicator_labels) VALUES ($1,$2,$3,$4,$5,$6,TRUE,$7,$8) ON CONFLICT (household_id, name) DO UPDATE SET predefined_key = EXCLUDED.predefined_key`,
			householdID, pc.Name, pc.Icon, pc.Color, pc.SortOrder, pc.Category, pc.Name, string(labels)); err != nil {
			return err
		}
	}
	return nil
}

func nilToEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
