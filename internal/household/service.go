package household

import (
	"context"
	"fmt"
	"strconv"

	"github.com/HammerMeetNail/nabu/internal/audit"
)

type Service struct {
	store       Store
	authStore   AuthStore
	auditLogger audit.Logger
}

type AuthStore interface {
	SetUserHousehold(ctx context.Context, userID, householdID int64, role string) error
}

func NewService(store Store, authStore AuthStore) *Service {
	return &Service{
		store:       store,
		authStore:   authStore,
		auditLogger: audit.NopLogger{},
	}
}

// SetAuditLogger attaches a sink for household membership and configuration
// events. If logger is nil the call is a no-op (the service keeps its default
// NopLogger).
func (s *Service) SetAuditLogger(logger audit.Logger) {
	if logger != nil {
		s.auditLogger = logger
	}
}

// logAudit records a household event, merging the actor from ctx when the
// caller did not supply user_id/household_id explicitly. Household service
// methods always have the actor's userID as an explicit parameter (they are the
// authenticated principal), so they pass it directly; this still benefits from
// role enrichment via context when available.
func (s *Service) logAudit(ctx context.Context, event string, attrs map[string]string) {
	audit.Emit(ctx, s.auditLogger, event, attrs)
}

func formatID(id int64) string { return strconv.FormatInt(id, 10) }

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
	s.logAudit(ctx, "household.created", map[string]string{
		"user_id":      formatID(ownerID),
		"household_id": formatID(hh.ID),
		"name":         name,
	})
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
	if err := s.store.UpdateHousehold(ctx, hh.ID, name, initials); err != nil {
		return err
	}
	s.logAudit(ctx, "household.updated", map[string]string{
		"user_id":      formatID(userID),
		"household_id": formatID(hh.ID),
		"name":         name,
	})
	return nil
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
	invite, err := s.store.CreateInvite(ctx, hhID, userID, code, 0)
	if err != nil {
		return Invite{}, err
	}
	s.logAudit(ctx, "household.invite_created", map[string]string{
		"user_id":      formatID(userID),
		"household_id": formatID(hhID),
		"invite_id":    formatID(invite.ID),
	})
	return invite, nil
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
	actorHHID, role, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return err
	}
	if role != RoleOwner {
		return ErrNotAuthorized
	}
	invite, err := s.store.GetInviteByID(ctx, inviteID)
	if err != nil {
		return err
	}
	if invite.HouseholdID != actorHHID {
		return ErrNotAuthorized
	}
	if err := s.store.DeleteInvite(ctx, inviteID); err != nil {
		return err
	}
	s.logAudit(ctx, "household.invite_deleted", map[string]string{
		"user_id":      formatID(userID),
		"household_id": formatID(actorHHID),
		"invite_id":    formatID(inviteID),
	})
	return nil
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
		s.logAudit(ctx, "household.member_joined", map[string]string{
			"user_id":       formatID(userID),
			"household_id":  formatID(hh.ID),
			"invite_method": "permanent_code",
		})
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
	s.logAudit(ctx, "household.member_joined", map[string]string{
		"user_id":       formatID(userID),
		"household_id":  formatID(hh.ID),
		"invite_method": "invite_code",
	})
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
	actorHHID, actorRole, err := s.store.GetMembership(ctx, actorUserID)
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
	// Prevent cross-household role manipulation.
	if hhID != actorHHID {
		return ErrNotAuthorized
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
	if err := s.store.UpdateMemberRole(ctx, hhID, targetUserID, newRole); err != nil {
		return err
	}
	s.logAudit(ctx, "household.member_role_changed", map[string]string{
		"user_id":         formatID(actorUserID),
		"target_user_id":  formatID(targetUserID),
		"household_id":    formatID(hhID),
		"new_role":        newRole,
	})
	return nil
}

func (s *Service) RemoveMember(ctx context.Context, actorUserID, targetUserID int64) error {
	actorHHID, actorRole, err := s.store.GetMembership(ctx, actorUserID)
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
	// Prevent cross-household member removal.
	if hhID != actorHHID {
		return ErrNotAuthorized
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
	if err := s.store.RemoveMember(ctx, hhID, targetUserID); err != nil {
		return err
	}
	s.logAudit(ctx, "household.member_removed", map[string]string{
		"user_id":        formatID(actorUserID),
		"target_user_id": formatID(targetUserID),
		"household_id":   formatID(actorHHID),
	})
	return nil
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
				if err := s.store.RemoveMember(ctx, hhID, userID); err != nil {
					return err
				}
				s.logAudit(ctx, "household.member_left", map[string]string{
					"user_id":      formatID(userID),
					"household_id": formatID(hhID),
				})
				return nil
			}
		}
		return ErrLastOwner
	}
	if err := s.store.RemoveMember(ctx, hhID, userID); err != nil {
		return err
	}
	s.logAudit(ctx, "household.member_left", map[string]string{
		"user_id":      formatID(userID),
		"household_id": formatID(hhID),
	})
	return nil
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
	if err := s.store.UpdateMemberRole(ctx, hhID, newOwnerID, RoleOwner); err != nil {
		return err
	}
	s.logAudit(ctx, "household.ownership_transferred", map[string]string{
		"user_id":        formatID(currentOwnerID),
		"target_user_id": formatID(newOwnerID),
		"household_id":   formatID(hhID),
	})
	return nil
}
