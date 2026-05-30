package stats

import (
	"context"
	"sort"
	"time"

	"github.com/dave/choresy/internal/log"
)

func nowIn(loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	return time.Now().In(loc)
}

type LeaderboardEntry struct {
	UserID int64 `json:"userId"`
	Count  int   `json:"count"`
}

type StreakInfo struct {
	Current int `json:"current"`
	Longest int `json:"longest"`
}

type HeatmapCell struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type CategoryBreakdown struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

type BusyHour struct {
	Hour  int `json:"hour"`
	Count int `json:"count"`
}

type WeeklyRecap struct {
	TotalChores   int                 `json:"totalChores"`
	TopPerformer  *LeaderboardEntry   `json:"topPerformer"`
	MostActiveDay string              `json:"mostActiveDay"`
	ByCategory    []CategoryBreakdown `json:"byCategory"`
}

type WeeklyOverview struct {
	Leaderboard []LeaderboardEntry  `json:"leaderboard"`
	Streaks     StreakInfo          `json:"streaks"`
	Breakdown   []CategoryBreakdown `json:"breakdown"`
	Recap       WeeklyRecap         `json:"recap"`
}

type Service struct {
	logStore   log.Store
	choreStore choreStore
}

type choreStore interface {
	GetChore(ctx context.Context, id int64) (ChoreInfo, error)
	ListChores(ctx context.Context, householdID int64) ([]ChoreInfo, error)
}

type ChoreInfo struct {
	ID              int64
	Name            string
	Icon            string
	Color           string
	Category        string
	HasVolumeML     bool
	IndicatorLabels []string
}

type ChoreStats struct {
	ChoreID         int64          `json:"choreId"`
	ChoreName       string         `json:"choreName"`
	ChoreIcon       string         `json:"choreIcon"`
	TotalThisWeek   int            `json:"totalThisWeek"`
	TotalThisMonth  int            `json:"totalThisMonth"`
	IndicatorCounts map[string]int `json:"indicatorCounts,omitempty"`
	VolumeHistory   []VolumeDay    `json:"volumeHistory,omitempty"`
	AvgVolume       *float64       `json:"avgVolume,omitempty"`
	HasVolume       bool           `json:"hasVolume"`
	HasIndicators   bool           `json:"hasIndicators"`
}

type VolumeDay struct {
	Date    string `json:"date"`
	TotalML int    `json:"totalML"`
}

type ChoreTimeSeries struct {
	ChoreID   int64              `json:"choreId"`
	ChoreName string             `json:"choreName"`
	ChoreIcon string             `json:"choreIcon"`
	ByMember  []LeaderboardEntry `json:"byMember"`
	Periods   []TimeSeriesPeriod `json:"periods"`
}

type TimeSeriesPeriod struct {
	Start             string         `json:"start"`
	End               string         `json:"end"`
	Count             int            `json:"count"`
	TotalML           int            `json:"totalML"`
	Indicators        map[string]int `json:"indicators,omitempty"`
	VolumeByIndicator map[string]int `json:"volumeByIndicator,omitempty"`
}

func NewService(logStore log.Store, choreStore choreStore) *Service {
	return &Service{logStore: logStore, choreStore: choreStore}
}

func (s *Service) GetWeeklyLeaderboard(ctx context.Context, householdID int64, loc *time.Location) ([]LeaderboardEntry, error) {
	now := nowIn(loc)
	weekStart := wkStart(now, loc)
	weekEnd := weekStart.AddDate(0, 0, 7)
	return s.getLeaderboard(ctx, householdID, weekStart, weekEnd, loc)
}

func (s *Service) GetMonthlyLeaderboard(ctx context.Context, householdID int64, year int, month time.Month, loc *time.Location) ([]LeaderboardEntry, error) {
	start := time.Date(year, month, 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, 0)
	return s.getLeaderboard(ctx, householdID, start, end, loc)
}

func (s *Service) getLeaderboard(ctx context.Context, householdID int64, start, end time.Time, loc *time.Location) ([]LeaderboardEntry, error) {
	logs, err := s.fetchLogsInRange(ctx, householdID, start, end, loc)
	if err != nil {
		return nil, err
	}
	counts := map[int64]int{}
	for _, l := range logs {
		if logInRange(l, start, end, loc) {
			counts[l.UserID]++
		}
	}
	var entries []LeaderboardEntry
	for uid, c := range counts {
		entries = append(entries, LeaderboardEntry{UserID: uid, Count: c})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Count > entries[j].Count })
	return entries, nil
}

