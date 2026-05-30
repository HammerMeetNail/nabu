package household

import (
	"context"
	"fmt"
)

type Service struct {
	store     Store
	authStore AuthStore
}

type AuthStore interface {
	SetUserHousehold(ctx context.Context, userID, householdID int64, role string) error
}

func NewService(store Store, authStore AuthStore) *Service {
	return &Service{store: store, authStore: authStore}
}

func (s *Service) CreateHousehold(ctx context.Context, name, initials string, ownerID int64) (Household, error) {
	if name == "" {
		return Household{}, fmt.Errorf("household name must not be empty")
	}
	if initials == "" {
		initials = GenerateInitials(name)
	}
	// Multi-household: allow creating even if already in a household
	hh, err := s.store.CreateHousehold(ctx, name, initials, ownerID)
	if err != nil {
		return Household{}, err
	}
	if s.authStore != nil {
		_ = s.authStore.SetUserHousehold(ctx, ownerID, hh.ID, RoleOwner)
	}
	return hh, nil
}

func (s *Service) GetHousehold(ctx context.Context, userID int64) (Household, []Member, error) {
	_, _, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return Household{}, nil, ErrNotFound
	}
	hh, cerr := s.store.GetUserHousehold(ctx, userID)
	if cerr != nil {
		return Household{}, nil, cerr
	}
	members, err := s.store.GetMembers(ctx, hh.ID)
	if err != nil {
		return hh, nil, err
	}
	return hh, members, nil
}

func (s *Service) UpdateHousehold(ctx context.Context, userID int64, name, initials string) error {
	if name == "" {
		return fmt.Errorf("name must not be empty")
	}
	if initials == "" {
		initials = GenerateInitials(name)
	}
	_, role, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return err
	}
	if role != RoleOwner && role != RoleAdmin {
		return ErrNotAuthorized
	}
	hh, err := s.store.GetUserHousehold(ctx, userID)
	if err != nil {
		return err
	}
	return s.store.UpdateHousehold(ctx, hh.ID, name, initials)
}

func (s *Service) CreateInvite(ctx context.Context, userID int64) (Invite, error) {
	hhID, role, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return Invite{}, err
	}
	if role != RoleOwner {
		return Invite{}, ErrNotAuthorized
	}
	code := GenerateInviteCode()
	return s.store.CreateInvite(ctx, hhID, userID, code, 0)
}

func (s *Service) GetInvites(ctx context.Context, userID int64) ([]Invite, error) {
	hhID, role, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return nil, err
	}
	if role != RoleOwner {
		return nil, ErrNotAuthorized
	}
	return s.store.GetInvites(ctx, hhID)
}

func (s *Service) DeleteInvite(ctx context.Context, userID, inviteID int64) error {
	_, role, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return err
	}
	if role != RoleOwner {
		return ErrNotAuthorized
	}
	return s.store.DeleteInvite(ctx, inviteID)
}

func (s *Service) JoinHousehold(ctx context.Context, userID int64, inviteCode string) (Household, error) {
	invite, err := s.store.GetInviteByCode(ctx, inviteCode)
	if err != nil && err != ErrInviteNotFound {
		return Household{}, err
	}

	// If the code wasn't found in the invites table, try the permanent household invite code.
	if err == ErrInviteNotFound {
		hh, hhErr := s.store.GetHouseholdByInviteCode(ctx, inviteCode)
		if hhErr != nil {
			return Household{}, ErrInviteNotFound
		}
		// Check if already a member of this specific household
		_, memberErr := s.store.GetMembershipForHousehold(ctx, userID, hh.ID)
		if memberErr == nil {
			return Household{}, ErrAlreadyMember
		}
		members, membErr := s.store.GetMembers(ctx, hh.ID)
		if membErr != nil {
			return Household{}, membErr
		}
		if len(members) >= MaxMembersPerHousehold {
			return Household{}, fmt.Errorf("household is full")
		}
		if addErr := s.store.AddMember(ctx, hh.ID, userID, RoleMember); addErr != nil {
			return Household{}, addErr
		}
		if s.authStore != nil {
			_ = s.authStore.SetUserHousehold(ctx, userID, hh.ID, RoleMember)
		}
		return hh, nil
	}

	// Check if already a member of this specific household
	_, memberErr := s.store.GetMembershipForHousehold(ctx, userID, invite.HouseholdID)
	if memberErr == nil {
		return Household{}, ErrAlreadyMember
	}

	members, err := s.store.GetMembers(ctx, invite.HouseholdID)
	if err != nil {
		return Household{}, err
	}
	if len(members) >= MaxMembersPerHousehold {
		return Household{}, fmt.Errorf("household is full")
	}

	if err := s.store.AddMember(ctx, invite.HouseholdID, userID, RoleMember); err != nil {
		return Household{}, err
	}
	if err := s.store.UseInvite(ctx, inviteCode); err != nil {
		return Household{}, err
	}

	hh, err := s.store.GetHousehold(ctx, invite.HouseholdID)
	if err != nil {
		return Household{}, err
	}
	if s.authStore != nil {
		_ = s.authStore.SetUserHousehold(ctx, userID, hh.ID, RoleMember)
	}
	return hh, nil
}

