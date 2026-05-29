package log_test

import (
	"context"
	"testing"
	"time"

	chorelog "github.com/dave/choresy/internal/log"
)

// ─── Service tests ────────────────────────────────────────────────────────────

func TestLogService_LogChore_Basic(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	l, err := svc.LogChore(ctx, 1, 10, 100, "done", nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("LogChore: %v", err)
	}
	if l.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if l.HouseholdID != 1 || l.UserID != 10 || l.ChoreID != 100 {
		t.Errorf("wrong IDs: %+v", l)
	}
	if l.Note != "done" {
		t.Errorf("Note = %q", l.Note)
	}
	if l.Indicators == nil {
		t.Error("Indicators must not be nil")
	}
	if l.CompletedAt.IsZero() {
		t.Error("CompletedAt must not be zero")
	}
}

func TestLogService_LogChore_WithCompletedAt(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	ts := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)
	l, err := svc.LogChore(ctx, 1, 10, 100, "", nil, nil, nil, &ts, nil)
	if err != nil {
		t.Fatalf("LogChore: %v", err)
	}
	if !l.CompletedAt.Equal(ts) {
		t.Errorf("CompletedAt = %v, want %v", l.CompletedAt, ts)
	}
}

func TestLogService_LogChore_WithDate_NoCompletedAt(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	d := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	l, err := svc.LogChore(ctx, 1, 10, 100, "", nil, &d, nil, nil, nil)
	if err != nil {
		t.Fatalf("LogChore: %v", err)
	}
	// Should be noon UTC on that date
	if l.CompletedAt.Hour() != 12 {
		t.Errorf("expected noon UTC, got hour=%d", l.CompletedAt.Hour())
	}
	if l.CompletedAt.Day() != 15 {
		t.Errorf("expected day 15, got %d", l.CompletedAt.Day())
	}
}

func TestLogService_LogChore_WithSlotHour(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	hour := 8
	l, err := svc.LogChore(ctx, 1, 10, 100, "", nil, nil, &hour, nil, nil)
	if err != nil {
		t.Fatalf("LogChore: %v", err)
	}
	if l.SlotHour == nil || *l.SlotHour != 8 {
		t.Errorf("SlotHour = %v, want 8", l.SlotHour)
	}
}

func TestLogService_LogChore_WithVolumeML(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	vol := 150
	l, err := svc.LogChore(ctx, 1, 10, 100, "", nil, nil, nil, nil, &vol)
	if err != nil {
		t.Fatalf("LogChore: %v", err)
	}
	if l.VolumeML == nil || *l.VolumeML != 150 {
		t.Errorf("VolumeML = %v, want 150", l.VolumeML)
	}
}

func TestLogService_UpdateLog(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	l, _ := svc.LogChore(ctx, 1, 10, 100, "original", []string{"a"}, nil, nil, nil, nil)

	newTime := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	newUID := int64(20)
	hour := 9
	vol := 200
	logDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	err := svc.UpdateLog(ctx, l.ID, "updated", []string{"b", "c"}, &vol, &newUID, &newTime, &hour, &logDate)
	if err != nil {
		t.Fatalf("UpdateLog: %v", err)
	}

	day := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	logs, _ := svc.GetDayLogs(ctx, 1, day)
	if len(logs) == 0 {
		t.Fatal("expected log on updated date")
	}
	got := logs[0]
	if got.Note != "updated" {
		t.Errorf("Note = %q", got.Note)
	}
	if len(got.Indicators) != 2 {
		t.Errorf("Indicators = %v, want 2", got.Indicators)
	}
	if got.UserID != 20 {
		t.Errorf("UserID = %d, want 20", got.UserID)
	}
	if got.SlotHour == nil || *got.SlotHour != 9 {
		t.Errorf("SlotHour = %v, want 9", got.SlotHour)
	}
	if got.VolumeML == nil || *got.VolumeML != 200 {
		t.Errorf("VolumeML = %v, want 200", got.VolumeML)
	}
}

func TestLogService_UpdateLog_NotFound(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	err := svc.UpdateLog(context.Background(), 9999, "", nil, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing log")
	}
}

func TestLogService_UndoLog(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	l, _ := svc.LogChore(ctx, 1, 10, 100, "", nil, nil, nil, nil, nil)
	err := svc.UndoLog(ctx, 1, l.ID)
	if err != nil {
		t.Fatalf("UndoLog: %v", err)
	}

	today := time.Now().UTC()
	todayLogs, _ := svc.GetTodayLogs(ctx, 1)
	for _, tl := range todayLogs {
		if tl.ID == l.ID {
			t.Errorf("log %d should be gone after undo (today=%v)", l.ID, today)
		}
	}
}

func TestLogService_UndoLog_WrongHousehold(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	l, _ := svc.LogChore(ctx, 1, 10, 100, "", nil, nil, nil, nil, nil)
	err := svc.UndoLog(ctx, 2, l.ID) // wrong household
	if err == nil {
		t.Fatal("expected error undoing log from another household")
	}
}

func TestLogService_UndoLog_NotFound(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	err := svc.UndoLog(context.Background(), 1, 9999)
	if err == nil {
		t.Fatal("expected error for missing log")
	}
}

func TestLogService_GetTodayLogs(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	_, _ = svc.LogChore(ctx, 1, 10, 100, "", nil, nil, nil, nil, nil)
	_, _ = svc.LogChore(ctx, 1, 10, 101, "", nil, nil, nil, nil, nil)

	logs, err := svc.GetTodayLogs(ctx, 1)
	if err != nil {
		t.Fatalf("GetTodayLogs: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(logs))
	}
}

func TestLogService_GetDayLogs(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	day1 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)

	_, _ = svc.LogChore(ctx, 1, 10, 100, "", nil, &day1, nil, nil, nil)
	_, _ = svc.LogChore(ctx, 1, 10, 101, "", nil, &day2, nil, nil, nil)

	logs, err := svc.GetDayLogs(ctx, 1, day1)
	if err != nil {
		t.Fatalf("GetDayLogs: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log for day1, got %d", len(logs))
	}
	if logs[0].ChoreID != 100 {
		t.Errorf("wrong chore: %d", logs[0].ChoreID)
	}
}

