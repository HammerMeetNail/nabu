package schedule

import (
	"context"
	"testing"
)

func TestMemoryStore_CreateAndGet(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	sch := ChoreSchedule{
		HouseholdID:   1,
		ChoreID:       10,
		FrequencyType: "daily",
		IsActive:      true,
	}
	created, err := store.Create(ctx, sch)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected non-zero ID after Create")
	}
	if created.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}

	got, err := store.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ChoreID != 10 {
		t.Errorf("ChoreID = %d, want 10", got.ChoreID)
	}
}

func TestMemoryStore_Get_NotFound(t *testing.T) {
	store := NewMemoryStore()
	_, err := store.Get(context.Background(), 9999)
	if err == nil {
		t.Fatal("expected error for missing schedule")
	}
}

func TestMemoryStore_ListByHousehold(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	_, _ = store.Create(ctx, ChoreSchedule{HouseholdID: 1, ChoreID: 10, FrequencyType: "daily"})
	_, _ = store.Create(ctx, ChoreSchedule{HouseholdID: 1, ChoreID: 11, FrequencyType: "weekly"})
	_, _ = store.Create(ctx, ChoreSchedule{HouseholdID: 2, ChoreID: 20, FrequencyType: "daily"})

	list, err := store.ListByHousehold(ctx, 1)
	if err != nil {
		t.Fatalf("ListByHousehold: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 schedules for household 1, got %d", len(list))
	}

	list2, _ := store.ListByHousehold(ctx, 2)
	if len(list2) != 1 {
		t.Errorf("expected 1 schedule for household 2, got %d", len(list2))
	}

	list3, _ := store.ListByHousehold(ctx, 999)
	if len(list3) != 0 {
		t.Errorf("expected 0 schedules for household 999, got %d", len(list3))
	}
}

func TestMemoryStore_Update(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	created, _ := store.Create(ctx, ChoreSchedule{HouseholdID: 1, ChoreID: 10, FrequencyType: "daily", IsActive: true})

	created.FrequencyType = "weekly"
	created.IsActive = false
	updated, err := store.Update(ctx, created)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.FrequencyType != "weekly" {
		t.Errorf("FrequencyType = %q, want 'weekly'", updated.FrequencyType)
	}
	if updated.IsActive {
		t.Error("IsActive should be false after update")
	}
	if updated.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestMemoryStore_Update_NotFound(t *testing.T) {
	store := NewMemoryStore()
	_, err := store.Update(context.Background(), ChoreSchedule{ID: 9999})
	if err == nil {
		t.Fatal("expected error for missing schedule")
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	created, _ := store.Create(ctx, ChoreSchedule{HouseholdID: 1, ChoreID: 10, FrequencyType: "daily"})
	if err := store.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := store.Get(ctx, created.ID)
	if err == nil {
		t.Fatal("expected error: schedule should be deleted")
	}
}

func TestMemoryStore_Delete_NotFound(t *testing.T) {
	store := NewMemoryStore()
	err := store.Delete(context.Background(), 9999)
	if err == nil {
		t.Fatal("expected error for missing schedule")
	}
}

func TestMemoryStore_ConcurrentIDsUnique(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	ids := map[int64]bool{}
	for i := 0; i < 10; i++ {
		sch, _ := store.Create(ctx, ChoreSchedule{HouseholdID: 1, ChoreID: int64(i), FrequencyType: "daily"})
		if ids[sch.ID] {
			t.Fatalf("duplicate ID %d", sch.ID)
		}
		ids[sch.ID] = true
	}
}
