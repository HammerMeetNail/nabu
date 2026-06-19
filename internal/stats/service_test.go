package stats_test

import (
	"context"
	"testing"
	"time"

	chorelog "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/stats"
)

// stubChoreStore satisfies stats.choreStore (unexported interface, satisfied
// structurally from outside the package).
type stubChoreStore struct {
	chores []stats.ChoreInfo
}

func (s *stubChoreStore) GetChore(_ context.Context, id int64) (stats.ChoreInfo, error) {
	for _, c := range s.chores {
		if c.ID == id {
			return c, nil
		}
	}
	return stats.ChoreInfo{}, nil
}

func (s *stubChoreStore) ListChores(_ context.Context, _ int64) ([]stats.ChoreInfo, error) {
	return s.chores, nil
}

// seedLogs creates a log service and adds logs at specific times, returning the service.
func seedService(t *testing.T, logs []chorelog.ChoreLog) (*stats.Service, *stubChoreStore) {
	t.Helper()
	logStore := chorelog.NewMemoryStore()
	logSvc := chorelog.NewService(logStore)
	ctx := context.Background()
	for _, l := range logs {
		d := l.CompletedAt
		_, err := logSvc.LogChore(ctx, l.HouseholdID, l.UserID, l.ChoreID, nil, l.Note, l.Indicators, l.IndicatorVolumes, &d, l.SlotHour, &d, l.VolumeML, nil)
		if err != nil {
			t.Fatalf("seed log: %v", err)
		}
	}
	cs := &stubChoreStore{chores: []stats.ChoreInfo{
		{ID: 100, HouseholdID: 1, Name: "Dishes", Category: "kitchen"},
		{ID: 101, HouseholdID: 1, Name: "Vacuum", Category: "cleaning"},
	}}
	svc := stats.NewService(logStore, cs)
	return svc, cs
}

var utc = time.UTC

// ─── Leaderboard ─────────────────────────────────────────────────────────────

func TestGetMonthlyLeaderboard_Basic(t *testing.T) {
	now := time.Now().UTC()
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: time.Date(now.Year(), now.Month(), 2, 12, 0, 0, 0, utc)},
		{HouseholdID: 1, UserID: 10, ChoreID: 101, CompletedAt: time.Date(now.Year(), now.Month(), 3, 12, 0, 0, 0, utc)},
		{HouseholdID: 1, UserID: 20, ChoreID: 100, CompletedAt: time.Date(now.Year(), now.Month(), 4, 12, 0, 0, 0, utc)},
	}
	svc, _ := seedService(t, logs)
	entries, err := svc.GetMonthlyLeaderboard(context.Background(), 1, now.Year(), now.Month(), utc)
	if err != nil {
		t.Fatalf("GetMonthlyLeaderboard: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// User 10 has 2 logs so should be first
	if entries[0].UserID != 10 || entries[0].Count != 2 {
		t.Errorf("entries[0] = %+v, want user 10 count 2", entries[0])
	}
}

func TestGetWeeklyLeaderboard_EmptyWhenNoLogs(t *testing.T) {
	svc, _ := seedService(t, nil)
	entries, err := svc.GetWeeklyLeaderboard(context.Background(), 1, utc)
	if err != nil {
		t.Fatalf("GetWeeklyLeaderboard: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// ─── Streaks ──────────────────────────────────────────────────────────────────

func TestGetUserStreaks_CurrentStreak(t *testing.T) {
	today := time.Now().UTC()
	yesterday := today.AddDate(0, 0, -1)
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: time.Date(today.Year(), today.Month(), today.Day(), 12, 0, 0, 0, utc)},
		{HouseholdID: 1, UserID: 10, ChoreID: 101, CompletedAt: time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 12, 0, 0, 0, utc)},
	}
	svc, _ := seedService(t, logs)
	streaks, err := svc.GetUserStreaks(context.Background(), 1, 10, utc)
	if err != nil {
		t.Fatalf("GetUserStreaks: %v", err)
	}
	if streaks.Current < 2 {
		t.Errorf("Current = %d, want >= 2", streaks.Current)
	}
	if streaks.Longest < 2 {
		t.Errorf("Longest = %d, want >= 2", streaks.Longest)
	}
}

func TestGetUserStreaks_ZeroWhenNoLogs(t *testing.T) {
	svc, _ := seedService(t, nil)
	streaks, err := svc.GetUserStreaks(context.Background(), 1, 10, utc)
	if err != nil {
		t.Fatalf("GetUserStreaks: %v", err)
	}
	if streaks.Current != 0 {
		t.Errorf("Current = %d, want 0", streaks.Current)
	}
}

// ─── Heatmap ──────────────────────────────────────────────────────────────────

func TestGetHeatmap_CountsPerDay(t *testing.T) {
	d1 := time.Date(2026, 4, 10, 12, 0, 0, 0, utc)
	d2 := time.Date(2026, 4, 10, 14, 0, 0, 0, utc)
	d3 := time.Date(2026, 4, 11, 12, 0, 0, 0, utc)
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: d1},
		{HouseholdID: 1, UserID: 10, ChoreID: 101, CompletedAt: d2},
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: d3},
	}
	svc, _ := seedService(t, logs)
	start := time.Date(2026, 4, 9, 0, 0, 0, 0, utc)
	end := time.Date(2026, 4, 12, 0, 0, 0, 0, utc)
	cells, err := svc.GetHeatmap(context.Background(), 1, start, end, utc)
	if err != nil {
		t.Fatalf("GetHeatmap: %v", err)
	}
	byDate := map[string]int{}
	for _, c := range cells {
		byDate[c.Date] = c.Count
	}
	if byDate["2026-04-10"] != 2 {
		t.Errorf("2026-04-10 count = %d, want 2", byDate["2026-04-10"])
	}
	if byDate["2026-04-11"] != 1 {
		t.Errorf("2026-04-11 count = %d, want 1", byDate["2026-04-11"])
	}
}