func TestLogService_GetWeekLogs(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	start := time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC) // Tuesday
	inRange := time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC)
	outRange := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)

	_, _ = svc.LogChore(ctx, 1, 10, 100, "", nil, &inRange, nil, nil, nil)
	_, _ = svc.LogChore(ctx, 1, 10, 101, "", nil, &outRange, nil, nil, nil)

	logs, err := svc.GetWeekLogs(ctx, 1, start)
	if err != nil {
		t.Fatalf("GetWeekLogs: %v", err)
	}
	if len(logs) != 1 || logs[0].ChoreID != 100 {
		t.Errorf("expected 1 in-range log, got %v", logs)
	}
}

func TestLogService_GetMonthLogs(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	apr := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	may := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	_, _ = svc.LogChore(ctx, 1, 10, 100, "", nil, &apr, nil, nil, nil)
	_, _ = svc.LogChore(ctx, 1, 10, 101, "", nil, &may, nil, nil, nil)

	logs, err := svc.GetMonthLogs(ctx, 1, 2026, time.April)
	if err != nil {
		t.Fatalf("GetMonthLogs: %v", err)
	}
	if len(logs) != 1 || logs[0].ChoreID != 100 {
		t.Errorf("expected 1 April log, got %v", logs)
	}
}

func TestLogService_GetDailySummary(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	day := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	_, _ = svc.LogChore(ctx, 1, 10, 100, "", nil, &day, nil, nil, nil)
	_, _ = svc.LogChore(ctx, 1, 11, 101, "", nil, &day, nil, nil, nil)

	summary, err := svc.GetDailySummary(ctx, 1, day)
	if err != nil {
		t.Fatalf("GetDailySummary: %v", err)
	}
	if summary.TotalChores != 2 {
		t.Errorf("TotalChores = %d, want 2", summary.TotalChores)
	}
	if summary.ByUser[10] != 1 || summary.ByUser[11] != 1 {
		t.Errorf("ByUser = %v", summary.ByUser)
	}
}

func TestLogService_DailySummaryFromLogs(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	day := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)

	logs := []chorelog.ChoreLog{
		{ID: 1, UserID: 10, ChoreID: 100},
		{ID: 2, UserID: 10, ChoreID: 101},
		{ID: 3, UserID: 20, ChoreID: 102},
	}
	summary := svc.DailySummaryFromLogs(day, logs)
	if summary.TotalChores != 3 {
		t.Errorf("TotalChores = %d, want 3", summary.TotalChores)
	}
	if summary.ByUser[10] != 2 {
		t.Errorf("user 10 count = %d, want 2", summary.ByUser[10])
	}
	if summary.Date != "2026-04-10" {
		t.Errorf("Date = %q", summary.Date)
	}
}

func TestLogService_LatestPerChore(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	earlier := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)

	_, _ = svc.LogChore(ctx, 1, 10, 100, "old", nil, &earlier, nil, &earlier, nil)
	_, _ = svc.LogChore(ctx, 1, 10, 100, "new", nil, &later, nil, &later, nil)
	_, _ = svc.LogChore(ctx, 1, 10, 200, "only", nil, &later, nil, &later, nil)

	result, err := svc.LatestPerChore(ctx, 1)
	if err != nil {
		t.Fatalf("LatestPerChore: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if result[100].Note != "new" {
		t.Errorf("chore 100 latest note = %q, want 'new'", result[100].Note)
	}
}

func TestLogService_GetHistoryLogs(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)

	_, _ = svc.LogChore(ctx, 1, 10, 100, "old", nil, &old, nil, &old, nil)
	_, _ = svc.LogChore(ctx, 1, 10, 101, "mid", nil, &mid, nil, &mid, nil)
	_, _ = svc.LogChore(ctx, 1, 10, 102, "recent", nil, &recent, nil, &recent, nil)

	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	logs, hasMore, err := svc.GetHistoryLogs(ctx, 1, start, end)
	if err != nil {
		t.Fatalf("GetHistoryLogs: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 logs in range, got %d", len(logs))
	}
	if !hasMore {
		t.Error("expected hasMore=true (old log exists before start)")
	}
}

func TestLogService_UpdateLog_NilIndicators(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	l, _ := svc.LogChore(ctx, 1, 10, 100, "original", []string{"a"}, nil, nil, nil, nil)
	err := svc.UpdateLog(ctx, l.ID, "updated", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateLog with nil indicators: %v", err)
	}
	got, _ := svc.GetTodayLogs(ctx, 1)
	for _, g := range got {
		if g.ID == l.ID && g.Indicators == nil {
			t.Error("indicators should be normalised to []string{}, not nil")
		}
	}
}

func TestLogService_GetHistoryLogs_NoMore(t *testing.T) {
	svc := chorelog.NewService(chorelog.NewMemoryStore())
	ctx := context.Background()

	d := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	_, _ = svc.LogChore(ctx, 1, 10, 100, "", nil, &d, nil, &d, nil)

	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	_, hasMore, err := svc.GetHistoryLogs(ctx, 1, start, end)
	if err != nil {
		t.Fatalf("GetHistoryLogs: %v", err)
	}
	if hasMore {
		t.Error("expected hasMore=false when no older logs exist")
	}
}