func (s *Service) GetUserStreaks(ctx context.Context, householdID, userID int64, loc *time.Location) (StreakInfo, error) {
	now := nowIn(loc)
	start := now.AddDate(-1, 0, 0)
	end := now.AddDate(0, 0, 1)
	logs, err := s.fetchLogsInRange(ctx, householdID, start, end, loc)
	if err != nil {
		return StreakInfo{}, err
	}

	daySet := map[string]bool{}
	for _, l := range logs {
		if l.UserID == userID && logInRange(l, start, end, loc) {
			daySet[l.CompletedAt.In(loc).Format("2006-01-02")] = true
		}
	}

	checkNow := nowIn(loc)
	current := 0
	for i := 0; i < 365; i++ {
		d := checkNow.AddDate(0, 0, -i).Format("2006-01-02")
		if daySet[d] {
			current++
		} else {
			break
		}
	}

	longest := 0
	streak := 0
	streakStart := start
	for d := streakStart; !d.After(checkNow); d = d.AddDate(0, 0, 1) {
		if daySet[d.Format("2006-01-02")] {
			streak++
			if streak > longest {
				longest = streak
			}
		} else {
			streak = 0
		}
	}
	if streak > longest {
		longest = streak
	}

	return StreakInfo{Current: current, Longest: longest}, nil
}

func (s *Service) GetHeatmap(ctx context.Context, householdID int64, start, end time.Time, loc *time.Location) ([]HeatmapCell, error) {
	logs, err := s.fetchLogsInRange(ctx, householdID, start, end, loc)
	if err != nil {
		return nil, err
	}
	dayCount := map[string]int{}
	for _, l := range logs {
		if logInRange(l, start, end, loc) {
			dayCount[l.CompletedAt.In(loc).Format("2006-01-02")]++
		}
	}
	var cells []HeatmapCell
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		cells = append(cells, HeatmapCell{Date: key, Count: dayCount[key]})
	}
	return cells, nil
}

func (s *Service) GetCategoryBreakdown(ctx context.Context, householdID int64, start, end time.Time, loc *time.Location) ([]CategoryBreakdown, error) {
	chores, err := s.choreStore.ListChores(ctx, householdID)
	if err != nil {
		return nil, err
	}
	choreCat := map[int64]string{}
	for _, c := range chores {
		choreCat[c.ID] = c.Category
	}

	logs, err := s.fetchLogsInRange(ctx, householdID, start, end, loc)
	if err != nil {
		return nil, err
	}
	catCount := map[string]int{}
	for _, l := range logs {
		if logInRange(l, start, end, loc) {
			cat := choreCat[l.ChoreID]
			if cat == "" {
				cat = "custom"
			}
			catCount[cat]++
		}
	}

	var breakdown []CategoryBreakdown
	for cat, c := range catCount {
		breakdown = append(breakdown, CategoryBreakdown{Category: cat, Count: c})
	}
	sort.Slice(breakdown, func(i, j int) bool { return breakdown[i].Count > breakdown[j].Count })
	return breakdown, nil
}

func (s *Service) GetBusyHours(ctx context.Context, householdID int64, start, end time.Time, loc *time.Location) ([]BusyHour, error) {
	logs, err := s.fetchLogsInRange(ctx, householdID, start, end, loc)
	if err != nil {
		return nil, err
	}
	hourCount := map[int]int{}
	for _, l := range logs {
		if logInRange(l, start, end, loc) {
			hourCount[l.CompletedAt.In(loc).Hour()]++
		}
	}

	var hours []BusyHour
	for h := 0; h < 24; h++ {
		hours = append(hours, BusyHour{Hour: h, Count: hourCount[h]})
	}
	return hours, nil
}

