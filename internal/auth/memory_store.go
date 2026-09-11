package auth

import (
	"context"
	"maps"
	"sync"
	"time"
)

type MemoryStore struct {
	mu             sync.RWMutex
	inTransaction  bool
	idSeq          int64
	usersByEmail   map[string]User
	usersByID      map[int64]User
	passwordsByID  map[int64]string
	sessionsByHash map[string]Session
	tokensByHash   map[string]AuthToken
	mailByID       map[int64]AuthMail
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		usersByEmail:   map[string]User{},
		usersByID:      map[int64]User{},
		passwordsByID:  map[int64]string{},
		sessionsByHash: map[string]Session{},
		tokensByHash:   map[string]AuthToken{},
		mailByID:       map[int64]AuthMail{},
	}
}

func (s *MemoryStore) WithSession(ctx context.Context, userID int64, hash string, fn func() error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	session, ok := s.sessionsByHash[hash]
	user, exists := s.usersByID[userID]
	now := time.Now()
	if !ok || !exists || session.UserID != userID || session.AuthVersion != user.AuthVersion ||
		!now.Before(session.ExpiresAt) || now.Sub(session.LastSeenAt) >= sessionIdleTimeout {
		return ErrSessionNotFound
	}
	return fn()
}

// Copy-on-write gives memory mode the same rollback boundary as PostgreSQL.
func (s *MemoryStore) InTransaction(ctx context.Context, fn func(Store) error) error {
	if s.inTransaction {
		return fn(s)
	}
	tx, commit, unlock := s.PrepareTransaction()
	defer unlock()
	if err := fn(tx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	commit()
	return nil
}

// PrepareTransaction lets account deletion commit the auth and household
// snapshots together. The caller must unlock, even when discarding changes.
func (s *MemoryStore) PrepareTransaction() (Store, func(), func()) {
	s.mu.Lock()
	tx := &MemoryStore{inTransaction: true, idSeq: s.idSeq,
		usersByEmail: maps.Clone(s.usersByEmail), usersByID: maps.Clone(s.usersByID),
		passwordsByID: maps.Clone(s.passwordsByID), sessionsByHash: maps.Clone(s.sessionsByHash),
		tokensByHash: maps.Clone(s.tokensByHash), mailByID: maps.Clone(s.mailByID)}
	commit := func() {
		s.mailByID = tx.mailByID
		s.idSeq, s.usersByEmail, s.usersByID = tx.idSeq, tx.usersByEmail, tx.usersByID
		s.passwordsByID, s.sessionsByHash, s.tokensByHash = tx.passwordsByID, tx.sessionsByHash, tx.tokensByHash
	}
	return tx, commit, s.mu.Unlock
}

func (s *MemoryStore) nextID() int64 {
	s.idSeq++
	return s.idSeq
}

func (s *MemoryStore) CreateUser(_ context.Context, email, passwordHash string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.usersByEmail[email]; exists {
		return User{}, ErrDuplicateEmail
	}

	id := s.nextID()
	now := time.Now().UTC()
	user := User{
		ID:          id,
		HasPassword: passwordHash != "",
		Email:       email,
		DisplayName: emailToDisplay(email),
		AvatarColor: "#19323C",
		CreatedAt:   now,
	}
	s.usersByEmail[email] = user
	s.usersByID[id] = user
	s.passwordsByID[id] = passwordHash
	return user, nil
}

func (s *MemoryStore) GetUserByEmail(_ context.Context, email string) (User, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.usersByEmail[email]
	if !ok {
		return User{}, "", ErrUserNotFound
	}
	pass := s.passwordsByID[user.ID]
	return user, pass, nil
}

func (s *MemoryStore) GetUserByID(_ context.Context, id int64) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.usersByID[id]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return user, nil
}

func (s *MemoryStore) GetUserByIDWithHash(_ context.Context, id int64) (User, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.usersByID[id]
	if !ok {
		return User{}, "", ErrUserNotFound
	}
	pass := s.passwordsByID[id]
	return user, pass, nil
}

func (s *MemoryStore) FindUserByEmail(_ context.Context, email string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.usersByEmail[email]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return user, nil
}

func (s *MemoryStore) VerifyEmail(_ context.Context, userID int64) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.usersByID[userID]
	if !ok {
		return User{}, ErrUserNotFound
	}
	user.EmailVerified = true
	s.usersByID[userID] = user
	s.usersByEmail[user.Email] = user
	return user, nil
}

