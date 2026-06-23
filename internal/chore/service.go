package chore

import (
	"context"
	"fmt"
	"strconv"

	"github.com/HammerMeetNail/nabu/internal/audit"
)

type Service struct {
	store       Store
	auditLogger audit.Logger
}

func NewService(store Store) *Service {
	return &Service{store: store, auditLogger: audit.NopLogger{}}
}

// SetAuditLogger attaches a sink for chore mutation events. A nil logger is a
// no-op (the service keeps its default NopLogger).
func (s *Service) SetAuditLogger(logger audit.Logger) {
	if logger != nil {
		s.auditLogger = logger
	}
}

func (s *Service) logAudit(ctx context.Context, event string, attrs map[string]string) {
	audit.Emit(ctx, s.auditLogger, event, attrs)
}

func idStr(id int64) string { return strconv.FormatInt(id, 10) }

func (s *Service) CreateChore(ctx context.Context, householdID int64, userID int64, name, icon, color, category string, indicatorLabels, indicatorDefaults []string, followUpEnabled *bool) (Chore, error) {
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
	fu := true
	if followUpEnabled != nil {
		fu = *followUpEnabled
	}
	created, err := s.store.CreateChore(ctx, Chore{
		HouseholdID:       householdID,
		Name:              name,
		Icon:              icon,
		Color:             color,
		Category:          category,
		IsPredefined:      false,
		CreatedBy:         &userID,
		IndicatorLabels:   indicatorLabels,
		IndicatorDefaults: indicatorDefaults,
		FollowUpEnabled:   fu,
	})
	if err != nil {
		return Chore{}, err
	}
	s.logAudit(ctx, "chore.created", map[string]string{
		"household_id": idStr(householdID),
		"chore_id":     idStr(created.ID),
		"name":         name,
	})
	return created, nil
}

func (s *Service) ListChores(ctx context.Context, householdID int64) ([]Chore, error) {
	return s.store.ListChores(ctx, householdID)
}

func (s *Service) GetChore(ctx context.Context, choreID int64) (Chore, error) {
	return s.store.GetChore(ctx, choreID)
}

func (s *Service) UpdateChore(ctx context.Context, choreID int64, householdID int64, name, icon, color, category string, indicatorLabels, indicatorDefaults []string, followUpEnabled *bool) error {
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
	if followUpEnabled != nil {
		existing.FollowUpEnabled = *followUpEnabled
	}
	if err := s.store.UpdateChore(ctx, existing); err != nil {
		return err
	}
	s.logAudit(ctx, "chore.updated", map[string]string{
		"household_id": idStr(householdID),
		"chore_id":     idStr(choreID),
	})
	return nil
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
	if err := s.store.DeleteChore(ctx, choreID); err != nil {
		return err
	}
	s.logAudit(ctx, "chore.deleted", map[string]string{
		"household_id": idStr(householdID),
		"chore_id":     idStr(choreID),
	})
	return nil
}

func (s *Service) ReorderChores(ctx context.Context, householdID int64, choreIDs []int64) error {
	if err := s.store.ReorderChores(ctx, householdID, choreIDs); err != nil {
		return err
	}
	s.logAudit(ctx, "chore.reordered", map[string]string{
		"household_id": idStr(householdID),
		"chore_count":  idStr(int64(len(choreIDs))),
	})
	return nil
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
			existing.HasRating = pc.HasRating
			existing.FollowUpEnabled = true
			if existing.IndicatorLabels == nil {
				existing.IndicatorLabels = []string{}
			}
			if existing.IndicatorDefaults == nil {
				existing.IndicatorDefaults = []string{}
			}
			if err := s.store.UpdateChore(ctx, existing); err != nil {
				return err
			}
			s.logAudit(ctx, "chore.default_restored", map[string]string{
				"household_id": idStr(householdID),
				"chore_id":     idStr(choreID),
			})
			return nil
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
			HasRating:         pc.HasRating,
			FollowUpEnabled:   true,
		})
	}
	return result
}

func (s *Service) SeedDefaultChores(ctx context.Context, householdID int64) error {
	if err := s.store.SeedPredefinedChores(ctx, householdID); err != nil {
		return err
	}
	s.logAudit(ctx, "chore.defaults_seeded", map[string]string{
		"household_id": idStr(householdID),
	})
	return nil
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
	{Name: "Read Book", Icon: "📖", Color: "#8B5CF6", Category: "personal", SortOrder: 13, HasRating: true},
	{Name: "Watch Movie", Icon: "🎬", Color: "#EF4444", Category: "personal", SortOrder: 14, HasRating: true},
}
