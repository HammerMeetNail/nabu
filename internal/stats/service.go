package stats

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/HammerMeetNail/nabu/internal/log"
)

type viewerKey struct{}

// WithViewer returns a context carrying the viewer's user ID for visibility filtering.
func WithViewer(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, viewerKey{}, userID)
}

func viewerIDFromContext(ctx context.Context) int64 {
	if v, ok := ctx.Value(viewerKey{}).(int64); ok {
		return v
	}
	return 0
}

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
	logStore    log.Store
	choreStore  choreStore
	memberships MembershipReader
}

type MembershipReader interface {
	GetMembershipForHousehold(ctx context.Context, userID, householdID int64) (string, error)
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
	HasRating       bool
	MetricType      string
	MetricUnit      string
	IndicatorLabels []string
	Visibility      string
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
	ChoreID    int64              `json:"choreId"`
	ChoreName  string             `json:"choreName"`
	ChoreIcon  string             `json:"choreIcon"`
	MetricType string             `json:"metricType,omitempty"`
	MetricUnit string             `json:"metricUnit,omitempty"`
	ByMember   []LeaderboardEntry `json:"byMember"`
	Periods    []TimeSeriesPeriod `json:"periods"`
}

type TimeSeriesPeriod struct {
	Start             string         `json:"start"`
	End               string         `json:"end"`
	Count             int            `json:"count"`
	TotalML           int            `json:"totalML"`
	TotalDuration     int            `json:"totalDuration,omitempty"` // sum of duration_seconds in the bucket
	Indicators        map[string]int `json:"indicators,omitempty"`
	VolumeByIndicator map[string]int `json:"volumeByIndicator,omitempty"`
}

