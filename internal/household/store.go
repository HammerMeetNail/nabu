package household

import (
	"context"
	"crypto/rand"
	"math/big"
	"time"
)

type Household struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	InviteCode string    `json:"inviteCode"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Member struct {
	UserID      int64  `json:"userId"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	AvatarColor string `json:"avatarColor"`
	Role        string `json:"role"`
}

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

type Invite struct {
	ID          int64      `json:"id"`
	HouseholdID int64      `json:"householdId"`
	Code        string     `json:"code"`
	CreatedBy   int64      `json:"createdBy"`
	MaxUses     int        `json:"maxUses"`
	UsedCount   int        `json:"usedCount"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type Store interface {
	CreateHousehold(ctx context.Context, name string, ownerID int64) (Household, error)
	GetHousehold(ctx context.Context, id int64) (Household, error)
	GetUserHousehold(ctx context.Context, userID int64) (Household, error)
	UpdateHousehold(ctx context.Context, id int64, name string) error
	GetMembers(ctx context.Context, householdID int64) ([]Member, error)
	AddMember(ctx context.Context, householdID, userID int64, role string) error
	RemoveMember(ctx context.Context, householdID, userID int64) error
	UpdateMemberRole(ctx context.Context, householdID, userID int64, role string) error
	GetMembership(ctx context.Context, userID int64) (int64, string, error)
	CreateInvite(ctx context.Context, householdID, createdBy int64, code string, maxUses int) (Invite, error)
	GetInviteByCode(ctx context.Context, code string) (Invite, error)
	GetInvites(ctx context.Context, householdID int64) ([]Invite, error)
	UseInvite(ctx context.Context, code string) error
	DeleteInvite(ctx context.Context, id int64) error
}

func GenerateInviteCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, 6)
	for i := range buf {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		buf[i] = chars[n.Int64()]
	}
	return string(buf)
}
