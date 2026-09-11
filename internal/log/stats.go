package log

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// StatsQuery keeps the canonical date candidate window separate from the
// completion interval displayed by a chart. Historical log_date values may
// differ from the viewer's local completion date, so neither replaces the other.
type StatsQuery struct {
	CandidateStart, CandidateEnd time.Time
	Start, End                   *time.Time
	ChoreID                      int64
}

// StatsLogs reads only analytics fields, without sorting or hydrating notes,
// titles and other unrelated log data. A chore chart can seek the existing
// household/chore/completed_at index instead of loading the whole household.
func (s *PostgresStore) StatsLogs(ctx context.Context, householdID int64, q StatsQuery) ([]ChoreLog, error) {
	args := []any{householdID, q.CandidateStart.Format(time.DateOnly), q.CandidateEnd.Format(time.DateOnly)}
	access, accessArgs := readAccessSQL(ctx, householdID, "l.chore_id", "l.household_id", 4)
	args = append(args, accessArgs...)
	where := `l.household_id=$1
 AND COALESCE(l.log_date,(l.completed_at AT TIME ZONE 'UTC')::date) >= $2::date
 AND COALESCE(l.log_date,(l.completed_at AT TIME ZONE 'UTC')::date) < $3::date` + access
	add := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	if q.ChoreID != 0 {
		where += " AND l.chore_id = " + add(q.ChoreID)
	}
	if q.Start != nil {
		where += " AND l.completed_at >= " + add(*q.Start)
	}
	if q.End != nil {
		where += " AND l.completed_at < " + add(*q.End)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT l.id,l.chore_id,l.user_id,l.completed_at,COALESCE(l.indicators,'[]'),l.volume_ml,l.indicator_volumes::text,l.duration_seconds
 FROM chore_logs l WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := []ChoreLog{}
	for rows.Next() {
		l := ChoreLog{HouseholdID: householdID}
		var indicators string
		var volumes sql.NullString
		var volume, duration sql.NullInt64
		if err := rows.Scan(&l.ID, &l.ChoreID, &l.UserID, &l.CompletedAt, &indicators, &volume, &volumes, &duration); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(indicators), &l.Indicators)
		if volumes.Valid {
			_ = json.Unmarshal([]byte(volumes.String), &l.IndicatorVolumes)
		}
		l.VolumeML, l.DurationSeconds = nullIntToPtr(volume), nullIntToPtr(duration)
		logs = append(logs, l)
	}
	return logs, rows.Err()
}