// ─── Category Breakdown ───────────────────────────────────────────────────────

func TestGetCategoryBreakdown(t *testing.T) {
	d := time.Date(2026, 4, 10, 12, 0, 0, 0, utc)
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: d},
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: d},
		{HouseholdID: 1, UserID: 10, ChoreID: 101, CompletedAt: d},
	}
	svc, _ := seedService(t, logs)
	start := time.Date(2026, 4, 1, 0, 0, 0, 0, utc)
	end := time.Date(2026, 5, 1, 0, 0, 0, 0, utc)
	bd, err := svc.GetCategoryBreakdown(context.Background(), 1, start, end, utc)
	if err != nil {
		t.Fatalf("GetCategoryBreakdown: %v", err)
	}
	byCat := map[string]int{}
	for _, c := range bd {
		byCat[c.Category] = c.Count
	}
	if byCat["kitchen"] != 2 {
		t.Errorf("kitchen = %d, want 2", byCat["kitchen"])
	}
	if byCat["cleaning"] != 1 {
		t.Errorf("cleaning = %d, want 1", byCat["cleaning"])
	}
}

// ─── Busy Hours ───────────────────────────────────────────────────────────────

func TestGetBusyHours_Returns24Slots(t *testing.T) {
	d := time.Date(2026, 4, 10, 9, 0, 0, 0, utc)
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: d},
	}
	svc, _ := seedService(t, logs)
	start := time.Date(2026, 4, 1, 0, 0, 0, 0, utc)
	end := time.Date(2026, 5, 1, 0, 0, 0, 0, utc)
	hours, err := svc.GetBusyHours(context.Background(), 1, start, end, utc, nil, nil)
	if err != nil {
		t.Fatalf("GetBusyHours: %v", err)
	}
	if len(hours) != 24 {
		t.Errorf("expected 24 slots, got %d", len(hours))
	}
	for _, h := range hours {
		if h.Hour == 9 && h.Count != 1 {
			t.Errorf("hour 9 count = %d, want 1", h.Count)
		}
	}
}

