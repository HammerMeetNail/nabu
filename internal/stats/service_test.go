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
		_, err := logSvc.LogChore(ctx, l.HouseholdID, l.UserID, l.ChoreID, l.Note, l.Indicators, l.IndicatorVolumes, &d, l.SlotHour, &d, l.VolumeML)
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

	entries, err := svc.GetTopChores(context.Background(), 1, 0, 5, utc)
	if err != nil {
		t.Fatalf("GetTopChores: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].ChoreName != "Dishes" {
		t.Errorf("first entry should be Dishes (most monthly logs), got %s", entries[0].ChoreName)
	}
	if entries[0].ThisMonth != 3 {
		t.Errorf("Dishes ThisMonth = %d, want 3", entries[0].ThisMonth)
	}
	if entries[0].Today != 3 {
		t.Errorf("Dishes Today = %d, want 3", entries[0].Today)
	}

	if entries[1].ChoreName != "Vacuum" {
		t.Errorf("second entry should be Vacuum, got %s", entries[1].ChoreName)
	}
	if entries[1].ThisMonth != 2 {
		t.Errorf("Vacuum ThisMonth = %d, want 2", entries[1].ThisMonth)
	}
	if entries[1].Today != 2 {
		t.Errorf("Vacuum Today = %d, want 2", entries[1].Today)
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

	entries, err := svc.GetTopChores(context.Background(), 1, 10, 5, utc)
	if err != nil {
		t.Fatalf("GetTopChores(user 10): %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for user 10, got %d", len(entries))
	}
	if entries[0].ChoreName != "Dishes" {
		t.Errorf("user 10 top chore should be Dishes, got %s", entries[0].ChoreName)
	}
	if entries[0].ThisMonth != 2 {
		t.Errorf("user 10 Dishes ThisMonth = %d, want 2", entries[0].ThisMonth)
	}

	entries2, err := svc.GetTopChores(context.Background(), 1, 20, 5, utc)
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
	entries, err := svc.GetTopChores(context.Background(), 1, 0, 5, utc)
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
			_, err := logSvc.LogChore(ctx, 1, 10, ch.ID, "", nil, nil, &d, nil, &d, nil)
			if err != nil {
				t.Fatalf("seed log: %v", err)
			}
		}
	}

	svc := stats.NewService(logStore, cs)

	entries, err := svc.GetTopChores(context.Background(), 1, 0, 3, utc)
	if err != nil {
		t.Fatalf("GetTopChores: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (limit), got %d", len(entries))
	}
	if entries[0].ThisMonth != 6 {
		t.Errorf("first entry ThisMonth = %d, want 6", entries[0].ThisMonth)
	}
	if entries[1].ThisMonth != 5 {
		t.Errorf("second entry ThisMonth = %d, want 5", entries[1].ThisMonth)
	}
	if entries[2].ThisMonth != 4 {
		t.Errorf("third entry ThisMonth = %d, want 4", entries[2].ThisMonth)
	}
}

func TestGetTopChores_WeekAndDayCounts(t *testing.T) {
	now := time.Now().UTC()
	midnight := now.Truncate(24 * time.Hour)
	if now.Hour() < 4 {
		t.Skip("skip: test requires UTC hour >= 4 to avoid day boundary")
	}
	ref := midnight.Add(3 * time.Hour) // 3am today

	logs := []chorelog.ChoreLog{
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: ref},
		{HouseholdID: 1, UserID: 10, ChoreID: 100, CompletedAt: ref.Add(-30 * time.Minute)},
		{HouseholdID: 1, UserID: 10, ChoreID: 101, CompletedAt: ref.Add(-2 * time.Hour)},
	}
	svc, _ := seedService(t, logs)

	entries, err := svc.GetTopChores(context.Background(), 1, 0, 5, utc)
	if err != nil {
		t.Fatalf("GetTopChores: %v", err)
	}
	if len(entries) < 1 {
		t.Fatal("expected at least 1 entry")
	}
	if entries[0].ChoreName != "Dishes" {
		t.Fatalf("expected Dishes first (2 logs vs 1 for Vacuum), got %s", entries[0].ChoreName)
	}
	if entries[0].ThisMonth != 2 {
		t.Errorf("Dishes ThisMonth = %d, want 2", entries[0].ThisMonth)
	}
	if entries[0].Today != 2 {
		t.Errorf("Dishes Today = %d, want 2", entries[0].Today)
	}
	if entries[1].ThisMonth != 1 {
		t.Errorf("Vacuum ThisMonth = %d, want 1", entries[1].ThisMonth)
	}
}
