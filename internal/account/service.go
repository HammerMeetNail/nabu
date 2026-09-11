// Package account coordinates cross-domain account operations. Its only job
// today is account deletion (App Store guideline 5.1.1(v)): validating the
// user's household roles, deleting sole-member households, and removing the
// user with all personal data.
package account

import (
	"context"
	"errors"
	"fmt"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/household"
)

// ErrMustTransferOwnership is returned when the user is the only owner of a
// household that still has other members. The client should guide the user
// through transferring ownership (or removing the other members) first.
type ErrMustTransferOwnership struct {
	HouseholdName string
}

func (e ErrMustTransferOwnership) Error() string {
	return fmt.Sprintf("you are the only owner of %q; transfer ownership or remove its other members first", e.HouseholdName)
}

type Service struct {
	users             auth.Store
	households        household.Store
	inTransaction     bool
	memoryCleanup     func(int64, []int64)
	cleanupUserID     int64
	cleanupHouseholds []int64
}

func NewService(users auth.Store, households household.Store) *Service {
	return &Service{users: users, households: households}
}

// transaction joins the household and auth stores. PostgreSQL shares one SQL
// transaction; memory mode prepares both snapshots before committing either.
func (s *Service) transaction(ctx context.Context, fn func(*Service) error) error {
	if _, ok := s.households.(interface {
		AuthInTransaction(auth.Store) (auth.Store, error)
	}); ok {
		return s.households.InTransaction(ctx, func(households household.Store) error {
			shared, ok := households.(interface {
				AuthInTransaction(auth.Store) (auth.Store, error)
			})
			if !ok {
				return errors.New("account transaction unavailable")
			}
			users, err := shared.AuthInTransaction(s.users)
			if err != nil {
				return err
			}
			tx := *s
			tx.users, tx.households, tx.inTransaction = users, households, true
			return fn(&tx)
		})
	}
	hp, ok := s.households.(interface {
		PrepareTransaction() (household.Store, func(), func())
	})
	if !ok {
		return errors.New("account transaction unavailable")
	}
	up, ok := s.users.(interface {
		PrepareTransaction() (auth.Store, func(), func())
	})
	if !ok {
		return errors.New("account transaction unavailable")
	}
	households, commitHouseholds, unlockHouseholds := hp.PrepareTransaction()
	defer unlockHouseholds()
	users, commitUsers, unlockUsers := up.PrepareTransaction()
	defer unlockUsers()
	tx := *s
	tx.users, tx.households, tx.inTransaction = users, households, true
	if err := fn(&tx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if tx.memoryCleanup != nil && tx.cleanupUserID != 0 {
		tx.memoryCleanup(tx.cleanupUserID, tx.cleanupHouseholds)
	}
	commitUsers()
	commitHouseholds()
	return nil
}

// DeleteAccount permanently deletes the user. Semantics per household:
//
//   - sole member            → the household and all its data are deleted
//   - only owner, has others → rejected with ErrMustTransferOwnership
//   - otherwise              → equivalent to leaving the household
//
// Validation runs over every membership before anything is mutated, so a
// rejection never leaves a half-deleted account. Personal data (sessions,
// tokens, preferences, notifications, push subscriptions, the user's chore
// logs) is removed; household-shared content in surviving households stays,
// with chore authorship and schedule assignment cleared and the user's
// invites revoked.
func (s *Service) DeleteAccount(ctx context.Context, userID int64) error {
	if !s.inTransaction {
		return s.transaction(ctx, func(tx *Service) error { return tx.DeleteAccount(ctx, userID) })
	}
	memberships, err := s.households.ListUserHouseholds(ctx, userID)
	if err != nil {
		return err
	}

	var deleteHouseholds []int64
	for _, hh := range memberships {
		members, err := s.households.GetMembers(ctx, hh.ID)
		if err != nil {
			return err
		}
		if len(members) <= 1 {
			deleteHouseholds = append(deleteHouseholds, hh.ID)
			continue
		}
		if hh.Role == household.RoleOwner {
			otherOwner := false
			for _, m := range members {
				if m.UserID != userID && m.Role == household.RoleOwner {
					otherOwner = true
					break
				}
			}
			if !otherOwner {
				return ErrMustTransferOwnership{HouseholdName: hh.Name}
			}
		}
	}

	if locker, ok := s.households.(interface {
		LockAccountData(context.Context, int64) error
	}); ok {
		if err := locker.LockAccountData(ctx, userID); err != nil {
			return err
		}
	}

	// Revoke membership and disclosed invitations, then clear references and
	// delete the user, then delete households with no surviving members. The
	// complete sequence shares the policy transaction and rolls back on error.
	for _, membership := range memberships {
		if err := s.households.RemoveMember(ctx, membership.ID, userID); err != nil {
			return err
		}
	}
	if memory, ok := s.households.(interface{ RemoveUserInvites(int64) }); ok {
		memory.RemoveUserInvites(userID)
	}
	if err := s.users.DeleteUser(ctx, userID); err != nil {
		return err
	}
	s.cleanupUserID, s.cleanupHouseholds = userID, deleteHouseholds
	for _, hhID := range deleteHouseholds {
		if err := s.households.DeleteHousehold(ctx, hhID); err != nil {
			return err
		}
	}
	return nil
}

// SetMemoryCleanup enlists infallible domain cleanup at the memory commit
// point, after every policy/write/context check and before releasing identity.
func (s *Service) SetMemoryCleanup(cleanup func(int64, []int64)) { s.memoryCleanup = cleanup }