func TestGetBusyHours_FilterByChore(t *testing.T) {
	d := time.Date(2026, 4, 10, 9, 0, 0, 0, utc)
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: d},
		{HouseholdID: 1, UserID: 10, ChoreID: 101, CompletedAt: d},
	}
	svc, _ := seedService(t, logs)
	start := time.Date(2026, 4, 1, 0, 0, 0, 0, utc)
	end := time.Date(2026, 5, 1, 0, 0, 0, 0, utc)

	cid := int64(100)
	hours, err := svc.GetBusyHours(context.Background(), 1, start, end, utc, &cid, nil)
	if err != nil {
		t.Fatalf("GetBusyHours: %v", err)
	}
	for _, h := range hours {
		if h.Hour == 9 && h.Count != 1 {
			t.Errorf("hour 9 count = %d, want 1 (filtered to chore 100)", h.Count)
		}
	}
}

func TestGetBusyHours_FilterByUser(t *testing.T) {
	d := time.Date(2026, 4, 10, 9, 0, 0, 0, utc)
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: d},
		{HouseholdID: 1, UserID: 20, ChoreID: 100, CompletedAt: d},
	}
	svc, _ := seedService(t, logs)
	start := time.Date(2026, 4, 1, 0, 0, 0, 0, utc)
	end := time.Date(2026, 5, 1, 0, 0, 0, 0, utc)

	uid := int64(10)
	hours, err := svc.GetBusyHours(context.Background(), 1, start, end, utc, nil, &uid)
	if err != nil {
		t.Fatalf("GetBusyHours: %v", err)
	}
	for _, h := range hours {
		if h.Hour == 9 && h.Count != 1 {
			t.Errorf("hour 9 count = %d, want 1 (filtered to user 10)", h.Count)
		}
	}
}

// ─── Weekly Recap ─────────────────────────────────────────────────────────────

func TestGetWeeklyRecap_TotalChores(t *testing.T) {
	now := time.Now().UTC()
	// Two logs this week for user 10
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: now},
		{HouseholdID: 1, UserID: 10, ChoreID: 101, CompletedAt: now},
		{HouseholdID: 1, UserID: 20, ChoreID: 100, CompletedAt: now},
	}
	svc, _ := seedService(t, logs)
	recap, err := svc.GetWeeklyRecap(context.Background(), 1, utc)
	if err != nil {
		t.Fatalf("GetWeeklyRecap: %v", err)
	}
	if recap.TotalChores != 3 {
		t.Errorf("TotalChores = %d, want 3", recap.TotalChores)
	}
	if recap.TopPerformer == nil {
		t.Fatal("expected non-nil TopPerformer")
	}
	if recap.TopPerformer.UserID != 10 {
		t.Errorf("TopPerformer = user %d, want 10", recap.TopPerformer.UserID)
	}
	if recap.MostActiveDay == "" {
		t.Error("MostActiveDay should not be empty")
	}
}

// ─── Chore Stats ──────────────────────────────────────────────────────────────

func TestGetChoreStats_WeekAndMonthCounts(t *testing.T) {
	now := time.Now().UTC()
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: now},
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: now},
	}
	svc, _ := seedService(t, logs)
	result, err := svc.GetChoreStats(context.Background(), 1, utc, nil, nil)
	if err != nil {
		t.Fatalf("GetChoreStats: %v", err)
	}
	found := false
	for _, cs := range result {
		if cs.ChoreID == 100 {
			found = true
			if cs.TotalThisWeek < 2 {
				t.Errorf("TotalThisWeek = %d, want >= 2", cs.TotalThisWeek)
			}
			if cs.TotalThisMonth < 2 {
				t.Errorf("TotalThisMonth = %d, want >= 2", cs.TotalThisMonth)
			}
		}
	}
	if !found {
		t.Error("chore 100 not found in chore stats")
	}
}

