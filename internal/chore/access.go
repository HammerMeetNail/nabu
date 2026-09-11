package chore

import (
	"context"
)

// MembershipReader is a narrow interface over household.Store for role lookup.
type MembershipReader interface {
	GetMembershipForHousehold(ctx context.Context, userID, householdID int64) (string, error)
}

// household role constants - duplicated to avoid import cycle.
// Must stay in sync with household.RoleOwner / RoleAdmin.
const (
	roleOwner = "owner"
	roleAdmin = "admin"
)

// CanView reports whether userID can see the given chore in householdID.
// householdID is the caller's active household, not derived from session role.
func (s *Service) CanView(ctx context.Context, userID, householdID int64, c Chore) (bool, error) {
	if c.HouseholdID != householdID {
		return false, nil
	}
	if s.memberships == nil {
		if c.Visibility == VisibilityAdmins {
			return false, ErrNotAdmin
		}
		return true, nil
	}
	role, err := s.memberships.GetMembershipForHousehold(ctx, userID, householdID)
	if err != nil {
		return false, err
	}
	if c.Visibility != VisibilityAdmins {
		return role == "member" || role == roleOwner || role == roleAdmin, nil
	}
	return role == roleOwner || role == roleAdmin, nil
}

// RequireAdmin checks that userID is owner/admin in householdID.
func (s *Service) RequireAdmin(ctx context.Context, userID, householdID int64) error {
	if s.memberships == nil {
		return nil
	}
	role, err := s.memberships.GetMembershipForHousehold(ctx, userID, householdID)
	if err != nil {
		return err
	}
	if role == roleOwner || role == roleAdmin {
		return nil
	}
	return ErrNotAdmin
}

// GetVisible returns the chore if the caller can view it, otherwise ErrNotFound
// (indistinguishable from an unknown ID).
func (s *Service) GetVisible(ctx context.Context, userID, householdID, choreID int64) (Chore, error) {
	c, err := s.store.GetChore(ctx, choreID)
	if err != nil {
		return Chore{}, err
	}
	ok, err := s.CanView(ctx, userID, householdID, c)
	if err != nil {
		return Chore{}, err
	}
	if !ok {
		return Chore{}, ErrNotFound
	}
	return c, nil
}

// RequireVisible is like GetVisible but returns ErrNotFound for any access failure,
// preserving the 404 response for hidden tasks.
func (s *Service) RequireVisible(ctx context.Context, userID, householdID, choreID int64) (Chore, error) {
	return s.GetVisible(ctx, userID, householdID, choreID)
}

// ListVisible returns chores in householdID visible to userID, ordered by sortOrder.
func (s *Service) ListVisible(ctx context.Context, userID, householdID int64) ([]Chore, error) {
	all, err := s.store.ListChores(ctx, householdID)
	if err != nil {
		return nil, err
	}
	// Resolve permissions once even when the collection is empty or contains
	// only shared chores. A failed permission lookup is never an empty success.
	role := "member"
	if s.memberships != nil {
		role, err = s.memberships.GetMembershipForHousehold(ctx, userID, householdID)
		if err != nil {
			return nil, err
		}
	}
	var visible []Chore
	for _, c := range all {
		if c.HouseholdID == householdID && (c.Visibility != VisibilityAdmins || role == roleOwner || role == roleAdmin) {
			visible = append(visible, c)
		}
	}
	if visible == nil {
		visible = []Chore{}
	}
	return visible, nil
}

// VisibleChoreIDs returns the set of chore IDs visible to the user, for
// filtering logs/schedules/stats.
func (s *Service) VisibleChoreIDs(ctx context.Context, userID, householdID int64) (map[int64]struct{}, error) {
	visible, err := s.ListVisible(ctx, userID, householdID)
	if err != nil {
		return nil, err
	}
	set := make(map[int64]struct{}, len(visible))
	for _, c := range visible {
		set[c.ID] = struct{}{}
	}
	return set, nil
}