func (s *Service) GetWeeklyRecap(ctx context.Context, householdID int64, loc *time.Location) (WeeklyRecap, error) {
	now := nowIn(loc)
	weekStart := wkStart(now, loc)
	weekEnd := weekStart.AddDate(0, 0, 7)

	logs, err := s.fetchLogsInRange(ctx, householdID, weekStart, weekEnd, loc)
	if err != nil {
		return WeeklyRecap{}, err
	}

	recap := WeeklyRecap{}

	counts := map[int64]int{}
	dayCounts := map[string]int{}
	for _, l := range logs {
		if logInRange(l, weekStart, weekEnd, loc) {
			counts[l.UserID]++
			dayCounts[l.CompletedAt.In(loc).Weekday().String()]++
		}
	}
	recap.TotalChores = 0
	for _, c := range counts {
		recap.TotalChores += c
	}

	var top *LeaderboardEntry
	for uid, c := range counts {
		if top == nil || c > top.Count {
			top = &LeaderboardEntry{UserID: uid, Count: c}
		}
	}
	recap.TopPerformer = top

	var bestDay string
	bestCount := 0
	for d, c := range dayCounts {
		if c > bestCount {
			bestDay = d
			bestCount = c
		}
	}
	recap.MostActiveDay = bestDay

	chores, err := s.choreStore.ListChores(ctx, householdID)
	if err != nil {
		return recap, nil
	}
	choreCat := map[int64]string{}
	for _, c := range chores {
		choreCat[c.ID] = c.Category
	}
	catCount := map[string]int{}
	for _, l := range logs {
		if logInRange(l, weekStart, weekEnd, loc) {
			cat := choreCat[l.ChoreID]
			if cat == "" {
				cat = "custom"
			}
			catCount[cat]++
		}
	}
	for cat, c := range catCount {
		recap.ByCategory = append(recap.ByCategory, CategoryBreakdown{Category: cat, Count: c})
	}

	return recap, nil
}

func wkStart(t time.Time, loc *time.Location) time.Time {
	wd := t.Weekday()
	if wd == time.Sunday {
		wd = 7
	}
	start := t.AddDate(0, 0, -int(wd)+1)
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
}

func (s *Service) GetChoreStats(ctx context.Context, householdID int64, loc *time.Location) ([]ChoreStats, error) {
	now := nowIn(loc)
	weekStart := wkStart(now, loc)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	days30Start := now.AddDate(0, 0, -29)
	end := now.AddDate(0, 0, 1)

	logs, err := s.fetchLogsInRange(ctx, householdID, days30Start, end, loc)
	if err != nil {
		return nil, err
	}

	chores, err := s.choreStore.ListChores(ctx, householdID)
	if err != nil {
		return nil, err
	}

	var result []ChoreStats
	for _, ch := range chores {
		var weekCount, monthCount int
		indicatorCounts := map[string]int{}
		volumeByDay := map[string]int{}
		var totalVolume, volumeLogs int

		for _, l := range logs {
			if l.ChoreID != ch.ID {
				continue
			}
			if !l.CompletedAt.In(loc).Before(weekStart) {
				weekCount++
			}
			if !l.CompletedAt.In(loc).Before(monthStart) {
				monthCount++
			}
			for _, ind := range l.Indicators {
				indicatorCounts[ind]++
			}
			if l.VolumeML != nil && *l.VolumeML > 0 {
				dayKey := l.CompletedAt.In(loc).Format("2006-01-02")
				volumeByDay[dayKey] += *l.VolumeML
				totalVolume += *l.VolumeML
				volumeLogs++
			}
		}

		cs := ChoreStats{
			ChoreID:        ch.ID,
			ChoreName:      ch.Name,
			ChoreIcon:      ch.Icon,
			TotalThisWeek:  weekCount,
			TotalThisMonth: monthCount,
			HasVolume:      ch.HasVolumeML,
			HasIndicators:  len(ch.IndicatorLabels) > 0,
		}

		if len(indicatorCounts) > 0 {
			cs.IndicatorCounts = indicatorCounts
		}

		if ch.HasVolumeML && volumeLogs > 0 {
			avg := float64(totalVolume) / float64(volumeLogs)
			cs.AvgVolume = &avg
			for d := days30Start; !d.After(now); d = d.AddDate(0, 0, 1) {
				key := d.Format("2006-01-02")
				cs.VolumeHistory = append(cs.VolumeHistory, VolumeDay{Date: key, TotalML: volumeByDay[key]})
			}
		}

		result = append(result, cs)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalThisMonth > result[j].TotalThisMonth
	})

	return result, nil
}

