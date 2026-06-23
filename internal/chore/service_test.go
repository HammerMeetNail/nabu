package chore_test

import (
	"context"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/chore"
)

// ─── Service tests ────────────────────────────────────────────────────────────

func TestService_CreateChore_Basic(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	ctx := context.Background()

	c, err := svc.CreateChore(ctx, 1, 10, "Wash Dishes", "🍽️", "#3B82F6", "cleaning", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateChore: %v", err)
	}
	if c.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if c.Name != "Wash Dishes" {
		t.Errorf("Name = %q, want %q", c.Name, "Wash Dishes")
	}
	if c.Icon != "🍽️" {
		t.Errorf("Icon = %q, want %q", c.Icon, "🍽️")
	}
	if c.Color != "#3B82F6" {
		t.Errorf("Color = %q", c.Color)
	}
	if c.Category != "cleaning" {
		t.Errorf("Category = %q", c.Category)
	}
	if c.IsPredefined {
		t.Error("custom chore must not be predefined")
	}
	if c.IndicatorLabels == nil {
		t.Error("IndicatorLabels must not be nil")
	}
}

func TestService_CreateChore_Defaults(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	ctx := context.Background()

	c, err := svc.CreateChore(ctx, 1, 10, "My Chore", "", "", "", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateChore: %v", err)
	}
	if c.Icon == "" {
		t.Error("expected default icon")
	}
	if c.Color == "" {
		t.Error("expected default color")
	}
	if c.Category == "" {
		t.Error("expected default category")
	}
}

