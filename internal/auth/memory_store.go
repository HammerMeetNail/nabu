package auth

import (
	"context"
	"sync"
	"time"
)

type MemoryStore struct {
	mu             sync.RWMutex
	idSeq          int64
	usersByEmail   map[string]User
	usersByID      map[int64]User
	passwordsByID  map[int64]string
	sessionsByHash map[string]Session
	tokensByHash   map[string]AuthToken
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		usersByEmail:   map[string]User{},
		usersByID:      map[int64]User{},
		passwordsByID:  map[int64]string{},
		sessionsByHash: map[string]Session{},
		tokensByHash:   map[string]AuthToken{},
	}
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

	if _, ok := s.usersByID[userID]; !ok {
		return ErrUserNotFound
	}
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

func (s *MemoryStore) CreateSession(_ context.Context, userID int64, tokenHash string, expiresAt time.Time) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := Session{
		ID:        randomToken(32),
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
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
