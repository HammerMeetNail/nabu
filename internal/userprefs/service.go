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
// It reads existing preferences first so that other fields (e.g.
// HiddenHomeChoreIDs) are preserved.
func (s *Service) UpdateChoreOrder(ctx context.Context, userID int64, choreOrder []int64) error {
	if choreOrder == nil {
		choreOrder = []int64{}
	}
	prefs, err := s.store.Get(ctx, userID)
	if err != nil {
		return err
	}
	prefs.ChoreOrder = choreOrder
	return s.store.Upsert(ctx, userID, prefs)
}

// UpdateHiddenHomeChores replaces the list of chore IDs that are hidden from
// the user's home grid.  The chores still exist and are accessible from the
// Chores tab.
func (s *Service) UpdateHiddenHomeChores(ctx context.Context, userID int64, hiddenIDs []int64) error {
	if hiddenIDs == nil {
		hiddenIDs = []int64{}
	}
	prefs, err := s.store.Get(ctx, userID)
	if err != nil {
		return err
	}
	prefs.HiddenHomeChoreIDs = hiddenIDs
	return s.store.Upsert(ctx, userID, prefs)
}

// UpdateTimezone persists the user's IANA timezone name (e.g.
// "America/New_York") for stats aggregation.  An empty string means UTC.
func (s *Service) UpdateTimezone(ctx context.Context, userID int64, tz string) error {
	prefs, err := s.store.Get(ctx, userID)
	if err != nil {
		return err
	}
	prefs.Timezone = tz
	return s.store.Upsert(ctx, userID, prefs)
}