// ChoreSummary is a period-scoped aggregate for one chore, used by the
// user-defined widgets (Phase 4) so a widget's period actually bounds the
// numbers (unlike the year-scoped time-series byMember). "all" is true all-time.
type ChoreSummary struct {
	ChoreID       int64              `json:"choreId"`
	Count         int                `json:"count"`
	TotalML       int                `json:"totalML"`
	TotalDuration int                `json:"totalDuration"`
	ByMember      []LeaderboardEntry `json:"byMember"`
	MetricType    string             `json:"metricType,omitempty"`
	MetricUnit    string             `json:"metricUnit,omitempty"`
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

func (s *Service) WithMemberships(m MembershipReader) *Service {
	s.memberships = m
	return s
}

func (s *Service) visibleChoreIDs(ctx context.Context, householdID, userID int64) (map[int64]struct{}, error) {
	if userID == 0 {
		return nil, nil // Internal aggregate calls do not carry a viewer.
	}
	if s.memberships == nil {
		return nil, fmt.Errorf("membership resolver unavailable")
	}
	chores, err := s.choreStore.ListChores(ctx, householdID)
	if err != nil {
		return nil, err
	}
	role, err := s.memberships.GetMembershipForHousehold(ctx, userID, householdID)
	if err != nil {
		return nil, err
	}
	visible := make(map[int64]struct{}, len(chores))
	for _, c := range chores {
		if c.HouseholdID != householdID {
			continue
		}
		if c.Visibility == "admins" {
			if role != "owner" && role != "admin" {
				continue
			}
		}
		visible[c.ID] = struct{}{}
	}
	return visible, nil
}

func (s *Service) visibleChoreIDsForContext(ctx context.Context, householdID int64) (map[int64]struct{}, error) {
	return s.visibleChoreIDs(ctx, householdID, viewerIDFromContext(ctx))
}

func (s *Service) readContext(ctx context.Context, householdID int64) (context.Context, map[int64]struct{}, error) {
	visible, err := s.visibleChoreIDsForContext(ctx, householdID)
	if err != nil {
		return ctx, nil, err
	}
	if viewerIDFromContext(ctx) != 0 {
		ctx = log.WithReadAccess(ctx, householdID, viewerIDFromContext(ctx), visible)
	}
	return ctx, visible, nil
}

func filterLogs(logs []log.ChoreLog, visible map[int64]struct{}) []log.ChoreLog {
	if visible == nil {
		return logs
	}
	var out []log.ChoreLog
	for _, l := range logs {
		if _, ok := visible[l.ChoreID]; ok {
			out = append(out, l)
		}
	}
	return out
}

func (s *Service) canViewChore(ctx context.Context, householdID, choreID, userID int64) (bool, error) {
	if userID == 0 {
		return true, nil
	}
	if s.memberships == nil {
		return false, fmt.Errorf("membership resolver unavailable")
	}
	c, err := s.choreStore.GetChore(ctx, choreID)
	if err != nil {
		return false, err
	}
	if c.HouseholdID != householdID {
		return false, nil
	}
	if c.Visibility == "admins" {
		role, err := s.memberships.GetMembershipForHousehold(ctx, userID, householdID)
		if err != nil {
			return false, err
		}
		if role != "owner" && role != "admin" {
			return false, nil
		}
	}
	return true, nil
}

func (s *Service) canViewChoreForContext(ctx context.Context, householdID, choreID int64) (bool, error) {
	return s.canViewChore(ctx, householdID, choreID, viewerIDFromContext(ctx))
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
	if _, ok := s.logStore.(aggregateReader); ok {
		return s.aggregateLeaderboard(ctx, householdID, time.Time{}, time.Time{}, true)
	}
	// Fetch all logs for the household with no time bound. We widen the
	// window by a generous margin and then filter by local completion time
	// inside getLeaderboard; since the window spans the entire history,
	// every local time falls within it.
	epochStart := time.Unix(0, 0).UTC()
	farFuture := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	readCtx, visible, err := s.readContext(ctx, householdID)
	if err != nil {
		return nil, err
	}
	logs, err := s.logStore.ListLogsRange(readCtx, householdID, epochStart, farFuture)
	if err != nil {
		return nil, err
	}
	logs = filterLogs(logs, visible)
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
	if _, ok := s.logStore.(aggregateReader); ok {
		return s.aggregateLeaderboard(ctx, householdID, start, end, false)
	}
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

	return streaksFromDays(daySet, start, now)
}

func streaksFromDays(daySet map[string]bool, start, now time.Time) StreakInfo {

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
	dayCount := map[string]int{}
	if _, ok := s.logStore.(aggregateReader); ok && aggregateTimeZone(loc) != "" {
		q := aggregateWindow(start, end, false, log.GroupLocalDate)
		q.TimeZone = aggregateTimeZone(loc)
		rows, err := s.readAggregate(ctx, householdID, q)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			dayCount[row.Date] = row.Count
		}
	} else {
		logs, err := s.fetchLogsInRange(ctx, householdID, start, end, loc)
		if err != nil {
			return nil, err
		}
		for _, l := range logs {
			if logInRange(l, start, end, loc) {
				dayCount[l.CompletedAt.In(loc).Format("2006-01-02")]++
			}
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
	if _, ok := s.logStore.(aggregateReader); ok {
		return s.aggregateCategories(ctx, householdID, start, end)
	}
	chores, err := s.visibleChoresForContext(ctx, householdID)
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
	if choreID != nil {
		if ok, err := s.canViewChoreForContext(ctx, householdID, *choreID); err != nil {
			return nil, err
		} else if !ok {
			return nil, fmt.Errorf("chore not found")
		}
	}
	if _, ok := s.logStore.(aggregateReader); ok && aggregateTimeZone(loc) != "" {
		q := aggregateWindow(start, end, false, log.GroupLocalHour)
		q.TimeZone = aggregateTimeZone(loc)
		if choreID != nil {
			q.ChoreID = *choreID
		}
		if userID != nil {
			q.UserID, q.MatchUser = *userID, true
		}
		rows, err := s.readAggregate(ctx, householdID, q)
		if err != nil {
			return nil, err
		}
		hours := make([]BusyHour, 24)
		for h := range hours {
			hours[h].Hour = h
		}
		for _, row := range rows {
			hours[row.Hour].Count = row.Count
		}
		return hours, nil
	}
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

	chores, err := s.visibleChoresForContext(ctx, householdID)
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
	start := t.AddDate(0, 0, -int(t.Weekday()))
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

	chores, err := s.visibleChoresForContext(ctx, householdID)
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
	if _, ok := s.logStore.(aggregateReader); ok && aggregateTimeZone(loc) != "" {
		return s.aggregateOverview(ctx, householdID, userID, loc)
	}
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

	chores, err := s.visibleChoresForContext(ctx, householdID)
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
	if _, ok := s.logStore.(aggregateReader); ok {
		return s.aggregateTopChores(ctx, householdID, userID, n, period, loc)
	}
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
		readCtx, visible, err := s.readContext(ctx, householdID)
		if err != nil {
			return nil, err
		}
		allLogs, err := s.logStore.ListLogsRange(readCtx, householdID, epochStart, farFuture)
		if err != nil {
			return nil, err
		}
		allLogs = filterLogs(allLogs, visible)
		chores, err := s.visibleChoresForContext(ctx, householdID)
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

	chores, err := s.visibleChoresForContext(ctx, householdID)
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
	if ok, err := s.canViewChoreForContext(ctx, householdID, choreID); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("chore not found")
	}
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
		sun := wkStart(today, loc)
		start = sun.AddDate(0, 0, -11*7)
		buckets = buildWeekBuckets(start, today, loc)
	case "monthly":
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, -5, 0)
		buckets = buildMonthBuckets(start, today, loc)
	case "all":
		start = time.Date(2020, 1, 1, 0, 0, 0, 0, loc)
		buckets = buildMonthBuckets(start, today, loc)
	default:
		start = today.AddDate(0, 0, -13)
		buckets = buildDayBuckets(start, today, loc)
	}

	var logFetchStart, logFetchEnd time.Time
	if period == "all" {
		logFetchStart = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		logFetchEnd = today.AddDate(0, 0, 1)
	} else {
		logFetchStart = today.AddDate(-1, 0, 0)
		logFetchEnd = today.AddDate(0, 0, 1)
	}

	var countStart, countEnd time.Time
	switch period {
	case "weekly":
		countStart = wkStart(today, loc)
		countEnd = countStart.AddDate(0, 0, 7)
	case "monthly":
		countStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		countEnd = countStart.AddDate(0, 1, 0)
	case "all":
		countStart = time.Date(2020, 1, 1, 0, 0, 0, 0, loc)
		countEnd = today.AddDate(0, 0, 1)
	default:
		countStart = today
		countEnd = today.AddDate(0, 0, 1)
	}

	// Only this chore's displayed buckets and member totals need metrics. Keep
	// the legacy canonical date candidates so mismatched log_date values retain
	// their meaning; completion bounds provide the selective index seek.
	readStart, readEnd := buckets[0].start, buckets[len(buckets)-1].end
	if countStart.Before(readStart) {
		readStart = countStart
	}
	if countEnd.After(readEnd) {
		readEnd = countEnd
	}
	logs, err := s.fetchStatsLogs(ctx, householdID, log.StatsQuery{
		CandidateStart: logFetchStart.Add(-48 * time.Hour), CandidateEnd: logFetchEnd.Add(48 * time.Hour),
		Start: &readStart, End: &readEnd, ChoreID: choreID,
	})
	if err != nil {
		return nil, err
	}

	byMember := map[int64]int{}
	for _, l := range logs {
		if l.ChoreID == choreID && logInRange(l, countStart, countEnd, loc) {
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
		totalDuration     int
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
				if l.DurationSeconds != nil {
					periodData[i].totalDuration += *l.DurationSeconds
				}
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
		ChoreID:    ch.ID,
		ChoreName:  ch.Name,
		ChoreIcon:  ch.Icon,
		MetricType: ch.MetricType,
		MetricUnit: ch.MetricUnit,
		ByMember:   memberEntries,
	}

	for i, b := range buckets {
		tp := TimeSeriesPeriod{
			Start:         b.start.Format("2006-01-02"),
			End:           b.end.Format("2006-01-02"),
			Count:         periodData[i].count,
			TotalML:       periodData[i].totalML,
			TotalDuration: periodData[i].totalDuration,
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

// GetFeedingGaps computes inter-log interval "gaps" for a chore — the
// generalized interval analysis (Phase 3). When choreID is nil it targets the
// household's "Feed Baby" chore (the historical baby-care behavior). When set,
// it must belong to the household; any chore where intervals matter (feeds,
// medication, watering) can be analyzed.
func (s *Service) GetFeedingGaps(ctx context.Context, householdID int64, choreID *int64, start, end time.Time, loc *time.Location) ([]FeedingGap, error) {
	chores, err := s.visibleChoresForContext(ctx, householdID)
	if err != nil {
		return nil, err
	}

	var feedBabyID int64
	if choreID != nil {
		// Explicit ID must be visible; if not visible, treat as not found -> return 404 via error.
		if ok, err := s.canViewChoreForContext(ctx, householdID, *choreID); err != nil {
			return nil, err
		} else if !ok {
			return nil, fmt.Errorf("chore not found")
		}
		for _, ch := range chores {
			if ch.HouseholdID == householdID && ch.ID == *choreID {
				feedBabyID = ch.ID
				break
			}
		}
		// Also check if the chore exists but is private and not in visible list (already handled by canView)
		if feedBabyID == 0 {
			return nil, fmt.Errorf("chore not found")
		}
	} else {
		for _, ch := range chores {
			if ch.HouseholdID == householdID && ch.Name == "Feed Baby" {
				feedBabyID = ch.ID
				break
			}
		}
	}
	if feedBabyID == 0 {
		return nil, nil
	}

	logs, err := s.fetchStatsLogs(ctx, householdID, log.StatsQuery{
		CandidateStart: start.Add(-48 * time.Hour), CandidateEnd: end.Add(48 * time.Hour),
		Start: &start, End: &end, ChoreID: feedBabyID,
	})
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
		if feedLogs[i].CompletedAt.Equal(feedLogs[j].CompletedAt) {
			return feedLogs[i].ID < feedLogs[j].ID
		}
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

// GetChoreSummary returns a period-scoped aggregate (count, amount, duration,
// per-member split) for a single chore. period is day|week|month|all; "all"
// counts every log ever recorded. The chore must belong to householdID.
func (s *Service) GetChoreSummary(ctx context.Context, householdID, choreID int64, period string, loc *time.Location) (*ChoreSummary, error) {
	if ok, err := s.canViewChoreForContext(ctx, householdID, choreID); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("chore not found")
	}
	ch, err := s.choreStore.GetChore(ctx, choreID)
	if err != nil {
		return nil, err
	}
	if ch.HouseholdID != householdID {
		return nil, fmt.Errorf("chore not found")
	}

	now := nowIn(loc)
	var start, end time.Time
	allTime := false
	switch period {
	case "day":
		y, m, d := now.Date()
		start = time.Date(y, m, d, 0, 0, 0, 0, loc)
		end = start.AddDate(0, 0, 1)
	case "month":
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		end = start.AddDate(0, 1, 0)
	case "all":
		allTime = true
	default: // "week"
		start = wkStart(now, loc)
		end = start.AddDate(0, 0, 7)
	}

	if _, ok := s.logStore.(aggregateReader); ok {
		return s.aggregateSummary(ctx, householdID, ch, start, end, allTime)
	}

	var logs []log.ChoreLog
	if allTime {
		epochStart := time.Unix(0, 0).UTC()
		farFuture := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
		readCtx, visible, accessErr := s.readContext(ctx, householdID)
		if accessErr != nil {
			return nil, accessErr
		}
		logs, err = s.logStore.ListLogsRange(readCtx, householdID, epochStart, farFuture)
		if err != nil {
			return nil, err
		}
		logs = filterLogs(logs, visible)
	} else {
		logs, err = s.fetchLogsInRange(ctx, householdID, start, end, loc)
	}
	if err != nil {
		return nil, err
	}

	summary := &ChoreSummary{
		ChoreID:    choreID,
		MetricType: ch.MetricType,
		MetricUnit: ch.MetricUnit,
		ByMember:   []LeaderboardEntry{},
	}
	byMember := map[int64]int{}
	for _, l := range logs {
		if l.ChoreID != choreID {
			continue
		}
		if !allTime && !logInRange(l, start, end, loc) {
			continue
		}
		summary.Count++
		byMember[l.UserID]++
		if len(l.IndicatorVolumes) > 0 {
			for _, v := range l.IndicatorVolumes {
				if v > 0 {
					summary.TotalML += v
				}
			}
		} else if l.VolumeML != nil && *l.VolumeML > 0 {
			summary.TotalML += *l.VolumeML
		}
		if l.DurationSeconds != nil {
			summary.TotalDuration += *l.DurationSeconds
		}
	}
	for uid, c := range byMember {
		summary.ByMember = append(summary.ByMember, LeaderboardEntry{UserID: uid, Count: c})
	}
	sort.Slice(summary.ByMember, func(i, j int) bool { return summary.ByMember[i].Count > summary.ByMember[j].Count })
	return summary, nil
}

// fetchLogsInRange fetches logs with widened bounds to account for timezone
// offsets, so that the caller can filter by local date in Go. It also filters
// to the viewer's visible chores when a viewer is present in ctx.
func (s *Service) fetchLogsInRange(ctx context.Context, householdID int64, start, end time.Time, loc *time.Location) ([]log.ChoreLog, error) {
	return s.fetchStatsLogs(ctx, householdID, log.StatsQuery{CandidateStart: start.Add(-48 * time.Hour), CandidateEnd: end.Add(48 * time.Hour)})
}

type statsLogReader interface {
	StatsLogs(context.Context, int64, log.StatsQuery) ([]log.ChoreLog, error)
}

func (s *Service) fetchStatsLogs(ctx context.Context, householdID int64, q log.StatsQuery) ([]log.ChoreLog, error) {
	readCtx, visible, err := s.readContext(ctx, householdID)
	if err != nil {
		return nil, err
	}
	if reader, ok := s.logStore.(statsLogReader); ok {
		return reader.StatsLogs(readCtx, householdID, q)
	}
	logs, err := s.logStore.ListLogsRange(readCtx, householdID, q.CandidateStart, q.CandidateEnd)
	if err != nil {
		return nil, err
	}
	logs = filterLogs(logs, visible)
	filtered := logs[:0]
	for _, l := range logs {
		if q.ChoreID != 0 && l.ChoreID != q.ChoreID {
			continue
		}
		if q.Start != nil && l.CompletedAt.Before(*q.Start) {
			continue
		}
		if q.End != nil && !l.CompletedAt.Before(*q.End) {
			continue
		}
		filtered = append(filtered, l)
	}
	return filtered, nil
}

func (s *Service) visibleChoresForContext(ctx context.Context, householdID int64) ([]ChoreInfo, error) {
	chores, err := s.choreStore.ListChores(ctx, householdID)
	if err != nil {
		return nil, err
	}
	visibleMap, err := s.visibleChoreIDsForContext(ctx, householdID)
	if err != nil {
		return nil, err
	}
	if visibleMap == nil {
		return chores, nil
	}
	var out []ChoreInfo
	for _, c := range chores {
		if _, ok := visibleMap[c.ID]; ok {
			out = append(out, c)
		}
	}
	return out, nil
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
