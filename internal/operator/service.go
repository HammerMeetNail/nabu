// Package operator contains platform-wide, read-only reporting and its
// deliberately separate credential boundary. Household roles grant no access.
package operator

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/HammerMeetNail/nabu/internal/auth"
)

var (
	ErrDenied       = errors.New("operator access denied")
	ErrRecentLogin  = errors.New("sign in again before creating a key")
	ErrInvalidKey   = errors.New("invalid or expired key")
	ErrInvalidInput = errors.New("invalid key settings")
	ErrTooManyKeys  = errors.New("revoke a key before creating another")
)

const (
	ScopeSummary = "summary"
	ScopeFull    = "full"
	keyPrefix    = "nabu_op_"
)

type Service struct {
	db      *sql.DB
	ownerID int64
	now     func() time.Time
}

// Access can only be minted after a current owner-session or key check. The
// reporting service also checks it so handler mistakes cannot expose data.
type Access struct {
	ownerID int64
	scope   string
	keyID   string
}

func (a Access) KeyID() string { return a.keyID }

func (s *Service) AuthorizeSession(user auth.User) (Access, error) {
	if !s.IsOwner(user) {
		return Access{}, ErrDenied
	}
	return Access{ownerID: user.ID, scope: ScopeFull}, nil
}

func (s *Service) requireAccess(access Access, scope string) error {
	if s == nil || s.ownerID < 1 || access.ownerID != s.ownerID ||
		(access.scope != ScopeFull && (access.scope != ScopeSummary || scope == ScopeFull)) {
		return ErrDenied
	}
	return nil
}

