package chore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/HammerMeetNail/nabu/internal/readlimit"
	"github.com/jackc/pgx/v5/pgconn"
)

// isDuplicateName reports a name the household already gives another chore:
// chores are UNIQUE (household_id, name).
func isDuplicateName(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) CreateChore(ctx context.Context, chore Chore) (Chore, error) {
	labels, _ := json.Marshal(nilToEmpty(chore.IndicatorLabels))
	defaults, _ := json.Marshal(nilToEmpty(chore.IndicatorDefaults))
	chore.NormalizeVisibility()
	chore.NormalizeMetric()
	subjects, _ := json.Marshal(nilToEmpty(chore.Subjects))
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO chores (household_id, name, icon, color, sort_order, category, is_predefined, predefined_key, created_by, indicator_labels, has_volume_ml, indicator_defaults, follow_up_enabled, last_follow_up_minutes, has_rating, metric_type, metric_unit, subjects, visibility)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) RETURNING id, created_at
	`, chore.HouseholdID, chore.Name, chore.Icon, chore.Color, chore.SortOrder, chore.Category, chore.IsPredefined, nullableString(chore.PredefinedKey), chore.CreatedBy, string(labels), chore.HasVolumeML, string(defaults), chore.FollowUpEnabled, chore.LastFollowUpMinutes, chore.HasRating, chore.MetricType, chore.MetricUnit, string(subjects), chore.Visibility).Scan(&chore.ID, &chore.CreatedAt)
	if isDuplicateName(err) {
		return Chore{}, ErrDuplicateName
	}
	return chore, err
}

func (s *PostgresStore) GetChore(ctx context.Context, id int64) (Chore, error) {
	var c Chore
	var labelsJSON string
	var defaultsJSON string
	var subjectsJSON string
	err := s.db.QueryRowContext(ctx, `SELECT id, household_id, name, icon, color, sort_order, category, is_predefined, COALESCE(predefined_key,''), created_by, created_at, indicator_labels, has_volume_ml, COALESCE(indicator_defaults,'[]'), follow_up_enabled, last_follow_up_minutes, has_rating, COALESCE(metric_type,'none'), COALESCE(metric_unit,''), COALESCE(subjects,'[]'), COALESCE(visibility,'household'), COALESCE(default_name,''), COALESCE(template_key,''), COALESCE(named_for,'') FROM chores WHERE id = $1`, id).Scan(&c.ID, &c.HouseholdID, &c.Name, &c.Icon, &c.Color, &c.SortOrder, &c.Category, &c.IsPredefined, &c.PredefinedKey, &c.CreatedBy, &c.CreatedAt, &labelsJSON, &c.HasVolumeML, &defaultsJSON, &c.FollowUpEnabled, &c.LastFollowUpMinutes, &c.HasRating, &c.MetricType, &c.MetricUnit, &subjectsJSON, &c.Visibility, &c.DefaultName, &c.TemplateKey, &c.NamedFor)
	if err == sql.ErrNoRows {
		return Chore{}, ErrNotFound
	}
	if err == nil {
		_ = json.Unmarshal([]byte(labelsJSON), &c.IndicatorLabels)
		if c.IndicatorLabels == nil {
			c.IndicatorLabels = []string{}
		}
		_ = json.Unmarshal([]byte(defaultsJSON), &c.IndicatorDefaults)
		if c.IndicatorDefaults == nil {
			c.IndicatorDefaults = []string{}
		}
		_ = json.Unmarshal([]byte(subjectsJSON), &c.Subjects)
		if c.Subjects == nil {
			c.Subjects = []string{}
		}
		c.NormalizeVisibility()
		c.NormalizeMetric()
	}
	return c, err
}

func (s *PostgresStore) ListChores(ctx context.Context, householdID int64) ([]Chore, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, household_id, name, icon, color, sort_order, category, is_predefined, COALESCE(predefined_key,''), created_by, created_at, indicator_labels, has_volume_ml, COALESCE(indicator_defaults,'[]'), follow_up_enabled, last_follow_up_minutes, has_rating, COALESCE(metric_type,'none'), COALESCE(metric_unit,''), COALESCE(subjects,'[]'), COALESCE(visibility,'household'), COALESCE(default_name,''), COALESCE(template_key,''), COALESCE(named_for,'') FROM chores WHERE household_id = $1 ORDER BY sort_order`+readlimit.SQL(ctx), householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chores []Chore
	for rows.Next() {
		var c Chore
		var labelsJSON string
		var defaultsJSON string
		var subjectsJSON string
		if err := rows.Scan(&c.ID, &c.HouseholdID, &c.Name, &c.Icon, &c.Color, &c.SortOrder, &c.Category, &c.IsPredefined, &c.PredefinedKey, &c.CreatedBy, &c.CreatedAt, &labelsJSON, &c.HasVolumeML, &defaultsJSON, &c.FollowUpEnabled, &c.LastFollowUpMinutes, &c.HasRating, &c.MetricType, &c.MetricUnit, &subjectsJSON, &c.Visibility, &c.DefaultName, &c.TemplateKey, &c.NamedFor); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(labelsJSON), &c.IndicatorLabels)
		if c.IndicatorLabels == nil {
			c.IndicatorLabels = []string{}
		}
		_ = json.Unmarshal([]byte(defaultsJSON), &c.IndicatorDefaults)
		if c.IndicatorDefaults == nil {
			c.IndicatorDefaults = []string{}
		}
		_ = json.Unmarshal([]byte(subjectsJSON), &c.Subjects)
		if c.Subjects == nil {
			c.Subjects = []string{}
		}
		c.NormalizeVisibility()
		c.NormalizeMetric()
		chores = append(chores, c)
	}
	if err := readlimit.Check(ctx, len(chores)); err != nil {
		return nil, err
	}
	return chores, rows.Err()
}

