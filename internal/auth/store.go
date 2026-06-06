package auth

import (
	"context"
	"time"
)

type User struct {
	ID            int64     `json:"id"`
	HouseholdID   *int64    `json:"householdId"`
	Email         string    `json:"email"`
	DisplayName   string    `json:"displayName"`
	AvatarColor   string    `json:"avatarColor"`
	EmailVerified bool      `json:"emailVerified"`
	Role          string    `json:"role"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Session struct {
	ID         string    `json:"id"`
	UserID     int64     `json:"userId"`
	ExpiresAt  time.Time `json:"expiresAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	CreatedAt  time.Time `json:"createdAt"`
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
	CreateUser(ctx context.Context, email, passwordHash string) (User, error)
	GetUserByEmail(ctx context.Context, email string) (User, string, error)
	GetUserByID(ctx context.Context, id int64) (User, error)
	GetUserByIDWithHash(ctx context.Context, id int64) (User, string, error)
	FindUserByEmail(ctx context.Context, email string) (User, error)
	VerifyEmail(ctx context.Context, userID int64) (User, error)
	UpdatePassword(ctx context.Context, userID int64, passwordHash string) error
	SetUserHousehold(ctx context.Context, userID, householdID int64, role string) error
	CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (Session, error)
	GetSession(ctx context.Context, tokenHash string) (Session, error)
	TouchSession(ctx context.Context, tokenHash string, lastSeenAt time.Time) error
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteUserSessions(ctx context.Context, userID int64) error
	CreateAuthToken(ctx context.Context, userID *int64, email, tokenHash, kind string, expiresAt time.Time) (AuthToken, error)
	ConsumeAuthToken(ctx context.Context, tokenHash, kind string) (AuthToken, error)
}
