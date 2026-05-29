package log_test

import (
	"context"
	"testing"
	"time"

	chorelog "github.com/dave/choresy/internal/log"
)

func TestMemoryStore_FindLog(t *testing.T) {
	store := chorelog.NewMemoryStore()
	ctx := context.Background()

	date := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	entry, err := store.CreateLog(ctx, chorelog.ChoreLog{
		HouseholdID: 1,
		UserID:      10,
		ChoreID:     5,
		CompletedAt: date,
		Note:        "test",
	})
	if err != nil {
		t.Fatalf("CreateLog: %v", err)
	}

	found, err := store.FindLog(ctx, 1, 5, date)
	if err != nil {
		t.Fatalf("FindLog: %v", err)
	}
	if found.ID != entry.ID {
		t.Errorf("FindLog ID = %d, want %d", found.ID, entry.ID)
	}
}

func TestMemoryStore_FindLogNotFound(t *testing.T) {
	store := chorelog.NewMemoryStore()
	ctx := context.Background()

	date := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	_, err := store.FindLog(ctx, 1, 999, date)
	if err == nil {
		t.Fatal("expected error for missing log")
	}
}

func TestMemoryStore_FindLogWrongDate(t *testing.T) {
	store := chorelog.NewMemoryStore()
	ctx := context.Background()

	date := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	store.CreateLog(ctx, chorelog.ChoreLog{
		HouseholdID: 1,
		UserID:      10,
		ChoreID:     5,
		CompletedAt: date,
	})

	otherDate := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	_, err := store.FindLog(ctx, 1, 5, otherDate)
	if err == nil {
		t.Fatal("expected error: log date should not match otherDate")
	}
}

// TestMemoryStore_GetLog_NilIndicators covers the nil-normalisation branch in
// GetLog (l.Indicators == nil → []string{}).
func TestMemoryStore_GetLog_NilIndicators(t *testing.T) {
	store := chorelog.NewMemoryStore()
	ctx := context.Background()

	// CreateLog stores the log as-is; Indicators will be nil
	entry, _ := store.CreateLog(ctx, chorelog.ChoreLog{
		HouseholdID: 1,
		ChoreID:     1,
		Indicators:  nil,
		CompletedAt: time.Now(),
	})
	got, err := store.GetLog(ctx, entry.ID)
	if err != nil {
		t.Fatalf("GetLog: %v", err)
	}
	if got.Indicators == nil {
		t.Error("GetLog should normalise nil Indicators to []string{}")
	}
	if len(got.Indicators) != 0 {
		t.Errorf("expected empty slice, got %v", got.Indicators)
	}
}

// TestMemoryStore_UpdateLog_NotFound covers the ErrNotFound path in UpdateLog.
func TestMemoryStore_UpdateLog_NotFound(t *testing.T) {
	store := chorelog.NewMemoryStore()
	err := store.UpdateLog(context.Background(), chorelog.ChoreLog{ID: 999})
	if err != chorelog.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestMemoryStore_UpdateLog_NilIndicators covers the nil branch in UpdateLog
// (log.Indicators == nil → existing.Indicators = []string{}).
func TestMemoryStore_UpdateLog_NilIndicators(t *testing.T) {
	store := chorelog.NewMemoryStore()
	ctx := context.Background()

	entry, _ := store.CreateLog(ctx, chorelog.ChoreLog{
		HouseholdID: 1,
		Indicators:  []string{"a"},
		CompletedAt: time.Now(),
	})
	err := store.UpdateLog(ctx, chorelog.ChoreLog{ID: entry.ID, Indicators: nil})
	if err != nil {
		t.Fatalf("UpdateLog: %v", err)
	}
	got, _ := store.GetLog(ctx, entry.ID)
	if got.Indicators == nil {
		t.Error("UpdateLog nil Indicators should be normalised to []string{}")
	}
	if len(got.Indicators) != 0 {
		t.Errorf("expected empty slice after nil update, got %v", got.Indicators)
	}
}

// TestMemoryStore_HistoryLogs_Empty covers the nil→[]ChoreLog{} normalisation
// when no logs match the requested range.
func TestMemoryStore_HistoryLogs_Empty(t *testing.T) {
	store := chorelog.NewMemoryStore()
	ctx := context.Background()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	result, hasOlder, err := store.HistoryLogs(ctx, 1, start, end)
	if err != nil {
		t.Fatalf("HistoryLogs: %v", err)
	}
	if result == nil {
		t.Error("HistoryLogs should return empty slice, not nil")
	}
	if hasOlder {
		t.Error("expected hasOlder=false with no logs")
	}
}

// TestMemoryStore_HistoryLogs_WithCompletedAt covers the CompletedAt fallback
// paths in logInRange and logBeforeRange (when LogDate is nil).
func TestMemoryStore_HistoryLogs_WithCompletedAt(t *testing.T) {
	store := chorelog.NewMemoryStore()
	ctx := context.Background()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	inRange := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	beforeRange := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	// Both logs have LogDate=nil so logInRange/logBeforeRange use CompletedAt
	store.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: 1, ChoreID: 1, CompletedAt: inRange})
	store.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: 1, ChoreID: 2, CompletedAt: beforeRange})

	result, hasOlder, err := store.HistoryLogs(ctx, 1, start, end)
	if err != nil {
		t.Fatalf("HistoryLogs: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 log in range, got %d", len(result))
	}
	if !hasOlder {
		t.Error("expected hasOlder=true (log exists before start)")
	}
}

// TestMemoryStore_LatestPerChore_NilIndicators covers the nil normalisation in
// LatestPerChore (l.Indicators == nil → []string{}).
func TestMemoryStore_LatestPerChore_NilIndicators(t *testing.T) {
	store := chorelog.NewMemoryStore()
	ctx := context.Background()

	store.CreateLog(ctx, chorelog.ChoreLog{
		HouseholdID: 1,
		ChoreID:     5,
		Indicators:  nil,
		CompletedAt: time.Now(),
	})
	result, err := store.LatestPerChore(ctx, 1)
	if err != nil {
		t.Fatalf("LatestPerChore: %v", err)
	}
	got, ok := result[5]
	if !ok {
		t.Fatal("expected entry for ChoreID=5")
	}
	if got.Indicators == nil {
		t.Error("LatestPerChore should normalise nil Indicators to []string{}")
	}
}

// TestMemoryStore_LatestPerChore_DifferentHousehold covers the
// `if l.HouseholdID != householdID { continue }` branch.
func TestMemoryStore_LatestPerChore_DifferentHousehold(t *testing.T) {
	store := chorelog.NewMemoryStore()
	ctx := context.Background()

	// Log for HH2 – must not appear in HH1 results
	store.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: 2, ChoreID: 5, CompletedAt: time.Now()})

	result, err := store.LatestPerChore(ctx, 1)
	if err != nil {
		t.Fatalf("LatestPerChore: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map for HH1, got entries: %v", result)
	}
}
