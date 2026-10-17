// Package apikey issues personal API keys. A key authenticates as its owner,
// so every household permission check the web UI relies on still applies;
// the HTTP layer separately limits keys to an allowlist of routes.
package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/HammerMeetNail/nabu/internal/auth"
)

var (
	ErrInvalidKey   = errors.New("invalid or revoked API key")
	ErrInvalidInput = errors.New("name must be 1-60 characters")
	ErrTooManyKeys  = errors.New("revoke a key before creating another")
	ErrNotFound     = errors.New("API key not found")
)

const (
	// TokenPrefix distinguishes personal keys from operator keys (nabu_op_).
	TokenPrefix = "nabu_pat_"
	// MaxActiveKeys caps unrevoked keys per user.
	MaxActiveKeys = 10
	idLen         = 16 // base64url of 12 random bytes
	secretLen     = 43 // base64url of 32 random bytes
)

// Key is the public view of a stored key. The token is never returned again.
type Key struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastUsedAt  *time.Time `json:"lastUsedAt"`
	RevokedAt   *time.Time `json:"revokedAt"`
	Invalidated bool       `json:"invalidated"` // password changed since creation
}

// UserLoader re-reads the owner on every request so a password change or
// account deletion takes effect immediately.
type UserLoader interface {
	AuthenticateUserVersion(ctx context.Context, userID, authVersion int64) (auth.User, error)
}

type Service struct {
	store Store
	users UserLoader
	now   func() time.Time
}

func NewService(store Store, users UserLoader) *Service {
	return &Service{store: store, users: users, now: func() time.Time { return time.Now().UTC() }}
}

func randomPart(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func validName(name string) bool {
	if name == "" || utf8.RuneCountInString(name) > 60 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func (s *Service) toKey(rec Record, currentVersion int64) Key {
	return Key{
		ID: rec.ID, Name: rec.Name, CreatedAt: rec.CreatedAt,
		LastUsedAt: rec.LastUsedAt, RevokedAt: rec.RevokedAt,
		Invalidated: rec.AuthVersion != currentVersion,
	}
}

// Create returns the clear token exactly once.
func (s *Service) Create(ctx context.Context, user auth.User, name string) (Key, string, error) {
	name = strings.TrimSpace(name)
	if !validName(name) {
		return Key{}, "", ErrInvalidInput
	}
	id, err := randomPart(12)
	if err != nil {
		return Key{}, "", err
	}
	secret, err := randomPart(32)
	if err != nil {
		return Key{}, "", err
	}
	token := TokenPrefix + id + "." + secret
	digest := sha256.Sum256([]byte(token))
	rec := Record{ID: id, UserID: user.ID, Name: name, TokenHash: digest[:], AuthVersion: user.AuthVersion, CreatedAt: s.now()}
	if err := s.store.Create(ctx, rec, MaxActiveKeys); err != nil {
		return Key{}, "", err
	}
	return s.toKey(rec, user.AuthVersion), token, nil
}

func (s *Service) List(ctx context.Context, user auth.User) ([]Key, error) {
	recs, err := s.store.List(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	keys := make([]Key, 0, len(recs))
	for _, rec := range recs {
		keys = append(keys, s.toKey(rec, user.AuthVersion))
	}
	return keys, nil
}

func (s *Service) Revoke(ctx context.Context, user auth.User, id string) error {
	if len(id) != idLen {
		return ErrNotFound
	}
	return s.store.Revoke(ctx, user.ID, id, s.now())
}

// Authenticate resolves a bearer token to its owner. The public ID narrows the
// lookup; the full token digest is compared in constant time.
func (s *Service) Authenticate(ctx context.Context, token string) (auth.User, string, error) {
	if !strings.HasPrefix(token, TokenPrefix) || len(token) > 128 {
		return auth.User{}, "", ErrInvalidKey
	}
	id, secret, ok := strings.Cut(token[len(TokenPrefix):], ".")
	if !ok || len(id) != idLen || len(secret) != secretLen {
		return auth.User{}, "", ErrInvalidKey
	}
	rec, err := s.store.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return auth.User{}, "", ErrInvalidKey
	}
	if err != nil {
		return auth.User{}, "", err
	}
	digest := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(rec.TokenHash, digest[:]) != 1 || rec.RevokedAt != nil {
		return auth.User{}, "", ErrInvalidKey
	}
	user, err := s.users.AuthenticateUserVersion(ctx, rec.UserID, rec.AuthVersion)
	if errors.Is(err, auth.ErrSessionNotFound) {
		return auth.User{}, "", ErrInvalidKey
	}
	if err != nil {
		return auth.User{}, "", err
	}
	_ = s.store.Touch(ctx, id, s.now())
	return user, id, nil
}
