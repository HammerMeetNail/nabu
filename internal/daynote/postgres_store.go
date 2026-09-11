package daynote

import (
	"context"
	"database/sql"
	"github.com/HammerMeetNail/nabu/internal/readlimit"
	"time"
)

type postgresStore struct {
	db *sql.DB
}

// NewPostgresStore returns a Store backed by PostgreSQL.
func NewPostgresStore(db *sql.DB) Store {
	return &postgresStore{db: db}
}

func (s *postgresStore) ListRange(ctx context.Context, householdID int64, start, end time.Time) ([]DayNote, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT to_char(note_date, 'YYYY-MM-DD'), note, updated_by, updated_at
		FROM day_notes
		WHERE household_id = $1 AND note_date >= $2::date AND note_date < $3::date
		ORDER BY note_date DESC`+readlimit.SQL(ctx),
		householdID, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DayNote
	for rows.Next() {
		var n DayNote
		var updatedBy sql.NullInt64
		if err := rows.Scan(&n.Date, &n.Note, &updatedBy, &n.UpdatedAt); err != nil {
			return nil, err
		}
		if updatedBy.Valid {
			v := updatedBy.Int64
			n.UpdatedBy = &v
		}
		out = append(out, n)
	}
	if err := readlimit.Check(ctx, len(out)); err != nil {
		return nil, err
	}
	return out, rows.Err()
}

func (s *postgresStore) Upsert(ctx context.Context, householdID int64, date, note string, updatedBy int64) (DayNote, error) {
	if note == "" {
		_, err := s.db.ExecContext(ctx,
			`DELETE FROM day_notes WHERE household_id = $1 AND note_date = $2::date`,
			householdID, date)
		return DayNote{Date: date, Note: ""}, err
	}
	var n DayNote
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO day_notes (household_id, note_date, note, updated_by, updated_at)
		VALUES ($1, $2::date, $3, $4, NOW())
		ON CONFLICT (household_id, note_date)
		DO UPDATE SET note = EXCLUDED.note, updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at
		RETURNING to_char(note_date, 'YYYY-MM-DD'), note, updated_by, updated_at`,
		householdID, date, note, updatedBy,
	).Scan(&n.Date, &n.Note, &updatedBy, &n.UpdatedAt)
	if err != nil {
		return DayNote{}, err
	}
	uid := updatedBy
	n.UpdatedBy = &uid
	return n, nil
}
