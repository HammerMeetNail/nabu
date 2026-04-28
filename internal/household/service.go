package household

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

func (s *Service) CreateHousehold(ctx context.Context, name string, ownerID int64) (Household, error) {
	if name == "" {
		return Household{}, fmt.Errorf("household name must not be empty")
	}
	_, _, err := s.store.GetMembership(ctx, ownerID)
	if err == nil {
		return Household{}, fmt.Errorf("user already belongs to a household")
	}
	return s.store.CreateHousehold(ctx, name, ownerID)
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

func (s *Service) UpdateHousehold(ctx context.Context, userID int64, name string) error {
	if name == "" {
		return fmt.Errorf("name must not be empty")
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
	return s.store.UpdateHousehold(ctx, hh.ID, name)
}

func (s *Service) CreateInvite(ctx context.Context, userID int64) (Invite, error) {
	hhID, role, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return Invite{}, err
	}
	if role != RoleOwner && role != RoleAdmin {
		return Invite{}, ErrNotAuthorized
	}
	code := GenerateInviteCode()
	return s.store.CreateInvite(ctx, hhID, userID, code, 0)
}

func (s *Service) GetInvites(ctx context.Context, userID int64) ([]Invite, error) {
	hhID, _, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.store.GetInvites(ctx, hhID)
}

func (s *Service) DeleteInvite(ctx context.Context, userID, inviteID int64) error {
	_, role, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return err
	}
	if role != RoleOwner && role != RoleAdmin {
		return ErrNotAuthorized
	}
	return s.store.DeleteInvite(ctx, inviteID)
}

func (s *Service) JoinHousehold(ctx context.Context, userID int64, inviteCode string) (Household, error) {
	_, _, err := s.store.GetMembership(ctx, userID)
	if err == nil {
		return Household{}, ErrAlreadyMember
	}

	invite, err := s.store.GetInviteByCode(ctx, inviteCode)
	if err != nil {
		return Household{}, err
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

	return s.store.GetHousehold(ctx, invite.HouseholdID)
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
	hhID, _, err := s.store.GetMembership(ctx, targetUserID)
	if err != nil {
		return err
	}
	return s.store.UpdateMemberRole(ctx, hhID, targetUserID, newRole)
}

func (s *Service) RemoveMember(ctx context.Context, actorUserID, targetUserID int64) error {
	_, actorRole, err := s.store.GetMembership(ctx, actorUserID)
	if err != nil {
		return err
	}
	if actorRole != RoleOwner && actorRole != RoleAdmin {
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
	_, _, err = s.store.GetMembership(ctx, newOwnerID)
	if err != nil {
		return ErrNotMember
	}
	if err := s.store.UpdateMemberRole(ctx, hhID, currentOwnerID, RoleAdmin); err != nil {
		return err
	}
	return s.store.UpdateMemberRole(ctx, hhID, newOwnerID, RoleOwner)
}