// ListUserHouseholds returns all households the user belongs to.
func (s *Service) ListUserHouseholds(ctx context.Context, userID int64) ([]HouseholdWithRole, error) {
	return s.store.ListUserHouseholds(ctx, userID)
}

// SwitchHousehold switches the user's active household.
func (s *Service) SwitchHousehold(ctx context.Context, userID, householdID int64) error {
	if err := s.store.SetActiveHousehold(ctx, userID, householdID); err != nil {
		return err
	}
	// Keep auth store in sync
	role, err := s.store.GetMembershipForHousehold(ctx, userID, householdID)
	if err != nil {
		return err
	}
	if s.authStore != nil {
		_ = s.authStore.SetUserHousehold(ctx, userID, householdID, role)
	}
	return nil
}

func (s *Service) UpdateMemberRole(ctx context.Context, actorUserID, targetUserID int64, newRole string) error {
	_, actorRole, err := s.store.GetMembership(ctx, actorUserID)
	if err != nil {
		return err
	}
	if actorRole != RoleOwner {
		return ErrNotAuthorized
	}
	if newRole != RoleAdmin && newRole != RoleMember {
		return fmt.Errorf("invalid role: %s", newRole)
	}
	hhID, targetRole, err := s.store.GetMembership(ctx, targetUserID)
	if err != nil {
		return err
	}
	if targetRole == RoleOwner {
		members, err := s.store.GetMembers(ctx, hhID)
		if err != nil {
			return err
		}
		owners := 0
		for _, m := range members {
			if m.Role == RoleOwner {
				owners++
			}
		}
		if owners <= 1 {
			return fmt.Errorf("cannot change the role of the last owner")
		}
	}
	return s.store.UpdateMemberRole(ctx, hhID, targetUserID, newRole)
}

func (s *Service) RemoveMember(ctx context.Context, actorUserID, targetUserID int64) error {
	_, actorRole, err := s.store.GetMembership(ctx, actorUserID)
	if err != nil {
		return err
	}
	if actorRole != RoleOwner {
		return ErrNotAuthorized
	}
	if actorUserID == targetUserID {
		return fmt.Errorf("use leave instead of remove for self")
	}
	hhID, _, cerr := s.store.GetMembership(ctx, targetUserID)
	if cerr != nil {
		return cerr
	}
	members, err := s.store.GetMembers(ctx, hhID)
	if err != nil {
		return err
	}
	owners := 0
	for _, m := range members {
		if m.Role == RoleOwner {
			owners++
		}
	}
	for _, m := range members {
		if m.UserID == targetUserID && m.Role == RoleOwner && owners <= 1 {
			return ErrLastOwner
		}
	}
	return s.store.RemoveMember(ctx, hhID, targetUserID)
}

func (s *Service) LeaveHousehold(ctx context.Context, userID int64) error {
	hhID, role, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return err
	}
	if role == RoleOwner {
		members, err := s.store.GetMembers(ctx, hhID)
		if err != nil {
			return err
		}
		for _, m := range members {
			if m.Role == RoleOwner && m.UserID != userID {
				return s.store.RemoveMember(ctx, hhID, userID)
			}
		}
		return ErrLastOwner
	}
	return s.store.RemoveMember(ctx, hhID, userID)
}

func (s *Service) TransferOwnership(ctx context.Context, currentOwnerID, newOwnerID int64) error {
	hhID, role, err := s.store.GetMembership(ctx, currentOwnerID)
	if err != nil {
		return err
	}
	if role != RoleOwner {
		return ErrNotAuthorized
	}
	_, err = s.store.GetMembershipForHousehold(ctx, newOwnerID, hhID)
	if err != nil {
		return ErrNotMember
	}
	if err := s.store.UpdateMemberRole(ctx, hhID, currentOwnerID, RoleAdmin); err != nil {
		return err
	}
	return s.store.UpdateMemberRole(ctx, hhID, newOwnerID, RoleOwner)
}
