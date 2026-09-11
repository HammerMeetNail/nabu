package log

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/HammerMeetNail/nabu/internal/chore"
)

// RecentAmounts is a viewer-authorized source independent of Activity pages,
// search results, the current calendar date, and the latest log's metric.
func (s *Service) RecentAmounts(ctx context.Context, actorID, householdID, choreID int64, units ...string) ([]int, error) {
	if s.chores == nil || s.memberships == nil || actorID <= 0 {
		return nil, ErrNotFound
	}
	c, err := s.chores.GetVisible(ctx, actorID, householdID, choreID)
	if err != nil {
		if errors.Is(err, chore.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	unit := c.MetricUnit
	if len(units) > 0 && units[0] != "" {
		unit = units[0]
		if err := validateUnit(unit); err != nil {
			return nil, err
		}
	}
	return s.store.RecentAmounts(WithReadAccess(ctx, householdID, actorID, map[int64]struct{}{choreID: {}}), householdID, choreID, unit)
}

func (s *MemoryStore) RecentAmounts(ctx context.Context, householdID, choreID int64, units ...string) ([]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	logs := []ChoreLog{}
	for _, entry := range s.logs {
		if readAllowed(ctx, householdID, entry.ChoreID) && entry.HouseholdID == householdID && entry.ChoreID == choreID {
			logs = append(logs, entry)
		}
	}
	sort.Slice(logs, func(i, j int) bool {
		if logs[i].CompletedAt.Equal(logs[j].CompletedAt) {
			return logs[i].ID > logs[j].ID
		}
		return logs[i].CompletedAt.After(logs[j].CompletedAt)
	})
	out, seen := []int{}, map[int]bool{}
	for _, entry := range logs {
		if len(units) > 0 && AmountUnit(entry.MetricUnit) != AmountUnit(units[0]) {
			continue
		}
		values := []int{}
		if entry.VolumeML != nil {
			values = append(values, *entry.VolumeML)
		}
		labels := []string{}
		for label := range entry.IndicatorVolumes {
			labels = append(labels, label)
		}
		sort.Strings(labels)
		for _, label := range labels {
			values = append(values, entry.IndicatorVolumes[label])
		}
		for _, amount := range values {
			if amount > 0 && amount <= maxVolumeML && !seen[amount] {
				out = append(out, amount)
				seen[amount] = true
				if len(out) == 3 {
					return out, nil
				}
			}
		}
	}
	return out, nil
}

func (s *PostgresStore) RecentAmounts(ctx context.Context, householdID, choreID int64, units ...string) ([]int, error) {
	out := []int{}
	unitFilter := ""
	if len(units) > 0 {
		unitFilter = " AND (CASE WHEN lower(trim(l.metric_unit)) IN ('','ml','oz') THEN 'mL' ELSE l.metric_unit END) = $4"
	}
	firstArg := 4
	if len(units) > 0 {
		firstArg = 5
	}
	access, accessArgs := readAccessSQL(ctx, householdID, "l.chore_id", "l.household_id", firstArg)
	// At most three bounded results. The household/chore/time index walks newest
	// first, skipping values already selected without fetching log text/metadata.
	for len(out) < 3 {
		args := []any{householdID, choreID, out}
		if len(units) > 0 {
			args = append(args, AmountUnit(units[0]))
		}
		var amount int
		err := s.db.QueryRowContext(ctx, `
			SELECT a.amount FROM chore_logs l
			CROSS JOIN LATERAL (
				SELECT l.volume_ml AS amount, 0 AS priority, '' AS label
				UNION ALL
				SELECT CASE WHEN value ~ '^[0-9]{1,6}$' THEN value::int END, 1, key
				FROM jsonb_each_text(CASE WHEN jsonb_typeof(l.indicator_volumes) = 'object' THEN l.indicator_volumes ELSE '{}'::jsonb END)
			) a
			WHERE l.household_id=$1 AND l.chore_id=$2 AND a.amount > 0 AND a.amount <= 100000
			  AND NOT (a.amount = ANY($3::int[]))`+unitFilter+access+`
			ORDER BY l.completed_at DESC,l.id DESC,a.priority,a.label
			LIMIT 1`, append(args, accessArgs...)...).Scan(&amount)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, amount)
	}
	return out, nil
}