func (s *Service) GetWeeklyOverview(ctx context.Context, householdID, userID int64, loc *time.Location) (WeeklyOverview, error) {
	now := nowIn(loc)
	weekStart := wkStart(now, loc)
	weekEnd := weekStart.AddDate(0, 0, 7)

	logs, err := s.fetchLogsInRange(ctx, householdID, weekStart, weekEnd, loc)
	if err != nil {
		return WeeklyOverview{}, err
	}

	chores, err := s.choreStore.ListChores(ctx, householdID)
	if err != nil {
		return WeeklyOverview{}, err
	}

	overview := WeeklyOverview{}

	// Leaderboard
	counts := map[int64]int{}
	for _, l := range logs {
		if logInRange(l, weekStart, weekEnd, loc) {
			counts[l.UserID]++
		}
	}
	overview.Leaderboard = []LeaderboardEntry{}
	for uid, c := range counts {
		overview.Leaderboard = append(overview.Leaderboard, LeaderboardEntry{UserID: uid, Count: c})
	}
	sort.Slice(overview.Leaderboard, func(i, j int) bool {
		return overview.Leaderboard[i].Count > overview.Leaderboard[j].Count
	})

	// Streaks (for the requesting user)
	overview.Streaks, _ = s.GetUserStreaks(ctx, householdID, userID, loc)

	// Breakdown
	choreCat := map[int64]string{}
	for _, c := range chores {
		choreCat[c.ID] = c.Category
	}
	catCount := map[string]int{}
	for _, l := range logs {
		if logInRange(l, weekStart, weekEnd, loc) {
			cat := choreCat[l.ChoreID]
			if cat == "" {
				cat = "custom"
			}
			catCount[cat]++
		}
	}
	overview.Breakdown = []CategoryBreakdown{}
	for cat, c := range catCount {
		overview.Breakdown = append(overview.Breakdown, CategoryBreakdown{Category: cat, Count: c})
	}
	sort.Slice(overview.Breakdown, func(i, j int) bool {
		return overview.Breakdown[i].Count > overview.Breakdown[j].Count
	})

	// Recap
	overview.Recap.TotalChores = 0
	for _, c := range counts {
		overview.Recap.TotalChores += c
	}

	var top *LeaderboardEntry
	for uid, c := range counts {
		if top == nil || c > top.Count {
			top = &LeaderboardEntry{UserID: uid, Count: c}
		}
	}
	overview.Recap.TopPerformer = top

	dayCounts := map[string]int{}
	for _, l := range logs {
		if logInRange(l, weekStart, weekEnd, loc) {
			dayCounts[l.CompletedAt.In(loc).Weekday().String()]++
		}
	}
	var bestDay string
	bestCount := 0
	for d, c := range dayCounts {
		if c > bestCount {
			bestDay = d
			bestCount = c
		}
	}
	overview.Recap.MostActiveDay = bestDay

	for cat, c := range catCount {
		overview.Recap.ByCategory = append(overview.Recap.ByCategory, CategoryBreakdown{Category: cat, Count: c})
	}

	return overview, nil
}