func TestGetChoreStats_VolumeHistory(t *testing.T) {
	now := time.Now().UTC()
	vol := 250
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: now, VolumeML: &vol},
	}
	svc, cs := seedService(t, logs)
	cs.chores[0].HasVolumeML = true

	result, err := svc.GetChoreStats(context.Background(), 1, utc, nil, nil)
	if err != nil {
		t.Fatalf("GetChoreStats: %v", err)
	}
	for _, s := range result {
		if s.ChoreID == 100 {
			if s.AvgVolume == nil {
				t.Error("AvgVolume should not be nil for volume chore")
			} else if *s.AvgVolume != 250 {
				t.Errorf("AvgVolume = %v, want 250", *s.AvgVolume)
			}
			if len(s.VolumeHistory) == 0 {
				t.Error("VolumeHistory should not be empty")
			}
		}
	}
}

// ─── Weekly Overview ──────────────────────────────────────────────────────────

func TestGetWeeklyOverview_Structure(t *testing.T) {
	now := time.Now().UTC()
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: now},
	}
	svc, _ := seedService(t, logs)
	ov, err := svc.GetWeeklyOverview(context.Background(), 1, 10, utc)
	if err != nil {
		t.Fatalf("GetWeeklyOverview: %v", err)
	}
	if ov.Leaderboard == nil {
		t.Error("Leaderboard should not be nil")
	}
	if ov.Breakdown == nil {
		t.Error("Breakdown should not be nil")
	}
}

func TestGetChoreTimeSeriesCrossHousehold(t *testing.T) {
	now := time.Now().UTC()
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: now},
	}
	svc, _ := seedService(t, logs)

	_, err := svc.GetChoreTimeSeries(context.Background(), 2, 100, "daily", utc)
	if err == nil {
		t.Fatal("GetChoreTimeSeries should reject chore from different household")
	}
}

// ─── Top Chores ───────────────────────────────────────────────────────────────

func TestGetTopChores_Basic(t *testing.T) {
	now := time.Now().UTC()
	midnight := now.Truncate(24 * time.Hour)
	if now.Hour() < 4 {
		t.Skip("skip: test requires UTC hour >= 4 to avoid day boundary")
	}
	ref := midnight.Add(3 * time.Hour) // 3am today
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: ref},
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: ref.Add(-30 * time.Minute)},
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: ref.Add(-2 * time.Hour)},
		{HouseholdID: 1, UserID: 10, ChoreID: 101, CompletedAt: ref.Add(-10 * time.Minute)},
		{HouseholdID: 1, UserID: 10, ChoreID: 101, CompletedAt: ref.Add(-3 * time.Hour)},
	}
	svc, _ := seedService(t, logs)

	entries, err := svc.GetTopChores(context.Background(), 1, 0, 5, "month", utc)
	if err != nil {
		t.Fatalf("GetTopChores: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].ChoreName != "Dishes" {
		t.Errorf("first entry should be Dishes (most monthly logs), got %s", entries[0].ChoreName)
	}
	if entries[0].Count != 3 {
		t.Errorf("Dishes Count = %d, want 3", entries[0].Count)
	}

	if entries[1].ChoreName != "Vacuum" {
		t.Errorf("second entry should be Vacuum, got %s", entries[1].ChoreName)
	}
	if entries[1].Count != 2 {
		t.Errorf("Vacuum Count = %d, want 2", entries[1].Count)
	}
}

