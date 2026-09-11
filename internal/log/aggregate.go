package log

import (
	"context"
	"fmt"
	"time"
)

type AggregateGroup uint8

const (
	GroupUser AggregateGroup = iota
	GroupChore
	GroupCategory
	GroupLocalDate
	GroupWeekSplit
	GroupMemberTotals
)

// AggregateQuery preserves two distinct filters: the store's canonical date
// candidates and, when set, the viewer's completion-time interval. Attribution
// filters never replace the viewer permission context.
type AggregateQuery struct {
	MatchUser                    bool
	CandidateStart, CandidateEnd string
	Start, End                   *time.Time
	ChoreID, UserID              int64
	Group                        AggregateGroup
	TimeZone                     string
}

type AggregateRow struct {
	UserID, ChoreID          int64
	Category, Date           string
	Weekday                  int
	Count, TotalML, Duration int
}

// Aggregate returns grouped scalars only; notes, titles and indicator labels
// never leave PostgreSQL on this path. No client-provided expression is SQL.
func (s *PostgresStore) Aggregate(ctx context.Context, householdID int64, q AggregateQuery) ([]AggregateRow, error) {
	args := []any{householdID, q.CandidateStart, q.CandidateEnd}
	access, accessArgs := readAccessSQL(ctx, householdID, "l.chore_id", "l.household_id", 4)
	args = append(args, accessArgs...)
	where := `l.household_id=$1 AND COALESCE(l.log_date,(l.completed_at AT TIME ZONE 'UTC')::date) >= $2::date
	 AND COALESCE(l.log_date,(l.completed_at AT TIME ZONE 'UTC')::date) < $3::date` + access
	add := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	if q.Start != nil {
		where += " AND l.completed_at >= " + add(*q.Start)
	}
	if q.End != nil {
		where += " AND l.completed_at < " + add(*q.End)
	}
	if q.UserID > 0 || q.MatchUser {
		where += " AND l.user_id = " + add(q.UserID)
	}
	if q.ChoreID > 0 {
		where += " AND l.chore_id = " + add(q.ChoreID)
	}
	user, chore, category, date, weekday := "0", "0", "''", "''", "0"
	join, group, metrics := "", "", "0,0"
	switch q.Group {
	case GroupUser, GroupMemberTotals:
		user, group = "l.user_id", "l.user_id"
	case GroupChore:
		chore, group = "l.chore_id", "l.chore_id"
	case GroupCategory:
		category = "COALESCE(NULLIF(c.category,''),'custom')"
		join, group = " JOIN chores c ON c.id=l.chore_id AND c.household_id=l.household_id", category
	case GroupLocalDate:
		date = "to_char(l.completed_at AT TIME ZONE " + add(q.TimeZone) + ",'YYYY-MM-DD')"
		group = date
	case GroupWeekSplit:
		user, category = "l.user_id", "COALESCE(NULLIF(c.category,''),'custom')"
		weekday = "EXTRACT(DOW FROM l.completed_at AT TIME ZONE " + add(q.TimeZone) + ")::int"
		join = " JOIN chores c ON c.id=l.chore_id AND c.household_id=l.household_id"
		group = user + "," + category + "," + weekday
	default:
		return nil, fmt.Errorf("invalid aggregate group")
	}
	if q.Group == GroupMemberTotals {
		// Sum metrics per log before grouping. Expanding a JSON object as joined
		// rows would multiply log counts and durations by its number of entries.
		metrics = `COALESCE(SUM(CASE WHEN jsonb_typeof(l.indicator_volumes)='object' AND l.indicator_volumes <> '{}'::jsonb
		 THEN (SELECT COALESCE(SUM(GREATEST(value::bigint,0)),0) FROM jsonb_each_text(l.indicator_volumes)
		       WHERE value ~ '^-?[0-9]{1,18}$') ELSE GREATEST(COALESCE(l.volume_ml,0),0) END),0),
		 COALESCE(SUM(l.duration_seconds),0)`
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+user+","+chore+","+category+","+date+","+weekday+",COUNT(*),"+metrics+
		" FROM chore_logs l"+join+" WHERE "+where+" GROUP BY "+group, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AggregateRow{}
	for rows.Next() {
		var row AggregateRow
		if err := rows.Scan(&row.UserID, &row.ChoreID, &row.Category, &row.Date, &row.Weekday, &row.Count, &row.TotalML, &row.Duration); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