func NewService(db *sql.DB, ownerID int64) *Service {
	return &Service{db: db, ownerID: ownerID, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) IsOwner(user auth.User) bool {
	return s != nil && s.db != nil && s.ownerID > 0 && user.ID == s.ownerID && user.EmailVerified
}

// Session auth has already loaded the current user; key auth must re-check the
// owner and credential version on every request so account recovery revokes keys.
func (s *Service) ownerVersion(ctx context.Context, userID int64, version int64) (bool, error) {
	if s == nil || userID != s.ownerID || s.ownerID < 1 {
		return false, nil
	}
	var valid bool
	err := s.db.QueryRowContext(ctx, `SELECT email_verified AND auth_version = $2 FROM users WHERE id = $1`, userID, version).Scan(&valid)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return valid, err
}

func (s *Service) RequireRecentSession(ctx context.Context, user auth.User) error {
	if !s.IsOwner(user) || user.SessionHash == "" {
		return ErrDenied
	}
	var authenticatedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT authenticated_at FROM sessions WHERE user_id = $1 AND token_hash = $2 AND auth_version = $3`,
		user.ID, user.SessionHash, user.AuthVersion).Scan(&authenticatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDenied
	}
	if err != nil {
		return err
	}
	now := s.now()
	if !authenticatedAt.Valid || now.Sub(authenticatedAt.Time) > 10*time.Minute || authenticatedAt.Time.After(now.Add(time.Minute)) {
		return ErrRecentLogin
	}
	return nil
}

type Key struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Scope       string     `json:"scope"`
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	LastUsedAt  *time.Time `json:"lastUsedAt"`
	RevokedAt   *time.Time `json:"revokedAt"`
	Invalidated bool       `json:"invalidated"`
}

func randomPart(bytes int) (string, error) {
	buf := make([]byte, bytes)
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

// CreateKey returns cleartext only once. Locking the owner row serializes the
// active-key cap across concurrent requests, including separate app replicas.
func (s *Service) CreateKey(ctx context.Context, user auth.User, name, scope string, days int) (Key, string, error) {
	name = strings.TrimSpace(name)
	if !validName(name) || (scope != ScopeSummary && scope != ScopeFull) || (days != 7 && days != 30 && days != 90) {
		return Key{}, "", ErrInvalidInput
	}
	if err := s.RequireRecentSession(ctx, user); err != nil {
		return Key{}, "", err
	}
	id, err := randomPart(12)
	if err != nil {
		return Key{}, "", err
	}
	secret, err := randomPart(32)
	if err != nil {
		return Key{}, "", err
	}
	token := keyPrefix + id + "." + secret
	digest := sha256.Sum256([]byte(token))
	now := s.now()
	key := Key{ID: id, Name: name, Scope: scope, CreatedAt: now, ExpiresAt: now.AddDate(0, 0, days)}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Key{}, "", err
	}
	defer func() { _ = tx.Rollback() }()
	var verified bool
	var version int64
	err = tx.QueryRowContext(ctx, `SELECT email_verified, auth_version FROM users WHERE id = $1 FOR UPDATE`, user.ID).Scan(&verified, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return Key{}, "", ErrDenied
	}
	if err != nil {
		return Key{}, "", err
	}
	if !verified || version != user.AuthVersion {
		return Key{}, "", ErrDenied
	}
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operator_api_keys WHERE owner_user_id = $1 AND revoked_at IS NULL AND expires_at > $2 AND auth_version = $3`, user.ID, now, version).Scan(&active); err != nil {
		return Key{}, "", err
	}
	if active >= 5 {
		return Key{}, "", ErrTooManyKeys
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO operator_api_keys (id, owner_user_id, name, scope, token_hash, auth_version, created_at, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, id, user.ID, name, scope, digest[:], version, now, key.ExpiresAt)
	if err != nil {
		return Key{}, "", err
	}
	if err = tx.Commit(); err != nil {
		return Key{}, "", err
	}
	return key, token, nil
}

func (s *Service) ListKeys(ctx context.Context, user auth.User) ([]Key, error) {
	if !s.IsOwner(user) {
		return nil, ErrDenied
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, scope, created_at, expires_at, last_used_at, revoked_at,
		(auth_version <> $2) AS invalidated
		FROM operator_api_keys WHERE owner_user_id = $1
		ORDER BY CASE WHEN revoked_at IS NULL AND expires_at > $3 AND auth_version = $2 THEN 0 ELSE 1 END,
		created_at DESC LIMIT 100`, user.ID, user.AuthVersion, s.now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]Key, 0)
	for rows.Next() {
		var key Key
		var used, revoked sql.NullTime
		if err := rows.Scan(&key.ID, &key.Name, &key.Scope, &key.CreatedAt, &key.ExpiresAt, &used, &revoked, &key.Invalidated); err != nil {
			return nil, err
		}
		if used.Valid {
			key.LastUsedAt = &used.Time
		}
		if revoked.Valid {
			key.RevokedAt = &revoked.Time
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Service) RevokeKey(ctx context.Context, user auth.User, id string) error {
	if !s.IsOwner(user) {
		return ErrDenied
	}
	if len(id) != 16 {
		return ErrInvalidInput
	}
	_, err := s.db.ExecContext(ctx, `UPDATE operator_api_keys SET revoked_at = COALESCE(revoked_at, $3)
		WHERE id = $1 AND owner_user_id = $2`, id, user.ID, s.now())
	return err
}

// AuthenticateKey accepts only the operator bearer format. The public key ID
// narrows the lookup; the 256-bit secret is checked in constant time.
func (s *Service) AuthenticateKey(ctx context.Context, token, requiredScope string) (Access, error) {
	if s == nil || !strings.HasPrefix(token, keyPrefix) || len(token) > 128 {
		return Access{}, ErrInvalidKey
	}
	parts := strings.Split(token[len(keyPrefix):], ".")
	if len(parts) != 2 || len(parts[0]) != 16 || len(parts[1]) != 43 {
		return Access{}, ErrInvalidKey
	}
	var verifiedID string
	var stored []byte
	var scope string
	var ownerID, version int64
	var expires time.Time
	var revoked sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT id, token_hash, scope, owner_user_id, auth_version, expires_at, revoked_at
		FROM operator_api_keys WHERE id = $1`, parts[0]).Scan(&verifiedID, &stored, &scope, &ownerID, &version, &expires, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return Access{}, ErrInvalidKey
	}
	if err != nil {
		return Access{}, err
	}
	digest := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(stored, digest[:]) != 1 || revoked.Valid || !expires.After(s.now()) ||
		(requiredScope == ScopeFull && scope != ScopeFull) {
		return Access{}, ErrInvalidKey
	}
	valid, err := s.ownerVersion(ctx, ownerID, version)
	if err != nil {
		return Access{}, err
	}
	if !valid {
		return Access{}, ErrInvalidKey
	}
	// At most one write per key per hour, even with a busy dashboard client.
	_, _ = s.db.ExecContext(ctx, `UPDATE operator_api_keys SET last_used_at = $2
		WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < $2 - INTERVAL '1 hour')`, parts[0], s.now())
	return Access{ownerID: ownerID, scope: scope, keyID: verifiedID}, nil
}