func (s *PostgresStore) UpdateChore(ctx context.Context, chore Chore) error {
	labels, _ := json.Marshal(nilToEmpty(chore.IndicatorLabels))
	defaults, _ := json.Marshal(nilToEmpty(chore.IndicatorDefaults))
	chore.NormalizeVisibility()
	chore.NormalizeMetric()
	subjects, _ := json.Marshal(nilToEmpty(chore.Subjects))
	_, err := s.db.ExecContext(ctx, `UPDATE chores SET name=$1, icon=$2, color=$3, category=$4, indicator_labels=$5, indicator_defaults=$6, follow_up_enabled=$7, last_follow_up_minutes=$8, has_volume_ml=$9, has_rating=$10, metric_type=$11, metric_unit=$12, subjects=$13, visibility=$14 WHERE id=$15`, chore.Name, chore.Icon, chore.Color, chore.Category, string(labels), string(defaults), chore.FollowUpEnabled, chore.LastFollowUpMinutes, chore.HasVolumeML, chore.HasRating, chore.MetricType, chore.MetricUnit, string(subjects), chore.Visibility, chore.ID)
	if isDuplicateName(err) {
		return ErrDuplicateName
	}
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
		pc.NormalizeVisibility()
		pc.NormalizeMetric()
		labels, _ := json.Marshal(nilToEmpty(pc.IndicatorLabels))
		defaults, _ := json.Marshal(nilToEmpty(pc.IndicatorDefaults))
		if _, err := s.db.ExecContext(ctx, `INSERT INTO chores (household_id, name, icon, color, sort_order, category, is_predefined, predefined_key, indicator_labels, has_volume_ml, indicator_defaults, has_rating, metric_type, metric_unit, visibility) VALUES ($1,$2,$3,$4,$5,$6,TRUE,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT (household_id, name) DO UPDATE SET predefined_key = EXCLUDED.predefined_key, indicator_defaults = EXCLUDED.indicator_defaults`,
			householdID, pc.Name, pc.Icon, pc.Color, pc.SortOrder, pc.Category, pc.Name, string(labels), pc.HasVolumeML, string(defaults), pc.HasRating, pc.MetricType, pc.MetricUnit, pc.Visibility); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) AddPackChores(ctx context.Context, householdID int64, chores []Chore) ([]Chore, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	added := []Chore{}
	for _, c := range chores {
		c.HouseholdID = householdID
		c.NormalizeVisibility()
		c.NormalizeMetric()
		labels, _ := json.Marshal(nilToEmpty(c.IndicatorLabels))
		defaults, _ := json.Marshal(nilToEmpty(c.IndicatorDefaults))
		subjects, _ := json.Marshal(nilToEmpty(c.Subjects))
		err := tx.QueryRowContext(ctx, `
			INSERT INTO chores (household_id, name, icon, color, sort_order, category, is_predefined, predefined_key, default_name, template_key, named_for, indicator_labels, has_volume_ml, indicator_defaults, follow_up_enabled, has_rating, metric_type, metric_unit, subjects, visibility)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
			ON CONFLICT (household_id, name) DO NOTHING RETURNING id, created_at
		`, c.HouseholdID, c.Name, c.Icon, c.Color, c.SortOrder, c.Category, c.IsPredefined, nullableString(c.PredefinedKey), nullableString(c.DefaultName), nullableString(c.TemplateKey), nullableString(c.NamedFor), string(labels), c.HasVolumeML, string(defaults), c.FollowUpEnabled, c.HasRating, c.MetricType, c.MetricUnit, string(subjects), c.Visibility).Scan(&c.ID, &c.CreatedAt)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		added = append(added, c)
	}
	return added, tx.Commit()
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
