package userprefs

import "context"

// Preferences holds all user-specific settings.
type Preferences struct {
	// ChoreOrder is an ordered list of chore IDs reflecting the user's
	// preferred sort order in the pick-chore and quick-log sheets.
	// Chores absent from this list are appended after the ordered ones.
	ChoreOrder []int64 `json:"choreOrder"`

	// HiddenHomeChoreIDs is the set of chore IDs the user has removed from
	// their home grid.  The chores still exist in the household and are
	// accessible from the Chores tab; they are simply not shown on the home
	// screen for this user.
	HiddenHomeChoreIDs []int64 `json:"hiddenHomeChoreIds"`
}

// Store is the persistence interface for user preferences.
type Store interface {
	// Get returns the preferences for userID.  If no row exists yet it returns
	// a zero-value Preferences (empty ChoreOrder) without an error.
	Get(ctx context.Context, userID int64) (Preferences, error)

	// Upsert creates or replaces the preferences for userID.
	Upsert(ctx context.Context, userID int64, p Preferences) error
}
