package chore

import (
	"context"
	"fmt"
)

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) CreateChore(ctx context.Context, householdID int64, userID int64, name, icon, color, category string, indicatorLabels, indicatorDefaults []string) (Chore, error) {
	if name == "" {
		return Chore{}, fmt.Errorf("name must not be empty")
	}
	if icon == "" {
		icon = "📋"
	}
	if color == "" {
		color = "#2E86AB"
	}
	if category == "" {
		category = "custom"
	}
	if indicatorLabels == nil {
		indicatorLabels = []string{}
	}
	if indicatorDefaults == nil {
		indicatorDefaults = []string{}
	}
	return s.store.CreateChore(ctx, Chore{
		HouseholdID:       householdID,
		Name:              name,
		Icon:              icon,
		Color:             color,
		Category:          category,
		IsPredefined:      false,
		CreatedBy:         &userID,
		IndicatorLabels:   indicatorLabels,
		IndicatorDefaults: indicatorDefaults,
	})
}

func (s *Service) ListChores(ctx context.Context, householdID int64) ([]Chore, error) {
	return s.store.ListChores(ctx, householdID)
}

func (s *Service) GetChore(ctx context.Context, choreID int64) (Chore, error) {
	return s.store.GetChore(ctx, choreID)
}

func (s *Service) UpdateChore(ctx context.Context, choreID int64, householdID int64, name, icon, color, category string, indicatorLabels, indicatorDefaults []string) error {
	existing, err := s.store.GetChore(ctx, choreID)
	if err != nil {
		return err
	}
	if existing.HouseholdID != householdID {
		return fmt.Errorf("chore not found")
	}
	if name != "" {
		existing.Name = name
	}
	if icon != "" {
		existing.Icon = icon
	}
	if color != "" {
		existing.Color = color
	}
	if category != "" {
		existing.Category = category
	}
	if indicatorLabels != nil {
		existing.IndicatorLabels = indicatorLabels
	}
	if indicatorDefaults != nil {
		existing.IndicatorDefaults = indicatorDefaults
	}
	return s.store.UpdateChore(ctx, existing)
}

func (s *Service) DeleteChore(ctx context.Context, choreID int64, householdID int64) error {
	chore, err := s.store.GetChore(ctx, choreID)
	if err != nil {
		return err
	}
	if chore.HouseholdID != householdID {
		return fmt.Errorf("chore not found")
	}
	if chore.IsPredefined {
		return fmt.Errorf("cannot delete predefined chores")
	}
	return s.store.DeleteChore(ctx, choreID)
}

func (s *Service) ReorderChores(ctx context.Context, householdID int64, choreIDs []int64) error {
	return s.store.ReorderChores(ctx, householdID, choreIDs)
}

func (s *Service) RestoreDefaultChore(ctx context.Context, choreID int64, householdID int64) error {
	existing, err := s.store.GetChore(ctx, choreID)
	if err != nil {
		return err
	}
	if existing.HouseholdID != householdID {
		return fmt.Errorf("chore not found")
	}
	if !existing.IsPredefined || existing.PredefinedKey == "" {
		return fmt.Errorf("chore is not a predefined chore")
	}
	for _, pc := range PredefinedChores {
		if pc.Name == existing.PredefinedKey {
			existing.Name = pc.Name
			existing.Icon = pc.Icon
			existing.Color = pc.Color
			existing.Category = pc.Category
			existing.IndicatorLabels = pc.IndicatorLabels
			existing.IndicatorDefaults = pc.IndicatorDefaults
			existing.HasVolumeML = pc.HasVolumeML
			if existing.IndicatorLabels == nil {
				existing.IndicatorLabels = []string{}
			}
			if existing.IndicatorDefaults == nil {
				existing.IndicatorDefaults = []string{}
			}
			return s.store.UpdateChore(ctx, existing)
		}
	}
	return fmt.Errorf("original predefined chore definition not found")
}

func (s *Service) GetSystemDefaults() []Chore {
	var result []Chore
	for _, pc := range PredefinedChores {
		result = append(result, Chore{
			Name:              pc.Name,
			Icon:              pc.Icon,
			Color:             pc.Color,
			Category:          pc.Category,
			IsPredefined:      true,
			SortOrder:         pc.SortOrder,
			IndicatorLabels:   pc.IndicatorLabels,
			IndicatorDefaults: pc.IndicatorDefaults,
			HasVolumeML:       pc.HasVolumeML,
		})
	}
	return result
}

func (s *Service) SeedDefaultChores(ctx context.Context, householdID int64) error {
	return s.store.SeedPredefinedChores(ctx, householdID)
}

var PredefinedChores = []Chore{
	{Name: "Feed Cats", Icon: "🐱", Color: "#F59E0B", Category: "feeding", SortOrder: 0},
	{Name: "Feed Baby", Icon: "🍼", Color: "#EC4899", Category: "feeding", SortOrder: 1, HasVolumeML: true, IndicatorLabels: []string{"🍼 formula", "🤱 breast"}, IndicatorDefaults: []string{"🍼 formula"}},
	{Name: "Change Baby", Icon: "👶", Color: "#8B5CF6", Category: "care", SortOrder: 2, IndicatorLabels: []string{"💩 poo", "💛 pee"}, IndicatorDefaults: []string{"💛 pee"}},
	{Name: "Water Plants", Icon: "🌱", Color: "#10B981", Category: "plants", SortOrder: 3},
	{Name: "Clean Litter Box", Icon: "🧹", Color: "#6366F1", Category: "cleaning", SortOrder: 4},
	{Name: "Take Out Trash", Icon: "🗑️", Color: "#6B7280", Category: "cleaning", SortOrder: 5},
	{Name: "Wash Dishes", Icon: "🍽️", Color: "#3B82F6", Category: "cleaning", SortOrder: 6},
	{Name: "Vacuum", Icon: "🧹", Color: "#06B6D4", Category: "cleaning", SortOrder: 7},
	{Name: "Laundry", Icon: "👕", Color: "#F97316", Category: "cleaning", SortOrder: 8},
	{Name: "Walk Dog", Icon: "🐕", Color: "#EF4444", Category: "care", SortOrder: 9},
	{Name: "Make Bed", Icon: "🛏️", Color: "#14B8A6", Category: "cleaning", SortOrder: 10},
	{Name: "Baby Bath", Icon: "🛀", Color: "#60A5FA", Category: "care", SortOrder: 11},
	{Name: "Cat Meds", Icon: "💊", Color: "#A78BFA", Category: "care", SortOrder: 12},
}
