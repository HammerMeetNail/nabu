package household

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/auth"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
)

type Service struct {
	store         Store
	inTransaction bool
	auditLogger   audit.Logger
	logStore      chorelog.Store
	userStore     interface {
		GetUserByID(context.Context, int64) (auth.User, error)
	}
}

type AuthStore interface {
	SetMembershipResolver(auth.MembershipResolver)
}

// Per-field caps (audit finding #10). The server is the authority even though
// the clients constrain these fields in the UI.
const (
	maxNameRunes    = 60
	maxInitialsRune = 8
)

// validateNameInitials enforces household name/initials caps on create and
// update. It rejects with a clear error rather than truncating.
func validateNameInitials(name, initials string) error {
	if utf8.RuneCountInString(name) == 0 {
		return fmt.Errorf("%w: household name must not be empty", ErrInvalidInput)
	}
	if utf8.RuneCountInString(name) > maxNameRunes {
		return fmt.Errorf("%w: household name must be %d characters or fewer", ErrInvalidInput, maxNameRunes)
	}
	if initials != "" && utf8.RuneCountInString(initials) > maxInitialsRune {
		return fmt.Errorf("%w: initials must be %d characters or fewer", ErrInvalidInput, maxInitialsRune)
	}
	return nil
}

func NewService(store Store, authStore AuthStore) *Service {
	if authStore != nil {
		authStore.SetMembershipResolver(func(ctx context.Context, userID int64) (*int64, string, error) {
			householdID, role, err := store.GetMembership(ctx, userID)
			if errors.Is(err, ErrNotFound) {
				return nil, "", nil
			}
			if err != nil {
				return nil, "", err
			}
			return &householdID, role, nil
		})
	}
	if memory, ok := store.(*MemoryStore); ok {
		if users, ok := authStore.(interface {
			UserExists(context.Context, int64) error
		}); ok {
			memory.userExists = users.UserExists
		}
	}
	return &Service{store: store, auditLogger: audit.NopLogger{}}
}

func (s *Service) transaction(ctx context.Context, fn func(*Service) error) error {
	var pending *audit.Recorder
	err := s.store.InTransaction(ctx, func(store Store) error {
		pending = audit.NewRecorder()
		tx := *s
		tx.store, tx.inTransaction, tx.auditLogger = store, true, pending
		return fn(&tx)
	})
	if err != nil {
		return err
	}
	for _, event := range pending.Events() {
		s.auditLogger.Log(ctx, event.Event, event.Attrs)
	}
	return nil
}

// SetAuditLogger attaches a sink for household membership and configuration
// events. If logger is nil the call is a no-op (the service keeps its default
// NopLogger).
func (s *Service) SetAuditLogger(logger audit.Logger) {
	if logger != nil {
		s.auditLogger = logger
	}
}

// WithHistoricalMembers lets household responses identify the authors of
// retained logs after their memberships have ended.
func (s *Service) WithHistoricalMembers(logStore chorelog.Store, userStore interface {
	GetUserByID(context.Context, int64) (auth.User, error)
}) {
	s.logStore = logStore
	s.userStore = userStore
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
	if !s.inTransaction {
		var result Household
		err := s.transaction(ctx, func(tx *Service) error {
			var err error
			result, err = tx.CreateHousehold(ctx, name, initials, ownerID)
			return err
		})
		if err != nil {
			return Household{}, err
		}
		return result, nil
	}
	if err := validateNameInitials(name, initials); err != nil {
		return Household{}, err
	}
	if initials == "" {
		initials = GenerateInitials(name)
	}
	// Multi-household: allow creating even if already in a household
	hh, err := s.store.CreateHousehold(ctx, name, initials, ownerID)
	if err != nil {
		return Household{}, err
	}
	s.logAudit(ctx, "household.created", map[string]string{
		"user_id":      formatID(ownerID),
		"household_id": formatID(hh.ID),
	})
	return hh, nil
}

func (s *Service) GetHousehold(ctx context.Context, userID int64) (Household, []Member, error) {
	householdID, role, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return Household{}, nil, err
	}
	hh, cerr := s.store.GetHousehold(ctx, householdID)
	if cerr != nil {
		return Household{}, nil, cerr
	}
	members, err := s.store.GetMembers(ctx, hh.ID)
	if err != nil {
		return hh, nil, err
	}
	// Read capabilities before the final authorization check. A revocation
	// after this check rotates these already-read values; one before it denies.
	currentID, currentRole, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return Household{}, nil, err
	}
	if currentID != householdID {
		return Household{}, nil, ErrNotAuthorized
	}
	if role != RoleOwner || currentRole != RoleOwner {
		hh.InviteCode = ""
	}
	return hh, members, nil
}