func TestGetTopChores_PerUser(t *testing.T) {
	now := time.Now().UTC()
	midnight := now.Truncate(24 * time.Hour)
	if now.Hour() < 4 {
		t.Skip("skip: test requires UTC hour >= 4 to avoid day boundary")
	}
	ref := midnight.Add(3 * time.Hour)
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: ref},
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: ref.Add(-1 * time.Hour)},
		{HouseholdID: 1, UserID: 20, ChoreID: 101, CompletedAt: ref.Add(-30 * time.Minute)},
		{HouseholdID: 1, UserID: 20, ChoreID: 101, CompletedAt: ref.Add(-2 * time.Hour)},
	}
	svc, _ := seedService(t, logs)

	entries, err := svc.GetTopChores(context.Background(), 1, 10, 5, "month", utc)
	if err != nil {
		t.Fatalf("GetTopChores(user 10): %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for user 10, got %d", len(entries))
	}
	if entries[0].ChoreName != "Dishes" {
		t.Errorf("user 10 top chore should be Dishes, got %s", entries[0].ChoreName)
	}
	if entries[0].Count != 2 {
		t.Errorf("user 10 Dishes Count = %d, want 2", entries[0].Count)
	}

	entries2, err := svc.GetTopChores(context.Background(), 1, 20, 5, "month", utc)
	if err != nil {
		t.Fatalf("GetTopChores(user 20): %v", err)
	}
	if len(entries2) != 1 {
		t.Fatalf("expected 1 entry for user 20, got %d", len(entries2))
	}
	if entries2[0].ChoreName != "Vacuum" {
		t.Errorf("user 20 top chore should be Vacuum, got %s", entries2[0].ChoreName)
	}
}

func TestGetTopChores_Empty(t *testing.T) {
	svc, _ := seedService(t, nil)
	entries, err := svc.GetTopChores(context.Background(), 1, 0, 5, "month", utc)
	if err != nil {
		t.Fatalf("GetTopChores: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestGetTopChores_Limit(t *testing.T) {
	now := time.Now().UTC()
	midnight := now.Truncate(24 * time.Hour)
	if now.Hour() < 4 {
		t.Skip("skip: test requires UTC hour >= 4 to avoid day boundary")
	}
	ref := midnight.Add(3 * time.Hour) // 3am today

	cs := &stubChoreStore{chores: []stats.ChoreInfo{}}
	for i := int64(1); i <= 6; i++ {
		cs.chores = append(cs.chores, stats.ChoreInfo{
			ID: 200 + i, HouseholdID: 1, Name: "Chore" + string(rune('A'-1+i)),
		})
	}

	logStore := chorelog.NewMemoryStore()
	logSvc := chorelog.NewService(logStore)
	ctx := context.Background()
	for i, ch := range cs.chores {
		count := 6 - i
		for j := 0; j < count; j++ {
			d := ref.Add(time.Duration(-j) * time.Hour)
			_, err := logSvc.LogChore(ctx, 1, 10, ch.ID, nil, "", nil, nil, &d, nil, &d, nil, nil)
			if err != nil {
				t.Fatalf("seed log: %v", err)
			}
		}
	}

	svc := stats.NewService(logStore, cs)

	entries, err := svc.GetTopChores(context.Background(), 1, 0, 3, "month", utc)
	if err != nil {
		t.Fatalf("GetTopChores: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (limit), got %d", len(entries))
	}
	if entries[0].Count != 6 {
		t.Errorf("first entry Count = %d, want 6", entries[0].Count)
	}
	if entries[1].Count != 5 {
		t.Errorf("second entry Count = %d, want 5", entries[1].Count)
	}
	if entries[2].Count != 4 {
		t.Errorf("third entry Count = %d, want 4", entries[2].Count)
	}
}

func TestGetTopChores_Periods(t *testing.T) {
	now := time.Now().UTC()
	midnight := now.Truncate(24 * time.Hour)
	if now.Hour() < 4 {
		t.Skip("skip: test requires UTC hour >= 4 to avoid day boundary")
	}
	ref := midnight.Add(3 * time.Hour) // 3am today

	logs := []chorelog.ChoreLog{
		// Two Dishes logs today — today falls inside day/week/month/all.
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: ref},
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: ref.Add(-30 * time.Minute)},
		// One Vacuum log today.
		{HouseholdID: 1, UserID: 10, ChoreID: 101, CompletedAt: ref.Add(-2 * time.Hour)},
	}
	svc, _ := seedService(t, logs)

	// All four periods should accept the parameter and return the same ranking
	// (today's logs are inside every window).
	for _, period := range []string{"day", "week", "month", "all"} {
		entries, err := svc.GetTopChores(context.Background(), 1, 0, 5, period, utc)
		if err != nil {
			t.Fatalf("GetTopChores(%s): %v", period, err)
		}
		if len(entries) != 2 {
			t.Fatalf("period %s: expected 2 entries, got %d", period, len(entries))
		}
		if entries[0].ChoreName != "Dishes" {
			t.Fatalf("period %s: expected Dishes first, got %s", period, entries[0].ChoreName)
		}
		if entries[0].Count != 2 {
			t.Errorf("period %s: Dishes Count = %d, want 2", period, entries[0].Count)
		}
		if entries[1].Count != 1 {
			t.Errorf("period %s: Vacuum Count = %d, want 1", period, entries[1].Count)
		}
	}
}

