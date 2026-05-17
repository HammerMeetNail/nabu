package userprefs

import "context"

// Service contains business logic for user preferences.
type Service struct {
	store Store
}

// NewService constructs a Service.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// GetPreferences returns the preferences for the given user.
func (s *Service) GetPreferences(ctx context.Context, userID int64) (Preferences, error) {
	return s.store.Get(ctx, userID)
}

// UpdateChoreOrder persists a new chore ordering for the user.
func (s *Service) UpdateChoreOrder(ctx context.Context, userID int64, choreOrder []int64) error {
	if choreOrder == nil {
		choreOrder = []int64{}
	}
	return s.store.Upsert(ctx, userID, Preferences{ChoreOrder: choreOrder})
}
