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

type Service struct {
	logStore   log.Store
	choreStore choreStore
}

type choreStore interface {
	GetChore(ctx context.Context, id int64) (ChoreInfo, error)
	ListChores(ctx context.Context, householdID int64) ([]ChoreInfo, error)
}

type ChoreInfo struct {
	ID       int64
	Name     string
	Icon     string
	Color    string
	Category string
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