func TestGetTopChores_PeriodBoundary(t *testing.T) {
	// Separates day vs week vs month vs all using a log that's deliberately
	// outside day/week but inside month — but only valid mid-month so the
	// 9-day-old log stays inside the current month.
	now := time.Now().UTC()
	midnight := now.Truncate(24 * time.Hour)
	if now.Hour() < 4 || now.Day() < 10 {
		t.Skip("skip: boundary test requires UTC hour >= 4 and day-of-month >= 10")
	}
	ref := midnight.Add(3 * time.Hour)

	logs := []chorelog.ChoreLog{
		// One Dishes log today.
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: ref},
		// One Dishes log 9 days ago — outside week, inside month (and all).
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: ref.Add(-9 * 24 * time.Hour)},
	}
	svc, _ := seedService(t, logs)

	day, _ := svc.GetTopChores(context.Background(), 1, 0, 5, "day", utc)
	if day[0].Count != 1 {
		t.Errorf("day Count = %d, want 1", day[0].Count)
	}
	week, _ := svc.GetTopChores(context.Background(), 1, 0, 5, "week", utc)
	if week[0].Count != 1 {
		t.Errorf("week Count = %d, want 1 (9-day-old log is outside the week)", week[0].Count)
	}
	month, _ := svc.GetTopChores(context.Background(), 1, 0, 5, "month", utc)
	if month[0].Count != 2 {
		t.Errorf("month Count = %d, want 2", month[0].Count)
	}
	all, _ := svc.GetTopChores(context.Background(), 1, 0, 5, "all", utc)
	if all[0].Count != 2 {
		t.Errorf("all Count = %d, want 2", all[0].Count)
	}
}

// ─── Feeding Gaps ────────────────────────────────────────────────────────────

func seedFeedingService(t *testing.T, logs []chorelog.ChoreLog) (*stats.Service, *stubChoreStore) {
	t.Helper()
	logStore := chorelog.NewMemoryStore()
	logSvc := chorelog.NewService(logStore)
	ctx := context.Background()
	for _, l := range logs {
		d := l.CompletedAt
		_, err := logSvc.LogChore(ctx, l.HouseholdID, l.UserID, l.ChoreID, nil, l.Note, l.Indicators, l.IndicatorVolumes, &d, l.SlotHour, &d, l.VolumeML, nil)
		if err != nil {
			t.Fatalf("seed feeding log: %v", err)
		}
	}
	cs := &stubChoreStore{chores: []stats.ChoreInfo{
		{ID: 200, HouseholdID: 1, Name: "Feed Baby", Category: "feeding", HasVolumeML: true},
		{ID: 201, HouseholdID: 1, Name: "Dishes", Category: "kitchen"},
	}}
	svc := stats.NewService(logStore, cs)
	return svc, cs
}