func (s *Service) GetChoreTimeSeries(ctx context.Context, householdID, choreID int64, period string, loc *time.Location) (*ChoreTimeSeries, error) {
	ch, err := s.choreStore.GetChore(ctx, choreID)
	if err != nil {
		return nil, err
	}

	now := nowIn(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	var start time.Time
	var buckets []timeBucket

	switch period {
	case "weekly":
		monday := wkStart(today, loc)
		start = monday.AddDate(0, 0, -11*7)
		buckets = buildWeekBuckets(start, today, loc)
	case "monthly":
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, -5, 0)
		buckets = buildMonthBuckets(start, today, loc)
	default:
		start = today.AddDate(0, 0, -13)
		buckets = buildDayBuckets(start, today, loc)
	}

	end := today.AddDate(0, 0, 1)

	logs, err := s.fetchLogsInRange(ctx, householdID, start, end, loc)
	if err != nil {
		return nil, err
	}

	yearStart := today.AddDate(-1, 0, 0)
	yearEnd := today.AddDate(0, 0, 1)
	allLogs, _ := s.fetchLogsInRange(ctx, householdID, yearStart, yearEnd, loc)

	byMember := map[int64]int{}
	for _, l := range allLogs {
		if l.ChoreID == choreID && logInRange(l, yearStart, yearEnd, loc) {
			byMember[l.UserID]++
		}
	}
	var memberEntries []LeaderboardEntry
	for uid, c := range byMember {
		memberEntries = append(memberEntries, LeaderboardEntry{UserID: uid, Count: c})
	}
	sort.Slice(memberEntries, func(i, j int) bool { return memberEntries[i].Count > memberEntries[j].Count })

	type bucketData struct {
		count              int
		totalML            int
		indicators         map[string]int
		volumeByIndicator  map[string]int
	}
	periodData := make([]bucketData, len(buckets))
	for _, l := range logs {
		if l.ChoreID != choreID {
			continue
		}
		t := l.CompletedAt.In(loc)
		for i, b := range buckets {
			if !t.Before(b.start) && t.Before(b.end) {
				periodData[i].count++
				if l.VolumeML != nil {
					periodData[i].totalML += *l.VolumeML
					for _, ind := range l.Indicators {
						if periodData[i].volumeByIndicator == nil {
							periodData[i].volumeByIndicator = map[string]int{}
						}
						periodData[i].volumeByIndicator[ind] += *l.VolumeML
					}
				}
				for _, ind := range l.Indicators {
					if periodData[i].indicators == nil {
						periodData[i].indicators = map[string]int{}
					}
					periodData[i].indicators[ind]++
				}
				break
			}
		}
	}

	result := &ChoreTimeSeries{
		ChoreID:   ch.ID,
		ChoreName: ch.Name,
		ChoreIcon: ch.Icon,
		ByMember:  memberEntries,
	}

	for i, b := range buckets {
		tp := TimeSeriesPeriod{
			Start:   b.start.Format("2006-01-02"),
			End:     b.end.Format("2006-01-02"),
			Count:   periodData[i].count,
			TotalML: periodData[i].totalML,
		}
		if len(periodData[i].indicators) > 0 {
			tp.Indicators = periodData[i].indicators
		}
		if len(periodData[i].volumeByIndicator) > 0 {
			tp.VolumeByIndicator = periodData[i].volumeByIndicator
		}
		result.Periods = append(result.Periods, tp)
	}

	return result, nil
}

type timeBucket struct {
	start time.Time
	end   time.Time
}

func buildDayBuckets(start, end time.Time, loc *time.Location) []timeBucket {
	var buckets []timeBucket
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		buckets = append(buckets, timeBucket{start: d, end: d.AddDate(0, 0, 1)})
	}
	return buckets
}

func buildWeekBuckets(start, end time.Time, loc *time.Location) []timeBucket {
	var buckets []timeBucket
	for d := start; !d.After(end); d = d.AddDate(0, 0, 7) {
		buckets = append(buckets, timeBucket{start: d, end: d.AddDate(0, 0, 7)})
	}
	return buckets
}

func buildMonthBuckets(start, end time.Time, loc *time.Location) []timeBucket {
	var buckets []timeBucket
	for d := start; !d.After(end); d = d.AddDate(0, 1, 0) {
		buckets = append(buckets, timeBucket{start: d, end: d.AddDate(0, 1, 0)})
	}
	return buckets
}

// fetchLogsInRange fetches logs with widened bounds to account for timezone
// offsets, so that the caller can filter by local date in Go.
func (s *Service) fetchLogsInRange(ctx context.Context, householdID int64, start, end time.Time, loc *time.Location) ([]log.ChoreLog, error) {
	bufStart := start.Add(-48 * time.Hour)
	bufEnd := end.Add(48 * time.Hour)
	return s.logStore.ListLogsRange(ctx, householdID, bufStart, bufEnd)
}

// logInRange returns true if the log's local date falls within [start, end).
// Always uses CompletedAt converted to the user's timezone, since the heatmap
// groups by the same conversion. LogDate (which may reflect the browser's
// local time rather than the user's chosen timezone) is not used at this
// layer — the store layer already handles LogDate-vs-CompletedAt filtering.
func logInRange(l log.ChoreLog, start, end time.Time, loc *time.Location) bool {
	local := l.CompletedAt.In(loc)
	return !local.Before(start) && local.Before(end)
}
