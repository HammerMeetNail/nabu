package household

import (
	"context"
	"crypto/rand"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"
)

type Household struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Initials   string    `json:"initials"`
	InviteCode string    `json:"inviteCode"`
	CreatedAt  time.Time `json:"createdAt"`
}

// HouseholdWithRole is a Household the user belongs to, annotated with their role.
type HouseholdWithRole struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Initials string `json:"initials"`
	Role     string `json:"role"`
}

type Member struct {
	UserID        int64  `json:"userId"`
	Email         string `json:"email"`
	DisplayName   string `json:"displayName"`
	AvatarColor   string `json:"avatarColor"`
	EmailVerified bool   `json:"emailVerified"`
	Role          string `json:"role"`
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
	CreateHousehold(ctx context.Context, name, initials string, ownerID int64) (Household, error)
	GetHousehold(ctx context.Context, id int64) (Household, error)
	GetUserHousehold(ctx context.Context, userID int64) (Household, error)
	UpdateHousehold(ctx context.Context, id int64, name, initials string) error
	GetMembers(ctx context.Context, householdID int64) ([]Member, error)
	AddMember(ctx context.Context, householdID, userID int64, role string) error
	RemoveMember(ctx context.Context, householdID, userID int64) error
	UpdateMemberRole(ctx context.Context, householdID, userID int64, role string) error
	// GetMembership returns (activeHouseholdID, role) for the user's currently active household.
	GetMembership(ctx context.Context, userID int64) (int64, string, error)
	// GetMembershipForHousehold returns the role of a user in a specific household.
	GetMembershipForHousehold(ctx context.Context, userID, householdID int64) (string, error)
	// ListUserHouseholds returns all households the user belongs to.
	ListUserHouseholds(ctx context.Context, userID int64) ([]HouseholdWithRole, error)
	// SetActiveHousehold switches the user's active household.
	SetActiveHousehold(ctx context.Context, userID, householdID int64) error
	GetHouseholdByInviteCode(ctx context.Context, code string) (Household, error)
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

// GenerateInitials derives short initials from a household name.
// It takes the first letter of each word, up to 3 characters, uppercased.
// If the name is a single word, it takes up to 3 characters from that word.
func GenerateInitials(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "?"
	}
	words := strings.Fields(name)
	var runes []rune
	if len(words) == 1 {
		// Single word: take up to 3 runes
		for i, r := range words[0] {
			if i >= 3 {
				break
			}
			_ = i
			runes = append(runes, []rune(string(r))...)
			if utf8.RuneCountInString(string(runes)) >= 3 {
				break
			}
		}
	} else {
		// Multiple words: take first letter of each word, up to 3
		for _, w := range words {
			if len(runes) >= 3 {
				break
			}
			if len(w) > 0 {
				r, _ := utf8.DecodeRuneInString(w)
				runes = append(runes, r)
			}
		}
	}
	return strings.ToUpper(string(runes))
}
