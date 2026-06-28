package stats

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/HammerMeetNail/nabu/internal/log"
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
	HouseholdID     int64
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
	TotalInRange    int            `json:"totalInRange"`
	IndicatorCounts map[string]int `json:"indicatorCounts,omitempty"`
	VolumeHistory   []VolumeDay    `json:"volumeHistory,omitempty"`
	AvgVolume       *float64       `json:"avgVolume,omitempty"`
	HasVolume       bool           `json:"hasVolume"`
	HasIndicators   bool           `json:"hasIndicators"`
}

type TopChoresEntry struct {
	ChoreID   int64  `json:"choreId"`
	ChoreName string `json:"choreName"`
	ChoreIcon string `json:"choreIcon"`
	Count     int    `json:"count"`
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

type FeedingGap struct {
	Hour            int    `json:"hour"`
	GapMinutes      int    `json:"gapMinutes"`
	PrecedingVolume int    `json:"precedingVolume"`
	FollowUpVolume  int    `json:"followUpVolume"`
	Date            string `json:"date"`
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

func (s *Service) GetDailyLeaderboard(ctx context.Context, householdID int64, loc *time.Location) ([]LeaderboardEntry, error) {
	now := nowIn(loc)
	y, m, d := now.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	return s.getLeaderboard(ctx, householdID, start, end, loc)
}

// GetAllTimeLeaderboard counts every log ever recorded for the household.
// The returned range bounds are empty (no sensible start/end), so callers
// should omit start/end from the response.
func (s *Service) GetAllTimeLeaderboard(ctx context.Context, householdID int64, loc *time.Location) ([]LeaderboardEntry, error) {
	// Fetch all logs for the household with no time bound. We widen the
	// window by a generous margin and then filter by local completion time
	// inside getLeaderboard; since the window spans the entire history,
	// every local time falls within it.
	epochStart := time.Unix(0, 0).UTC()
	farFuture := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	logs, err := s.logStore.ListLogsRange(ctx, householdID, epochStart, farFuture)
	if err != nil {
		return nil, err
	}
	counts := map[int64]int{}
	for _, l := range logs {
		counts[l.UserID]++
	}
	var entries []LeaderboardEntry
	for uid, c := range counts {
		entries = append(entries, LeaderboardEntry{UserID: uid, Count: c})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Count > entries[j].Count })
	return entries, nil
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
	return computeStreaks(logs, userID, start, end, loc), nil
}

func computeStreaks(logs []log.ChoreLog, userID int64, start, end time.Time, loc *time.Location) StreakInfo {
	now := nowIn(loc)
	daySet := map[string]bool{}
	for _, l := range logs {
		if l.UserID == userID && logInRange(l, start, end, loc) {
			daySet[l.CompletedAt.In(loc).Format("2006-01-02")] = true
		}
	}

	current := 0
	for i := 0; i < 365; i++ {
		d := now.AddDate(0, 0, -i).Format("2006-01-02")
		if daySet[d] {
			current++
		} else {
			break
		}
	}

	longest := 0
	streak := 0
	for d := start; !d.After(now); d = d.AddDate(0, 0, 1) {
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

	return StreakInfo{Current: current, Longest: longest}
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

func (s *Service) GetBusyHours(ctx context.Context, householdID int64, start, end time.Time, loc *time.Location, choreID, userID *int64) ([]BusyHour, error) {
	logs, err := s.fetchLogsInRange(ctx, householdID, start, end, loc)
	if err != nil {
		return nil, err
	}
	hourCount := map[int]int{}
	for _, l := range logs {
		if !logInRange(l, start, end, loc) {
			continue
		}
		if choreID != nil && l.ChoreID != *choreID {
			continue
		}
		if userID != nil && l.UserID != *userID {
			continue
		}
		hourCount[l.CompletedAt.In(loc).Hour()]++
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

func (s *Service) GetChoreStats(ctx context.Context, householdID int64, loc *time.Location, customStart, customEnd *time.Time) ([]ChoreStats, error) {
	now := nowIn(loc)
	weekStart := wkStart(now, loc)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)

	var fetchStart, fetchEnd time.Time
	if customStart != nil && customEnd != nil {
		fetchStart = *customStart
		fetchEnd = *customEnd
	} else {
		fetchStart = now.AddDate(0, 0, -29)
		fetchEnd = now.AddDate(0, 0, 1)
	}

	logs, err := s.fetchLogsInRange(ctx, householdID, fetchStart, fetchEnd, loc)
	if err != nil {
		return nil, err
	}

	chores, err := s.choreStore.ListChores(ctx, householdID)
	if err != nil {
		return nil, err
	}

	// Build a chore→logs index to avoid O(chores × logs) nested iteration.
	logsByChore := map[int64][]log.ChoreLog{}
	for _, l := range logs {
		logsByChore[l.ChoreID] = append(logsByChore[l.ChoreID], l)
	}

	var result []ChoreStats
	for _, ch := range chores {
		var weekCount, monthCount, rangeCount int
		indicatorCounts := map[string]int{}
		volumeByDay := map[string]int{}
		var totalVolume, volumeLogs int

		for _, l := range logsByChore[ch.ID] {
			inRange := logInRange(l, fetchStart, fetchEnd, loc)
			if inRange {
				rangeCount++
			}
			if !l.CompletedAt.In(loc).Before(weekStart) {
				weekCount++
			}
			if !l.CompletedAt.In(loc).Before(monthStart) {
				monthCount++
			}
			if !inRange {
				continue
			}
			for _, ind := range l.Indicators {
				indicatorCounts[ind]++
			}
			if len(l.IndicatorVolumes) > 0 {
				for _, vol := range l.IndicatorVolumes {
					if vol <= 0 {
						continue
					}
					dayKey := l.CompletedAt.In(loc).Format("2006-01-02")
					volumeByDay[dayKey] += vol
					totalVolume += vol
					volumeLogs++
				}
			} else if l.VolumeML != nil && *l.VolumeML > 0 {
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
			TotalInRange:   rangeCount,
			HasVolume:      ch.HasVolumeML,
			HasIndicators:  len(ch.IndicatorLabels) > 0,
		}

		if len(indicatorCounts) > 0 {
			cs.IndicatorCounts = indicatorCounts
		}

		if ch.HasVolumeML && volumeLogs > 0 {
			avg := float64(totalVolume) / float64(volumeLogs)
			cs.AvgVolume = &avg
			for d := fetchStart; !d.After(now); d = d.AddDate(0, 0, 1) {
				key := d.Format("2006-01-02")
				cs.VolumeHistory = append(cs.VolumeHistory, VolumeDay{Date: key, TotalML: volumeByDay[key]})
			}
		}

		result = append(result, cs)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].TotalInRange != result[j].TotalInRange {
			return result[i].TotalInRange > result[j].TotalInRange
		}
		return result[i].TotalThisMonth > result[j].TotalThisMonth
	})

	return result, nil
}

func (s *Service) GetWeeklyOverview(ctx context.Context, householdID, userID int64, loc *time.Location) (WeeklyOverview, error) {
	now := nowIn(loc)
	weekStart := wkStart(now, loc)
	weekEnd := weekStart.AddDate(0, 0, 7)

	// Fetch a full year of logs once to serve both the week overview and streaks.
	yearStart := now.AddDate(-1, 0, 0)
	yearEnd := now.AddDate(0, 0, 1)
	logs, err := s.fetchLogsInRange(ctx, householdID, yearStart, yearEnd, loc)
	if err != nil {
		return WeeklyOverview{}, err
	}

	chores, err := s.choreStore.ListChores(ctx, householdID)
	if err != nil {
		return WeeklyOverview{}, err
	}

	overview := WeeklyOverview{}

	// Leaderboard (week only)
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

	// Streaks (for the requesting user) — computed from the same year fetch.
	overview.Streaks = computeStreaks(logs, userID, yearStart, yearEnd, loc)

	// Breakdown (week only)
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

func (s *Service) GetTopChores(ctx context.Context, householdID int64, userID int64, n int, period string, loc *time.Location) ([]TopChoresEntry, error) {
	if n <= 0 {
		n = 5
	}
	if period == "" {
		period = "month"
	}

	now := nowIn(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	weekStart := wkStart(now, loc)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)

	var rangeStart, rangeEnd time.Time
	switch period {
	case "day":
		rangeStart = today
		rangeEnd = today.AddDate(0, 0, 1)
	case "week":
		rangeStart = weekStart
		rangeEnd = weekStart.AddDate(0, 0, 7)
	case "all":
		// No time bound — fetch every log ever recorded for the household.
		// We bypass fetchLogsInRange (which widens by 48h) and read the
		// raw log store so we don't risk overflow on a far-future end.
		epochStart := time.Unix(0, 0).UTC()
		farFuture := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
		allLogs, err := s.logStore.ListLogsRange(ctx, householdID, epochStart, farFuture)
		if err != nil {
			return nil, err
		}
		chores, err := s.choreStore.ListChores(ctx, householdID)
		if err != nil {
			return nil, err
		}
		counts := map[int64]int{}
		for _, l := range allLogs {
			if userID > 0 && l.UserID != userID {
				continue
			}
			counts[l.ChoreID]++
		}
		type allEntry struct {
			info  ChoreInfo
			count int
		}
		var ranked []allEntry
		for _, ch := range chores {
			if c, ok := counts[ch.ID]; ok && c > 0 {
				ranked = append(ranked, allEntry{info: ch, count: c})
			}
		}
		sort.Slice(ranked, func(i, j int) bool { return ranked[i].count > ranked[j].count })
		out := make([]TopChoresEntry, 0, n)
		for i := 0; i < n && i < len(ranked); i++ {
			out = append(out, TopChoresEntry{
				ChoreID:   ranked[i].info.ID,
				ChoreName: ranked[i].info.Name,
				ChoreIcon: ranked[i].info.Icon,
				Count:     ranked[i].count,
			})
		}
		return out, nil
	default:
		// "month" (and any unknown value) — mirror the historical default.
		rangeStart = monthStart
		rangeEnd = now
	}

	chores, err := s.choreStore.ListChores(ctx, householdID)
	if err != nil {
		return nil, err
	}

	logs, err := s.fetchLogsInRange(ctx, householdID, rangeStart, rangeEnd, loc)
	if err != nil {
		return nil, err
	}

	counts := map[int64]int{}
	for _, l := range logs {
		if userID > 0 && l.UserID != userID {
			continue
		}
		local := l.CompletedAt.In(loc)
		if !local.Before(rangeStart) && local.Before(rangeEnd) {
			counts[l.ChoreID]++
		}
	}

	type entry struct {
		info  ChoreInfo
		count int
	}
	var ranked []entry
	for _, ch := range chores {
		if c, ok := counts[ch.ID]; ok && c > 0 {
			ranked = append(ranked, entry{info: ch, count: c})
		}
	}

	sort.Slice(ranked, func(i, j int) bool { return ranked[i].count > ranked[j].count })

	out := make([]TopChoresEntry, 0, n)
	for i := 0; i < n && i < len(ranked); i++ {
		out = append(out, TopChoresEntry{
			ChoreID:   ranked[i].info.ID,
			ChoreName: ranked[i].info.Name,
			ChoreIcon: ranked[i].info.Icon,
			Count:     ranked[i].count,
		})
	}

	return out, nil
}

func (s *Service) GetChoreTimeSeries(ctx context.Context, householdID, choreID int64, period string, loc *time.Location) (*ChoreTimeSeries, error) {
	ch, err := s.choreStore.GetChore(ctx, choreID)
	if err != nil {
		return nil, err
	}
	if ch.HouseholdID != householdID {
		return nil, fmt.Errorf("chore not found")
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

	yearStart := today.AddDate(-1, 0, 0)
	yearEnd := today.AddDate(0, 0, 1)

	// The year range always encompasses the period range, so fetch once.
	logs, err := s.fetchLogsInRange(ctx, householdID, yearStart, yearEnd, loc)
	if err != nil {
		return nil, err
	}

	byMember := map[int64]int{}
	for _, l := range logs {
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
		count             int
		totalML           int
		indicators        map[string]int
		volumeByIndicator map[string]int
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
				if len(l.IndicatorVolumes) > 0 {
					for ind, vol := range l.IndicatorVolumes {
						if vol <= 0 {
							continue
						}
						periodData[i].totalML += vol
						if periodData[i].volumeByIndicator == nil {
							periodData[i].volumeByIndicator = map[string]int{}
						}
						periodData[i].volumeByIndicator[ind] += vol
					}
				} else if l.VolumeML != nil {
					periodData[i].totalML += *l.VolumeML
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

func (s *Service) GetFeedingGaps(ctx context.Context, householdID int64, start, end time.Time, loc *time.Location) ([]FeedingGap, error) {
	chores, err := s.choreStore.ListChores(ctx, householdID)
	if err != nil {
		return nil, err
	}

	var feedBabyID int64
	for _, ch := range chores {
		if ch.HouseholdID == householdID && ch.Name == "Feed Baby" {
			feedBabyID = ch.ID
			break
		}
	}
	if feedBabyID == 0 {
		return nil, nil
	}

	logs, err := s.fetchLogsInRange(ctx, householdID, start, end, loc)
	if err != nil {
		return nil, err
	}

	var feedLogs []log.ChoreLog
	for _, l := range logs {
		if l.ChoreID == feedBabyID && logInRange(l, start, end, loc) {
			feedLogs = append(feedLogs, l)
		}
	}

	sort.Slice(feedLogs, func(i, j int) bool {
		return feedLogs[i].CompletedAt.Before(feedLogs[j].CompletedAt)
	})

	var gaps []FeedingGap
	for i := 1; i < len(feedLogs); i++ {
		prev := feedLogs[i-1].CompletedAt.In(loc)
		curr := feedLogs[i].CompletedAt.In(loc)
		gapMinutes := int(curr.Sub(prev).Minutes())

		hour := prev.Hour()

		precedingVolume := 0
		if len(feedLogs[i-1].IndicatorVolumes) > 0 {
			for _, vol := range feedLogs[i-1].IndicatorVolumes {
				precedingVolume += vol
			}
		} else if feedLogs[i-1].VolumeML != nil {
			precedingVolume = *feedLogs[i-1].VolumeML
		}

		followUpVolume := 0
		if len(feedLogs[i].IndicatorVolumes) > 0 {
			for _, vol := range feedLogs[i].IndicatorVolumes {
				followUpVolume += vol
			}
		} else if feedLogs[i].VolumeML != nil {
			followUpVolume = *feedLogs[i].VolumeML
		}

		gaps = append(gaps, FeedingGap{
			Hour:            hour,
			GapMinutes:      gapMinutes,
			PrecedingVolume: precedingVolume,
			FollowUpVolume:  followUpVolume,
			Date:            prev.Format(time.DateOnly),
		})
	}

	return gaps, nil
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