func TestGetFeedingGaps_Basic(t *testing.T) {
	base := time.Date(2026, 6, 9, 10, 0, 0, 0, utc)
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 200, CompletedAt: base, IndicatorVolumes: map[string]int{"🍼 formula": 120}},
		{HouseholdID: 1, UserID: 10, ChoreID: 200, CompletedAt: base.Add(45 * time.Minute), IndicatorVolumes: map[string]int{"🤱 breast": 60}},
		{HouseholdID: 1, UserID: 10, ChoreID: 200, CompletedAt: base.Add(3 * time.Hour), IndicatorVolumes: map[string]int{"🍼 formula": 120}},
		{HouseholdID: 1, UserID: 10, ChoreID: 200, CompletedAt: base.Add(3*time.Hour + 50*time.Minute), IndicatorVolumes: map[string]int{"🍼 formula": 30}},
	}
	svc, _ := seedFeedingService(t, logs)

	start := time.Date(2026, 6, 9, 0, 0, 0, 0, utc)
	end := time.Date(2026, 6, 10, 0, 0, 0, 0, utc)

	gaps, err := svc.GetFeedingGaps(context.Background(), 1, start, end, utc)
	if err != nil {
		t.Fatalf("GetFeedingGaps: %v", err)
	}
	if len(gaps) != 3 {
		t.Fatalf("expected 3 gaps, got %d", len(gaps))
	}
	if gaps[0].Hour != 10 || gaps[0].GapMinutes != 45 || gaps[0].PrecedingVolume != 120 || gaps[0].FollowUpVolume != 60 {
		t.Errorf("gap[0] = {hour:%d, gap:%d, prev:%d, vol:%d}, want {10, 45, 120, 60}", gaps[0].Hour, gaps[0].GapMinutes, gaps[0].PrecedingVolume, gaps[0].FollowUpVolume)
	}
	if gaps[1].Hour != 10 || gaps[1].GapMinutes != 135 || gaps[1].PrecedingVolume != 60 || gaps[1].FollowUpVolume != 120 {
		t.Errorf("gap[1] = {hour:%d, gap:%d, prev:%d, vol:%d}, want {10, 135, 60, 120}", gaps[1].Hour, gaps[1].GapMinutes, gaps[1].PrecedingVolume, gaps[1].FollowUpVolume)
	}
	if gaps[2].Hour != 13 || gaps[2].GapMinutes != 50 || gaps[2].PrecedingVolume != 120 || gaps[2].FollowUpVolume != 30 {
		t.Errorf("gap[2] = {hour:%d, gap:%d, prev:%d, vol:%d}, want {13, 50, 120, 30}", gaps[2].Hour, gaps[2].GapMinutes, gaps[2].PrecedingVolume, gaps[2].FollowUpVolume)
	}
}

func TestGetFeedingGaps_EmptyWhenNoFeedChore(t *testing.T) {
	logStore := chorelog.NewMemoryStore()
	cs := &stubChoreStore{chores: []stats.ChoreInfo{
		{ID: 201, HouseholdID: 1, Name: "Dishes", Category: "kitchen"},
	}}
	svc := stats.NewService(logStore, cs)

	gaps, err := svc.GetFeedingGaps(context.Background(), 1, time.Date(2026, 6, 1, 0, 0, 0, 0, utc), time.Date(2026, 7, 1, 0, 0, 0, 0, utc), utc)
	if err != nil {
		t.Fatalf("GetFeedingGaps: %v", err)
	}
	if gaps != nil {
		t.Errorf("expected nil gaps when no Feed Baby chore, got %v", gaps)
	}
}

func TestGetFeedingGaps_SingleLog(t *testing.T) {
	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 200, CompletedAt: time.Date(2026, 6, 9, 10, 0, 0, 0, utc), IndicatorVolumes: map[string]int{"🍼 formula": 120}},
	}
	svc, _ := seedFeedingService(t, logs)

	gaps, err := svc.GetFeedingGaps(context.Background(), 1, time.Date(2026, 6, 9, 0, 0, 0, 0, utc), time.Date(2026, 6, 10, 0, 0, 0, 0, utc), utc)
	if err != nil {
		t.Fatalf("GetFeedingGaps: %v", err)
	}
	if len(gaps) != 0 {
		t.Errorf("expected 0 gaps for single log, got %d", len(gaps))
	}
}