func (s *Service) GetHistoricalMembers(ctx context.Context, userID int64) ([]HistoricalMember, error) {
	if s.logStore == nil || s.userStore == nil {
		return []HistoricalMember{}, nil
	}
	hhID, _, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return nil, ErrNotFound
	}
	members, err := s.store.GetMembers(ctx, hhID)
	if err != nil {
		return nil, err
	}
	active := make(map[int64]struct{}, len(members))
	for _, member := range members {
		active[member.UserID] = struct{}{}
	}
	userIDs, err := s.logStore.ListLogUserIDs(ctx, hhID)
	if err != nil {
		return nil, err
	}
	historical := make([]HistoricalMember, 0, len(userIDs))
	for _, id := range userIDs {
		if _, ok := active[id]; ok {
			continue
		}
		user, err := s.userStore.GetUserByID(ctx, id)
		if err != nil {
			continue // The account was deleted; its profile must remain private.
		}
		historical = append(historical, HistoricalMember{
			UserID:      user.ID,
			DisplayName: user.DisplayName,
			AvatarColor: user.AvatarColor,
		})
	}
	return historical, nil
}

// GetAdminHouseholdID returns the authenticated user's active household when
// they have permission to export household data. The membership lookup is
// intentional: the session's cached role is not an authorization boundary.
func (s *Service) GetAdminHouseholdID(ctx context.Context, userID int64) (int64, error) {
	hhID, role, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return 0, err
	}
	if role != RoleOwner && role != RoleAdmin {
		return 0, ErrNotAuthorized
	}
	return hhID, nil
}

func (s *Service) UpdateHousehold(ctx context.Context, userID int64, name, initials string) error {
	if !s.inTransaction {
		return s.transaction(ctx, func(tx *Service) error { return tx.UpdateHousehold(ctx, userID, name, initials) })
	}
	if err := validateNameInitials(name, initials); err != nil {
		return err
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
	})
	return nil
}

func (s *Service) CreateInvite(ctx context.Context, userID int64) (Invite, error) {
	if !s.inTransaction {
		var result Invite
		err := s.transaction(ctx, func(tx *Service) error {
			var err error
			result, err = tx.CreateInvite(ctx, userID)
			return err
		})
		if err != nil {
			return Invite{}, err
		}
		return result, nil
	}
	hhID, role, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return Invite{}, err
	}
	if role != RoleOwner {
		return Invite{}, ErrNotAuthorized
	}
	code := GenerateInviteCode()
	invite, err := s.store.CreateInvite(ctx, hhID, userID, code, 1)
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
	invites, err := s.store.GetInvites(ctx, hhID)
	if err != nil {
		return nil, err
	}
	currentID, currentRole, err := s.store.GetMembership(ctx, userID)
	if err != nil {
		return nil, err
	}
	if currentID != hhID || currentRole != RoleOwner {
		return nil, ErrNotAuthorized
	}
	return invites, nil
}

