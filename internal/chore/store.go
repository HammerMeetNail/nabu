package chore

import (
	"context"
	"time"
)

type Chore struct {
	ID                  int64     `json:"id"`
	HouseholdID         int64     `json:"householdId"`
	Name                string    `json:"name"`
	Icon                string    `json:"icon"`
	Color               string    `json:"color"`
	SortOrder           int       `json:"sortOrder"`
	Category            string    `json:"category"`
	IsPredefined        bool      `json:"isPredefined"`
	PredefinedKey       string    `json:"predefinedKey"`
	CreatedBy           *int64    `json:"createdBy"`
	CreatedAt           time.Time `json:"createdAt"`
	IndicatorLabels     []string  `json:"indicatorLabels"`
	IndicatorDefaults   []string  `json:"indicatorDefaults"`
	HasVolumeML         bool      `json:"hasVolumeML"`
	FollowUpEnabled     bool      `json:"followUpEnabled"`
	LastFollowUpMinutes int       `json:"lastFollowUpMinutes"`
	HasRating           bool      `json:"hasRating"`
}

type Store interface {
	CreateChore(ctx context.Context, chore Chore) (Chore, error)
	GetChore(ctx context.Context, id int64) (Chore, error)
	ListChores(ctx context.Context, householdID int64) ([]Chore, error)
	UpdateChore(ctx context.Context, chore Chore) error
	DeleteChore(ctx context.Context, id int64) error
	ReorderChores(ctx context.Context, householdID int64, choreIDs []int64) error
	SeedPredefinedChores(ctx context.Context, householdID int64) error
}