func TestService_CreateChore_EmptyNameError(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	_, err := svc.CreateChore(context.Background(), 1, 10, "", "", "", "", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestService_ListChores_Empty(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	chores, err := svc.ListChores(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListChores: %v", err)
	}
	if len(chores) != 0 {
		t.Fatalf("expected 0 chores, got %d", len(chores))
	}
}

func TestService_ListChores_MultipleHouseholds(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	ctx := context.Background()

	_, _ = svc.CreateChore(ctx, 1, 10, "HH1 Chore", "", "", "", nil, nil, nil)
	_, _ = svc.CreateChore(ctx, 2, 20, "HH2 Chore", "", "", "", nil, nil, nil)

	h1, _ := svc.ListChores(ctx, 1)
	h2, _ := svc.ListChores(ctx, 2)

	if len(h1) != 1 || h1[0].Name != "HH1 Chore" {
		t.Errorf("h1 = %v", h1)
	}
	if len(h2) != 1 || h2[0].Name != "HH2 Chore" {
		t.Errorf("h2 = %v", h2)
	}
}

func TestService_GetChore(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	ctx := context.Background()

	created, _ := svc.CreateChore(ctx, 1, 10, "Cat Feeding", "🐱", "", "", nil, nil, nil)

	got, err := svc.GetChore(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetChore: %v", err)
	}
	if got.Name != "Cat Feeding" {
		t.Errorf("Name = %q", got.Name)
	}
}

func TestService_GetChore_NotFound(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	_, err := svc.GetChore(context.Background(), 9999)
	if err == nil {
		t.Fatal("expected error for missing chore")
	}
}

func TestService_UpdateChore_PartialUpdate(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	ctx := context.Background()

	c, _ := svc.CreateChore(ctx, 1, 10, "Old Name", "🐱", "#000", "cleaning", nil, nil, nil)

	err := svc.UpdateChore(ctx, c.ID, 1, "New Name", "", "", "", nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateChore: %v", err)
	}

	updated, _ := svc.GetChore(ctx, c.ID)
	if updated.Name != "New Name" {
		t.Errorf("Name = %q, want %q", updated.Name, "New Name")
	}
	// Non-updated fields must be preserved
	if updated.Icon != "🐱" {
		t.Errorf("Icon changed: %q", updated.Icon)
	}
	if updated.Color != "#000" {
		t.Errorf("Color changed: %q", updated.Color)
	}
}

func TestService_UpdateChore_UpdateIndicators(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	ctx := context.Background()

	c, _ := svc.CreateChore(ctx, 1, 10, "Task", "", "", "", []string{"label1"}, nil, nil)
	err := svc.UpdateChore(ctx, c.ID, 1, "", "", "", "", []string{"new1", "new2"}, nil, nil)
	if err != nil {
		t.Fatalf("UpdateChore: %v", err)
	}
	updated, _ := svc.GetChore(ctx, c.ID)
	if len(updated.IndicatorLabels) != 2 {
		t.Errorf("IndicatorLabels = %v, want 2 elements", updated.IndicatorLabels)
	}
}

func TestService_UpdateChore_NotFound(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	err := svc.UpdateChore(context.Background(), 9999, 1, "X", "", "", "", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing chore")
	}
}

func TestService_DeleteChore(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	ctx := context.Background()

	c, _ := svc.CreateChore(ctx, 1, 10, "Temp", "", "", "", nil, nil, nil)
	err := svc.DeleteChore(ctx, c.ID, 1)
	if err != nil {
		t.Fatalf("DeleteChore: %v", err)
	}

	_, err = svc.GetChore(ctx, c.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestService_DeleteChore_Predefined(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	ctx := context.Background()

	// Seed predefined chores then try to delete one
	_ = svc.SeedDefaultChores(ctx, 1)
	chores, _ := svc.ListChores(ctx, 1)
	var predefined chore.Chore
	found := false
	for _, c := range chores {
		if c.IsPredefined {
			predefined = c
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no predefined chore found after seed")
	}
	err := svc.DeleteChore(ctx, predefined.ID, 1)
	if err == nil {
		t.Fatal("expected error when deleting predefined chore")
	}
}

func TestService_DeleteChore_NotFound(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	err := svc.DeleteChore(context.Background(), 9999, 1)
	if err == nil {
		t.Fatal("expected error for missing chore")
	}
}

func TestService_ReorderChores(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	ctx := context.Background()

	c1, _ := svc.CreateChore(ctx, 1, 10, "A", "", "", "", nil, nil, nil)
	c2, _ := svc.CreateChore(ctx, 1, 10, "B", "", "", "", nil, nil, nil)
	c3, _ := svc.CreateChore(ctx, 1, 10, "C", "", "", "", nil, nil, nil)

	err := svc.ReorderChores(ctx, 1, []int64{c3.ID, c1.ID, c2.ID})
	if err != nil {
		t.Fatalf("ReorderChores: %v", err)
	}

	chores, _ := svc.ListChores(ctx, 1)
	if len(chores) != 3 {
		t.Fatalf("expected 3 chores, got %d", len(chores))
	}
	// c3 was placed first, should have lowest sort order
	if chores[0].ID != c3.ID {
		t.Errorf("expected c3 first, got ID %d", chores[0].ID)
	}
}

func TestService_GetSystemDefaults(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	defaults := svc.GetSystemDefaults()
	if len(defaults) == 0 {
		t.Fatal("expected non-empty system defaults")
	}
	for _, d := range defaults {
		if d.Name == "" {
			t.Error("default chore has empty name")
		}
		if d.Icon == "" {
			t.Error("default chore has empty icon")
		}
		if !d.IsPredefined {
			t.Errorf("default chore %q must have IsPredefined=true", d.Name)
		}
	}
}

func TestService_SeedDefaultChores(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	ctx := context.Background()

	if err := svc.SeedDefaultChores(ctx, 1); err != nil {
		t.Fatalf("SeedDefaultChores: %v", err)
	}

	chores, err := svc.ListChores(ctx, 1)
	if err != nil {
		t.Fatalf("ListChores: %v", err)
	}
	defaults := svc.GetSystemDefaults()
	if len(chores) != len(defaults) {
		t.Errorf("got %d chores, want %d", len(chores), len(defaults))
	}
	// Idempotent: seeding again must not create duplicates
	if err := svc.SeedDefaultChores(ctx, 1); err != nil {
		t.Fatalf("second SeedDefaultChores: %v", err)
	}
	chores2, _ := svc.ListChores(ctx, 1)
	if len(chores2) != len(chores) {
		t.Errorf("seeding twice changed count: %d -> %d", len(chores), len(chores2))
	}
}

func TestService_RestoreDefaultChore(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	ctx := context.Background()

	_ = svc.SeedDefaultChores(ctx, 1)
	chores, _ := svc.ListChores(ctx, 1)

	// Find a predefined chore
	var target chore.Chore
	for _, c := range chores {
		if c.IsPredefined {
			target = c
			break
		}
	}
	originalName := target.Name

	// Mutate it via UpdateChore
	_ = svc.UpdateChore(ctx, target.ID, 1, "Modified Name", "", "", "", nil, nil, nil)

	// Restore it
	err := svc.RestoreDefaultChore(ctx, target.ID, 1)
	if err != nil {
		t.Fatalf("RestoreDefaultChore: %v", err)
	}

	restored, _ := svc.GetChore(ctx, target.ID)
	if restored.Name != originalName {
		t.Errorf("Name after restore = %q, want %q", restored.Name, originalName)
	}
}

func TestService_RestoreDefaultChore_NotPredefined(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	ctx := context.Background()

	c, _ := svc.CreateChore(ctx, 1, 10, "Custom", "", "", "", nil, nil, nil)
	err := svc.RestoreDefaultChore(ctx, c.ID, 1)
	if err == nil {
		t.Fatal("expected error restoring non-predefined chore")
	}
}

func TestService_RestoreDefaultChore_NotFound(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	err := svc.RestoreDefaultChore(context.Background(), 9999, 1)
	if err == nil {
		t.Fatal("expected error for missing chore")
	}
}

func TestService_UpdateChore_WithIconColorCategory(t *testing.T) {
	svc := chore.NewService(chore.NewMemoryStore())
	ctx := context.Background()

	c, _ := svc.CreateChore(ctx, 1, 10, "Sweep", "🧹", "#000", "cleaning", nil, nil, nil)

	// Pass non-empty icon, color, and category – covers all three update branches
	err := svc.UpdateChore(ctx, c.ID, 1, "", "🫧", "#AABBCC", "hygiene", nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateChore: %v", err)
	}

	updated, _ := svc.GetChore(ctx, c.ID)
	if updated.Icon != "🫧" {
		t.Errorf("Icon = %q, want 🫧", updated.Icon)
	}
	if updated.Color != "#AABBCC" {
		t.Errorf("Color = %q, want #AABBCC", updated.Color)
	}
	if updated.Category != "hygiene" {
		t.Errorf("Category = %q, want hygiene", updated.Category)
	}
	// Name should remain unchanged since we passed ""
	if updated.Name != "Sweep" {
		t.Errorf("Name changed to %q, want Sweep", updated.Name)
	}
}

// ─── MemoryStore direct tests ─────────────────────────────────────────────────

func TestMemoryStore_DuplicateName(t *testing.T) {
	store := chore.NewMemoryStore()
	ctx := context.Background()

	_, _ = store.CreateChore(ctx, chore.Chore{HouseholdID: 1, Name: "Dishes"})
	_, err := store.CreateChore(ctx, chore.Chore{HouseholdID: 1, Name: "Dishes"})
	if err != chore.ErrDuplicateName {
		t.Errorf("expected ErrDuplicateName, got %v", err)
	}
}

func TestMemoryStore_DuplicateName_DifferentHousehold(t *testing.T) {
	store := chore.NewMemoryStore()
	ctx := context.Background()

	_, _ = store.CreateChore(ctx, chore.Chore{HouseholdID: 1, Name: "Dishes"})
	_, err := store.CreateChore(ctx, chore.Chore{HouseholdID: 2, Name: "Dishes"})
	if err != nil {
		t.Errorf("same name in different household should be allowed, got %v", err)
	}
}

func TestMemoryStore_UpdateChore_NotFound(t *testing.T) {
	store := chore.NewMemoryStore()
	err := store.UpdateChore(context.Background(), chore.Chore{ID: 999})
	if err != chore.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_DeleteChore_NotFound(t *testing.T) {
	store := chore.NewMemoryStore()
	err := store.DeleteChore(context.Background(), 999)
	if err != chore.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_ReorderChores_IgnoresOtherHouseholds(t *testing.T) {
	store := chore.NewMemoryStore()
	ctx := context.Background()

	c1, _ := store.CreateChore(ctx, chore.Chore{HouseholdID: 1, Name: "A"})
	c2, _ := store.CreateChore(ctx, chore.Chore{HouseholdID: 2, Name: "B"})

	// Reorder for HH1 with HH2's ID mixed in (should be ignored)
	err := store.ReorderChores(ctx, 1, []int64{c1.ID, c2.ID})
	if err != nil {
		t.Fatalf("ReorderChores: %v", err)
	}
	// HH2 chore sort order must not have changed
	got, _ := store.GetChore(ctx, c2.ID)
	if got.SortOrder != 0 {
		t.Errorf("HH2 chore SortOrder changed to %d", got.SortOrder)
	}
}