func (s *MemoryStore) UpdatePassword(_ context.Context, userID int64, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.usersByID[userID]
	if !ok {
		return ErrUserNotFound
	}
	user.AuthVersion++
	user.HasPassword = passwordHash != ""
	s.usersByID[userID], s.usersByEmail[user.Email] = user, user
	s.passwordsByID[userID] = passwordHash
	return nil
}

func (s *MemoryStore) SetUserHousehold(_ context.Context, userID, householdID int64, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.usersByID[userID]
	if !ok {
		return ErrUserNotFound
	}
	user.HouseholdID = &householdID
	user.Role = role
	s.usersByID[userID] = user
	s.usersByEmail[user.Email] = user
	return nil
}

func (s *MemoryStore) CreateSession(_ context.Context, userID, authVersion int64, tokenHash string, expiresAt time.Time) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.usersByID[userID]
	if !ok || user.AuthVersion != authVersion {
		return Session{}, ErrInvalidCredentials
	}
	now := time.Now().UTC()
	session := Session{
		ID:          randomToken(32),
		UserID:      userID,
		AuthVersion: authVersion,
		ExpiresAt:   expiresAt,
		LastSeenAt:  now,
		CreatedAt:   now,
	}
	s.sessionsByHash[tokenHash] = session
	return session, nil
}

func (s *MemoryStore) GetSession(_ context.Context, tokenHash string) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessionsByHash[tokenHash]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return session, nil
}

func (s *MemoryStore) TouchSession(_ context.Context, tokenHash string, lastSeenAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessionsByHash[tokenHash]
	if !ok {
		return ErrSessionNotFound
	}
	session.LastSeenAt = lastSeenAt
	s.sessionsByHash[tokenHash] = session
	return nil
}

func (s *MemoryStore) DeleteSession(_ context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessionsByHash, tokenHash)
	return nil
}

func (s *MemoryStore) DeleteUserSessions(_ context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for hash, session := range s.sessionsByHash {
		if session.UserID == userID {
			delete(s.sessionsByHash, hash)
		}
	}
	return nil
}

func (s *MemoryStore) DeleteUser(_ context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.usersByID[userID]
	if !ok {
		return nil
	}
	delete(s.usersByEmail, user.Email)
	delete(s.usersByID, userID)
	delete(s.passwordsByID, userID)
	for hash, session := range s.sessionsByHash {
		if session.UserID == userID {
			delete(s.sessionsByHash, hash)
		}
	}
	for hash, token := range s.tokensByHash {
		if token.Email == user.Email || (token.UserID != nil && *token.UserID == userID) {
			delete(s.tokensByHash, hash)
		}
	}
	for id, job := range s.mailByID {
		if job.UserID == userID {
			delete(s.mailByID, id)
		}
	}

	return nil
}

func (s *MemoryStore) CreateAuthToken(_ context.Context, userID *int64, email, tokenHash, kind string, expiresAt time.Time) (AuthToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	token := AuthToken{
		ID:        s.nextID(),
		UserID:    userID,
		Email:     email,
		TokenHash: tokenHash,
		Kind:      kind,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}
	s.tokensByHash[tokenHash] = token
	return token, nil
}

func (s *MemoryStore) GetAuthToken(_ context.Context, tokenHash, kind string) (AuthToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	token, ok := s.tokensByHash[tokenHash]
	if !ok || token.Kind != kind || token.ConsumedAt != nil || !time.Now().UTC().Before(token.ExpiresAt) {
		return AuthToken{}, ErrInvalidToken
	}
	return token, nil
}

func (s *MemoryStore) DeleteUserAuthTokens(_ context.Context, userID int64, email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for hash, token := range s.tokensByHash {
		if token.Email == email || (token.UserID != nil && *token.UserID == userID) {
			delete(s.tokensByHash, hash)
		}
	}
	for id, job := range s.mailByID {
		if job.UserID == userID {
			delete(s.mailByID, id)
		}
	}

	return nil
}

func (s *MemoryStore) ConsumeAuthToken(_ context.Context, tokenHash, kind string) (AuthToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, ok := s.tokensByHash[tokenHash]
	if !ok || token.Kind != kind || token.ConsumedAt != nil || time.Now().UTC().After(token.ExpiresAt) {
		return AuthToken{}, ErrInvalidToken
	}
	now := time.Now().UTC()
	token.ConsumedAt = &now
	s.tokensByHash[tokenHash] = token
	return token, nil
}

func emailToDisplay(email string) string {
	for i, c := range email {
		if c == '@' {
			return email[:i]
		}
	}
	return email
}
