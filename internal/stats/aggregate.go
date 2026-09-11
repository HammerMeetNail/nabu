package stats

import (
	"context"
	"sort"
	"time"

	"github.com/HammerMeetNail/nabu/internal/log"
)

type aggregateReader interface {
	Aggregate(context.Context, int64, log.AggregateQuery) ([]log.AggregateRow, error)
}

func aggregateWindow(start, end time.Time, all bool, group log.AggregateGroup) log.AggregateQuery {
	q := log.AggregateQuery{Group: group}
	if all {
		q.CandidateStart, q.CandidateEnd = "1970-01-01", "9999-01-01"
	} else {
		q.CandidateStart, q.CandidateEnd = start.Add(-48*time.Hour).Format(time.DateOnly), end.Add(48*time.Hour).Format(time.DateOnly)
		q.Start, q.End = &start, &end
	}
	return q
}

func aggregateTimeZone(loc *time.Location) string {
	if loc == nil {
		return "UTC"
	}
	// Fixed test/internal zones may not be names understood by PostgreSQL.
	if _, err := time.LoadLocation(loc.String()); err != nil {
		return ""
	}
	return loc.String()
}

func (s *Service) readAggregate(ctx context.Context, householdID int64, q log.AggregateQuery) ([]log.AggregateRow, error) {
	readCtx, _, err := s.readContext(ctx, householdID)
	if err != nil {
		return nil, err
	}
	return s.logStore.(aggregateReader).Aggregate(readCtx, householdID, q)
}

func (s *Service) aggregateLeaderboard(ctx context.Context, householdID int64, start, end time.Time, all bool) ([]LeaderboardEntry, error) {
	rows, err := s.readAggregate(ctx, householdID, aggregateWindow(start, end, all, log.GroupUser))
	if err != nil {
		return nil, err
	}
	var out []LeaderboardEntry
	for _, r := range rows {
		out = append(out, LeaderboardEntry{UserID: r.UserID, Count: r.Count})
	}
	sortLeaderboard(out)
	return out, nil
}

func sortLeaderboard(entries []LeaderboardEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count == entries[j].Count {
			return entries[i].UserID < entries[j].UserID
		}
		return entries[i].Count > entries[j].Count
	})
}

func sortCategories(entries []CategoryBreakdown) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count == entries[j].Count {
			return entries[i].Category < entries[j].Category
		}
		return entries[i].Count > entries[j].Count
	})
}

func (s *Service) aggregateCategories(ctx context.Context, householdID int64, start, end time.Time) ([]CategoryBreakdown, error) {
	rows, err := s.readAggregate(ctx, householdID, aggregateWindow(start, end, false, log.GroupCategory))
	if err != nil {
		return nil, err
	}
	var out []CategoryBreakdown
	for _, r := range rows {
		out = append(out, CategoryBreakdown{Category: r.Category, Count: r.Count})
	}
	sortCategories(out)
	return out, nil
}

func (s *Service) aggregateTopChores(ctx context.Context, householdID, userID int64, n int, period string, loc *time.Location) ([]TopChoresEntry, error) {
	if n <= 0 {
		n = 5
	}
	now := nowIn(loc)
	var start, end time.Time
	switch period {
	case "day":
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		end = start.AddDate(0, 0, 1)
	case "week":
		start = wkStart(now, loc)
		end = start.AddDate(0, 0, 7)
	case "all":
	default:
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		end = now
	}
	q := aggregateWindow(start, end, period == "all", log.GroupChore)
	q.UserID = userID
	rows, err := s.readAggregate(ctx, householdID, q)
	if err != nil {
		return nil, err
	}
	chores, err := s.visibleChoresForContext(ctx, householdID)
	if err != nil {
		return nil, err
	}
	counts := map[int64]int{}
	for _, r := range rows {
		counts[r.ChoreID] = r.Count
	}
	out := []TopChoresEntry{}
	for _, ch := range chores {
		if count := counts[ch.ID]; count > 0 {
			out = append(out, TopChoresEntry{ChoreID: ch.ID, ChoreName: ch.Name, ChoreIcon: ch.Icon, Count: count})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].ChoreID < out[j].ChoreID
		}
		return out[i].Count > out[j].Count
	})
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}

func (s *Service) aggregateSummary(ctx context.Context, householdID int64, ch ChoreInfo, start, end time.Time, all bool) (*ChoreSummary, error) {
	q := aggregateWindow(start, end, all, log.GroupMemberTotals)
	q.ChoreID = ch.ID
	rows, err := s.readAggregate(ctx, householdID, q)
	if err != nil {
		return nil, err
	}
	out := &ChoreSummary{ChoreID: ch.ID, MetricType: ch.MetricType, MetricUnit: ch.MetricUnit, ByMember: []LeaderboardEntry{}}
	for _, r := range rows {
		out.Count += r.Count
		out.TotalML += r.TotalML
		out.TotalDuration += r.Duration
		out.ByMember = append(out.ByMember, LeaderboardEntry{UserID: r.UserID, Count: r.Count})
	}
	sortLeaderboard(out.ByMember)
	return out, nil
}

func (s *Service) aggregateOverview(ctx context.Context, householdID, userID int64, loc *time.Location) (WeeklyOverview, error) {
	now := nowIn(loc)
	yearStart, yearEnd := now.AddDate(-1, 0, 0), now.AddDate(0, 0, 1)
	weekStart := wkStart(now, loc)
	weekEnd := weekStart.AddDate(0, 0, 7)
	q := aggregateWindow(yearStart, yearEnd, false, log.GroupWeekSplit)
	q.Start, q.End = &weekStart, &weekEnd
	q.TimeZone = aggregateTimeZone(loc)
	rows, err := s.readAggregate(ctx, householdID, q)
	if err != nil {
		return WeeklyOverview{}, err
	}
	datesQuery := aggregateWindow(yearStart, yearEnd, false, log.GroupLocalDate)
	datesQuery.UserID, datesQuery.TimeZone = userID, q.TimeZone
	datesQuery.MatchUser = true
	dates, err := s.readAggregate(ctx, householdID, datesQuery)
	if err != nil {
		return WeeklyOverview{}, err
	}
	daySet := map[string]bool{}
	for _, r := range dates {
		daySet[r.Date] = true
	}
	out := WeeklyOverview{Leaderboard: []LeaderboardEntry{}, Breakdown: []CategoryBreakdown{}, Streaks: streaksFromDays(daySet, yearStart, now)}
	users, categories, weekdays := map[int64]int{}, map[string]int{}, map[int]int{}
	for _, r := range rows {
		users[r.UserID] += r.Count
		categories[r.Category] += r.Count
		weekdays[r.Weekday] += r.Count
		out.Recap.TotalChores += r.Count
	}
	for id, count := range users {
		out.Leaderboard = append(out.Leaderboard, LeaderboardEntry{UserID: id, Count: count})
	}
	sortLeaderboard(out.Leaderboard)
	if len(out.Leaderboard) > 0 {
		top := out.Leaderboard[0]
		out.Recap.TopPerformer = &top
	}
	for category, count := range categories {
		out.Breakdown = append(out.Breakdown, CategoryBreakdown{Category: category, Count: count})
	}
	sortCategories(out.Breakdown)
	if len(out.Breakdown) > 0 {
		out.Recap.ByCategory = append([]CategoryBreakdown{}, out.Breakdown...)
	}
	best := 0
	for day := 0; day < 7; day++ {
		if weekdays[day] > best {
			best = weekdays[day]
			out.Recap.MostActiveDay = time.Weekday(day).String()
		}
	}
	return out, nil
}
