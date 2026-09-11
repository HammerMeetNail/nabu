package auth

import (
	"context"
	"github.com/HammerMeetNail/nabu/internal/mail"
	"time"
)

type User struct {
	ID            int64     `json:"id"`
	HouseholdID   *int64    `json:"householdId"`
	Email         string    `json:"email"`
	DisplayName   string    `json:"displayName"`
	AvatarColor   string    `json:"avatarColor"`
	EmailVerified bool      `json:"emailVerified"`
	HasPassword   bool      `json:"hasPassword"`
	AuthVersion   int64     `json:"-"`
	SessionHash   string    `json:"-"` // request-only, never serialized or logged
	Role          string    `json:"role"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Session struct {
	ID          string    `json:"id"`
	UserID      int64     `json:"userId"`
	AuthVersion int64     `json:"-"`
	ExpiresAt   time.Time `json:"expiresAt"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
	CreatedAt   time.Time `json:"createdAt"`
}

type AuthToken struct {
	ID         int64      `json:"id"`
	UserID     *int64     `json:"userId"`
	TokenHash  string     `json:"-"`
	Kind       string     `json:"kind"`
	Email      string     `json:"email"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	ConsumedAt *time.Time `json:"consumedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
}

type Store interface {
	// InTransaction commits all callback writes together or rolls them back.
	// User reads inside the callback serialize credential changes for that user.
	InTransaction(ctx context.Context, fn func(Store) error) error
	CreateUser(ctx context.Context, email, passwordHash string) (User, error)
	GetUserByEmail(ctx context.Context, email string) (User, string, error)
	GetUserByID(ctx context.Context, id int64) (User, error)
	GetUserByIDWithHash(ctx context.Context, id int64) (User, string, error)
	FindUserByEmail(ctx context.Context, email string) (User, error)
	VerifyEmail(ctx context.Context, userID int64) (User, error)
	UpdatePassword(ctx context.Context, userID int64, passwordHash string) error
	SetUserHousehold(ctx context.Context, userID, householdID int64, role string) error
	CreateSession(ctx context.Context, userID, authVersion int64, tokenHash string, expiresAt time.Time) (Session, error)
	GetSession(ctx context.Context, tokenHash string) (Session, error)
	TouchSession(ctx context.Context, tokenHash string, lastSeenAt time.Time) error
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteUserSessions(ctx context.Context, userID int64) error
	CreateAuthToken(ctx context.Context, userID *int64, email, tokenHash, kind string, expiresAt time.Time) (AuthToken, error)
	GetAuthToken(ctx context.Context, tokenHash, kind string) (AuthToken, error)
	ConsumeAuthToken(ctx context.Context, tokenHash, kind string) (AuthToken, error)
	DeleteUserAuthTokens(ctx context.Context, userID int64, email string) error
	QueueAuthMail(ctx context.Context, userID int64, message mail.Message, expiresAt time.Time) error
	ClaimAuthMail(ctx context.Context, now, leaseUntil time.Time, leaseID string) (AuthMail, error)
	FinishAuthMail(ctx context.Context, job AuthMail, delivered bool, nextAttempt time.Time) error
	// DeleteUser permanently deletes the user and their personal data
	// (sessions, tokens, preferences, notifications, push subscriptions, logs
	// via FK cascade). Household-shared content survives with authorship
	// cleared. Deleting an already-deleted user is not an error.
	DeleteUser(ctx context.Context, userID int64) error
}

// AuthMail contains a short-lived email capability. Never log or expose it.
type AuthMail struct {
	ID          int64
	UserID      int64
	Message     mail.Message
	ExpiresAt   time.Time
	NextAttempt time.Time
	LeaseUntil  time.Time
	LeaseID     string
	Attempts    int
}