func (s *Service) DeleteInvite(ctx context.Context, userID, inviteID int64) error {
	if !s.inTransaction {
		return s.transaction(ctx, func(tx *Service) error { return tx.DeleteInvite(ctx, userID, inviteID) })
	}
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
	if !s.inTransaction {
		var result Household
		err := s.transaction(ctx, func(tx *Service) error {
			var err error
			result, err = tx.JoinHousehold(ctx, userID, inviteCode)
			return err
		})
		if err != nil {
			return Household{}, err
		}
		return result, nil
	}
	invite, err := s.store.GetInviteByCode(ctx, inviteCode)
	if err != nil && err != ErrInviteNotFound && err != ErrInviteExpired {
		return Household{}, err
	}

	// If the code wasn't found (or the one-time invite is exhausted/expired —
	// indistinguishable from unknown so the message never leaks code state),
	// try the permanent household invite code.
	if err == ErrInviteNotFound || err == ErrInviteExpired {
		hh, hhErr := s.store.GetHouseholdByInviteCode(ctx, inviteCode)
		if hhErr != nil {
			return Household{}, ErrInviteNotFound
		}
		// Check if already a member of this specific household
		_, memberErr := s.store.GetMembershipForHousehold(ctx, userID, hh.ID)
		if memberErr == nil {
			return Household{}, ErrAlreadyMember
		}
		if !errors.Is(memberErr, ErrNotMember) {
			return Household{}, memberErr
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
		s.logAudit(ctx, "household.member_joined", map[string]string{
			"user_id":       formatID(userID),
			"household_id":  formatID(hh.ID),
			"invite_method": "permanent_code",
		})
		hh.InviteCode = ""
		return hh, nil
	}

	// Check if already a member of this specific household
	_, memberErr := s.store.GetMembershipForHousehold(ctx, userID, invite.HouseholdID)
	if memberErr == nil {
		return Household{}, ErrAlreadyMember
	}
	if !errors.Is(memberErr, ErrNotMember) {
		return Household{}, memberErr
	}

	members, err := s.store.GetMembers(ctx, invite.HouseholdID)
	if err != nil {
		return Household{}, err
	}
	if len(members) >= MaxMembersPerHousehold {
		return Household{}, fmt.Errorf("household is full")
	}

	// Consumption and membership insertion share this transaction. A failed
	// insertion rolls back the use so a valid invitation remains recoverable.
	if err := s.store.UseInvite(ctx, inviteCode); err != nil {
		return Household{}, ErrInviteNotFound
	}
	if err := s.store.AddMember(ctx, invite.HouseholdID, userID, RoleMember); err != nil {
		return Household{}, err
	}

	hh, err := s.store.GetHousehold(ctx, invite.HouseholdID)
	if err != nil {
		return Household{}, err
	}
	s.logAudit(ctx, "household.member_joined", map[string]string{
		"user_id":       formatID(userID),
		"household_id":  formatID(hh.ID),
		"invite_method": "invite_code",
	})
	hh.InviteCode = ""
	return hh, nil
}

// ListUserHouseholds returns all households the user belongs to.
func (s *Service) ListUserHouseholds(ctx context.Context, userID int64) ([]HouseholdWithRole, error) {
	return s.store.ListUserHouseholds(ctx, userID)
}

// SwitchHousehold switches the user's active household.
func (s *Service) SwitchHousehold(ctx context.Context, userID, householdID int64) error {
	if !s.inTransaction {
		return s.transaction(ctx, func(tx *Service) error { return tx.SwitchHousehold(ctx, userID, householdID) })
	}
	// The membership check and pointer update share the lifecycle lock.
	if _, err := s.store.GetMembershipForHousehold(ctx, userID, householdID); err != nil {
		return err
	}
	return s.store.SetActiveHousehold(ctx, userID, householdID)
}

func (s *Service) UpdateMemberRole(ctx context.Context, actorUserID, targetUserID int64, newRole string) error {
	if !s.inTransaction {
		return s.transaction(ctx, func(tx *Service) error { return tx.UpdateMemberRole(ctx, actorUserID, targetUserID, newRole) })
	}
	actorHHID, actorRole, err := s.store.GetMembership(ctx, actorUserID)
	if err != nil {
		return err
	}
	if actorRole != RoleOwner {
		return ErrNotAuthorized
	}
	if newRole != RoleAdmin && newRole != RoleMember {
		return fmt.Errorf("%w: invalid role", ErrInvalidInput)
	}
	hhID := actorHHID
	targetRole, err := s.store.GetMembershipForHousehold(ctx, targetUserID, hhID)
	if errors.Is(err, ErrNotMember) {
		return ErrNotAuthorized
	}
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
			return ErrLastOwner
		}
	}
	if err := s.store.UpdateMemberRole(ctx, hhID, targetUserID, newRole); err != nil {
		return err
	}
	if newRole != targetRole {
		if err := s.store.RevokeInvites(ctx, hhID); err != nil {
			return err
		}
	}
	s.logAudit(ctx, "household.member_role_changed", map[string]string{
		"user_id":        formatID(actorUserID),
		"target_user_id": formatID(targetUserID),
		"household_id":   formatID(hhID),
		"new_role":       newRole,
	})
	return nil
}

func (s *Service) RemoveMember(ctx context.Context, actorUserID, targetUserID int64) error {
	if !s.inTransaction {
		return s.transaction(ctx, func(tx *Service) error { return tx.RemoveMember(ctx, actorUserID, targetUserID) })
	}
	actorHHID, actorRole, err := s.store.GetMembership(ctx, actorUserID)
	if err != nil {
		return err
	}
	if actorRole != RoleOwner {
		return ErrNotAuthorized
	}
	if actorUserID == targetUserID {
		return ErrNotAuthorized
	}
	hhID := actorHHID
	if _, err := s.store.GetMembershipForHousehold(ctx, targetUserID, hhID); errors.Is(err, ErrNotMember) {
		return ErrNotAuthorized
	} else if err != nil {
		return err
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
	if !s.inTransaction {
		return s.transaction(ctx, func(tx *Service) error { return tx.LeaveHousehold(ctx, userID) })
	}
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
	if !s.inTransaction {
		return s.transaction(ctx, func(tx *Service) error { return tx.TransferOwnership(ctx, currentOwnerID, newOwnerID) })
	}
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
	if err := s.store.RevokeInvites(ctx, hhID); err != nil {
		return err
	}
	s.logAudit(ctx, "household.ownership_transferred", map[string]string{
		"user_id":        formatID(currentOwnerID),
		"target_user_id": formatID(newOwnerID),
		"household_id":   formatID(hhID),
	})
	return nil
}
