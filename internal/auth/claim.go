package auth

import (
	"context"
	"errors"
	"fmt"
)

// Every proof flow locks the user before consuming tokens or replacing
// credentials. A failed session insert therefore leaves the proof usable.
func (s *Service) transactionalLogin(ctx context.Context, action func(*Service) (User, error)) (User, Session, error) {
	var user User
	var session Session
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		err = s.store.InTransaction(ctx, func(store Store) error {
			tx := *s
			tx.store = store
			var err error
			user, err = action(&tx)
			if err != nil {
				return err
			}
			session, err = tx.newSession(ctx, user)
			return err
		})
		// Another proof/registration can create the email between lookup and
		// insert. Start a fresh transaction, then lock and claim that account.
		if !errors.Is(err, ErrDuplicateEmail) {
			break
		}
	}
	if err != nil {
		return User{}, Session{}, err
	}
	// The durable write already committed. A profile lookup cannot turn it
	// into a failed signup; on a memory-store lookup failure return no household.
	if profile, profileErr := s.profile(ctx, user); profileErr == nil {
		user = profile
	} else {
		user.HouseholdID, user.Role = nil, ""
	}
	return user, session, nil
}

func (s *Service) loginWithVerifiedEmail(ctx context.Context, email, method string) (User, Session, error) {
	email, err := normalizeAndValidateEmail(email)
	if err != nil {
		return User{}, Session{}, err
	}
	user, session, err := s.transactionalLogin(ctx, func(tx *Service) (User, error) {
		user, err := tx.store.FindUserByEmail(ctx, email)
		if errors.Is(err, ErrUserNotFound) {
			user, err = tx.store.CreateUser(ctx, email, "")
		}
		if err != nil {
			return User{}, err
		}
		return tx.claimUnverifiedUser(ctx, user)
	})
	if err == nil {
		s.logAudit(ctx, "auth.login_succeeded", map[string]string{"method": method, "user_id": fmt.Sprintf("%d", user.ID)})
	}
	return user, session, err
}

func (s *Service) consumeEmailProof(ctx context.Context, token, kind string, allowCreate bool) (User, error) {
	hash := hashToken(token)
	proof, err := s.store.GetAuthToken(ctx, hash, kind)
	if err != nil {
		return User{}, err
	}
	user, err := s.store.FindUserByEmail(ctx, proof.Email)
	if errors.Is(err, ErrUserNotFound) && allowCreate && proof.UserID == nil {
		user, err = s.store.CreateUser(ctx, proof.Email, "")
	}
	if err != nil {
		return User{}, err
	}
	if proof.UserID != nil && *proof.UserID != user.ID {
		return User{}, ErrInvalidToken
	}
	if _, err := s.store.ConsumeAuthToken(ctx, hash, kind); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Service) claimUnverifiedUser(ctx context.Context, user User) (User, error) {
	if user.EmailVerified {
		return user, nil
	}
	// The password and sessions predating mailbox proof were established by
	// an unknown registrant. The owner can set a password from the new session.
	return s.replaceCredentials(ctx, user, "", true)
}

func (s *Service) replaceCredentials(ctx context.Context, user User, passwordHash string, verify bool) (User, error) {
	if err := s.store.UpdatePassword(ctx, user.ID, passwordHash); err != nil {
		return User{}, err
	}
	if err := s.store.DeleteUserSessions(ctx, user.ID); err != nil {
		return User{}, err
	}
	if err := s.store.DeleteUserAuthTokens(ctx, user.ID, user.Email); err != nil {
		return User{}, err
	}
	if verify {
		return s.store.VerifyEmail(ctx, user.ID)
	}
	updated, err := s.store.GetUserByID(ctx, user.ID)
	if err != nil {
		return User{}, err
	}
	if !updated.EmailVerified {
		if err := s.queueVerificationEmail(ctx, updated); err != nil {
			return User{}, err
		}
	}
	return updated, nil
}
