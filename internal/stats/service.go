package stats

import (
	"context"
	"sort"
	"time"

	"github.com/dave/choresy/internal/log"
)

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

func NewService(logStore log.Store, choreStore choreStore) *Service {
	return &Service{logStore: logStore, choreStore: choreStore}
}

func (s *Service) GetWeeklyLeaderboard(ctx context.Context, householdID int64) ([]LeaderboardEntry, error) {
	now := time.Now().UTC()
	weekStart := wkStart(now)
	weekEnd := weekStart.AddDate(0, 0, 7)
	return s.getLeaderboard(ctx, householdID, weekStart, weekEnd)
}

func (s *Service) GetMonthlyLeaderboard(ctx context.Context, householdID int64, year int, month time.Month) ([]LeaderboardEntry, error) {
	start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	return s.getLeaderboard(ctx, householdID, start, end)
}

func (s *Service) getLeaderboard(ctx context.Context, householdID int64, start, end time.Time) ([]LeaderboardEntry, error) {
	logs, err := s.logStore.ListLogsRange(ctx, householdID, start, end)
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

func (s *Service) GetUserStreaks(ctx context.Context, householdID, userID int64) (StreakInfo, error) {
	now := time.Now().UTC()
	start := now.AddDate(-1, 0, 0)
	logs, err := s.logStore.ListLogsRange(ctx, householdID, start, now.AddDate(0, 0, 1))
	if err != nil {
		return StreakInfo{}, err
	}

	daySet := map[string]bool{}
	for _, l := range logs {
		if l.UserID == userID {
			daySet[l.CompletedAt.UTC().Format("2006-01-02")] = true
		}
	}

	checkNow := time.Now().UTC()
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
	streakStart := now.AddDate(-1, 0, 0)
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

func (s *Service) GetHeatmap(ctx context.Context, householdID int64, start, end time.Time) ([]HeatmapCell, error) {
	logs, err := s.logStore.ListLogsRange(ctx, householdID, start, end)
	if err != nil {
		return nil, err
	}
	dayCount := map[string]int{}
	for _, l := range logs {
		dayCount[l.CompletedAt.UTC().Format("2006-01-02")]++
	}
	var cells []HeatmapCell
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		cells = append(cells, HeatmapCell{Date: key, Count: dayCount[key]})
	}
	return cells, nil
}

func (s *Service) GetCategoryBreakdown(ctx context.Context, householdID int64, start, end time.Time) ([]CategoryBreakdown, error) {
	chores, err := s.choreStore.ListChores(ctx, householdID)
	if err != nil {
		return nil, err
	}
	choreCat := map[int64]string{}
	for _, c := range chores {
		choreCat[c.ID] = c.Category
	}

	logs, err := s.logStore.ListLogsRange(ctx, householdID, start, end)
	if err != nil {
		return nil, err
	}
	catCount := map[string]int{}
	for _, l := range logs {
		cat := choreCat[l.ChoreID]
		if cat == "" {
			cat = "custom"
		}
		catCount[cat]++
	}

	var breakdown []CategoryBreakdown
	for cat, c := range catCount {
		breakdown = append(breakdown, CategoryBreakdown{Category: cat, Count: c})
	}
	sort.Slice(breakdown, func(i, j int) bool { return breakdown[i].Count > breakdown[j].Count })
	return breakdown, nil
}

func (s *Service) GetBusyHours(ctx context.Context, householdID int64, start, end time.Time) ([]BusyHour, error) {
	logs, err := s.logStore.ListLogsRange(ctx, householdID, start, end)
	if err != nil {
		return nil, err
	}
	hourCount := map[int]int{}
	for _, l := range logs {
		hourCount[l.CompletedAt.UTC().Hour()]++
	}

	var hours []BusyHour
	for h := 0; h < 24; h++ {
		hours = append(hours, BusyHour{Hour: h, Count: hourCount[h]})
	}
	return hours, nil
}

func (s *Service) GetWeeklyRecap(ctx context.Context, householdID int64) (WeeklyRecap, error) {
	now := time.Now().UTC()
	weekStart := wkStart(now)
	weekEnd := weekStart.AddDate(0, 0, 7)

	logs, err := s.logStore.ListLogsRange(ctx, householdID, weekStart, weekEnd)
	if err != nil {
		return WeeklyRecap{}, err
	}

	recap := WeeklyRecap{TotalChores: len(logs)}

	counts := map[int64]int{}
	dayCounts := map[string]int{}
	for _, l := range logs {
		counts[l.UserID]++
		dayCounts[l.CompletedAt.UTC().Weekday().String()]++
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
		cat := choreCat[l.ChoreID]
		if cat == "" {
			cat = "custom"
		}
		catCount[cat]++
	}
	for cat, c := range catCount {
		recap.ByCategory = append(recap.ByCategory, CategoryBreakdown{Category: cat, Count: c})
	}

	return recap, nil
}

func wkStart(t time.Time) time.Time {
	wd := t.Weekday()
	if wd == time.Sunday {
		wd = 7
	}
	start := t.AddDate(0, 0, -int(wd)+1)
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *Service) GetChoreStats(ctx context.Context, householdID int64) ([]ChoreStats, error) {
	now := time.Now().UTC()
	weekStart := wkStart(now)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	days30Start := now.AddDate(0, 0, -29)

	logs, err := s.logStore.ListLogsRange(ctx, householdID, days30Start, now.AddDate(0, 0, 1))
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
			if !l.CompletedAt.Before(weekStart) {
				weekCount++
			}
			if !l.CompletedAt.Before(monthStart) {
				monthCount++
			}
			for _, ind := range l.Indicators {
				indicatorCounts[ind]++
			}
			if l.VolumeML != nil && *l.VolumeML > 0 {
				dayKey := l.CompletedAt.UTC().Format("2006-01-02")
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

func (s *Service) GetWeeklyOverview(ctx context.Context, householdID, userID int64) (WeeklyOverview, error) {
	now := time.Now().UTC()
	weekStart := wkStart(now)
	weekEnd := weekStart.AddDate(0, 0, 7)

	logs, err := s.logStore.ListLogsRange(ctx, householdID, weekStart, weekEnd)
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
		counts[l.UserID]++
	}
	overview.Leaderboard = []LeaderboardEntry{}
	for uid, c := range counts {
		overview.Leaderboard = append(overview.Leaderboard, LeaderboardEntry{UserID: uid, Count: c})
	}
	sort.Slice(overview.Leaderboard, func(i, j int) bool {
		return overview.Leaderboard[i].Count > overview.Leaderboard[j].Count
	})

	// Streaks (for the requesting user)
	overview.Streaks, _ = s.GetUserStreaks(ctx, householdID, userID)

	// Breakdown
	choreCat := map[int64]string{}
	for _, c := range chores {
		choreCat[c.ID] = c.Category
	}
	catCount := map[string]int{}
	for _, l := range logs {
		cat := choreCat[l.ChoreID]
		if cat == "" {
			cat = "custom"
		}
		catCount[cat]++
	}
	overview.Breakdown = []CategoryBreakdown{}
	for cat, c := range catCount {
		overview.Breakdown = append(overview.Breakdown, CategoryBreakdown{Category: cat, Count: c})
	}
	sort.Slice(overview.Breakdown, func(i, j int) bool {
		return overview.Breakdown[i].Count > overview.Breakdown[j].Count
	})

	// Recap
	overview.Recap.TotalChores = len(logs)

	var top *LeaderboardEntry
	for uid, c := range counts {
		if top == nil || c > top.Count {
			top = &LeaderboardEntry{UserID: uid, Count: c}
		}
	}
	overview.Recap.TopPerformer = top

	dayCounts := map[string]int{}
	for _, l := range logs {
		dayCounts[l.CompletedAt.UTC().Weekday().String()]++
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
