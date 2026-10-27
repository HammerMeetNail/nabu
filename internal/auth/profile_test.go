package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The name and color a household sees: trimmed, 1 to 50 characters without
// control characters, and a #RRGGBB color. Both stores persist them.
func TestUpdateProfile(t *testing.T) {
	for kind, open := range map[string]func(*testing.T) Store{
		"memory":   func(*testing.T) Store { return NewMemoryStore() },
		"postgres": postgresClaimStore,
	} {
		t.Run(kind, func(t *testing.T) {
			store := open(t)
			svc := NewService(store)
			ctx := context.Background()
			user, _, err := svc.RegisterWithHash(ctx, fmt.Sprintf("profile-%d@example.invalid", time.Now().UnixNano()), "synthetic")
			if err != nil {
				t.Fatal(err)
			}
			originalColor := user.AvatarColor

			name := "  Dana  "
			updated, err := svc.UpdateProfile(ctx, user, &name, nil)
			if err != nil {
				t.Fatal(err)
			}
			if updated.DisplayName != "Dana" || updated.AvatarColor != originalColor {
				t.Fatalf("after a name change: %+v", updated)
			}
			color := "#2e86ab"
			updated, err = svc.UpdateProfile(ctx, updated, nil, &color)
			if err != nil {
				t.Fatal(err)
			}
			if updated.DisplayName != "Dana" || updated.AvatarColor != "#2E86AB" {
				t.Fatalf("after a color change: %+v", updated)
			}
			stored, err := store.GetUserByID(ctx, user.ID)
			if err != nil || stored.DisplayName != "Dana" || stored.AvatarColor != "#2E86AB" {
				t.Fatalf("stored %+v, %v", stored, err)
			}

			longest := strings.Repeat("é", MaxDisplayNameRunes)
			if _, err := svc.UpdateProfile(ctx, updated, &longest, nil); err != nil {
				t.Errorf("a %d-character name: %v", MaxDisplayNameRunes, err)
			}
			for _, bad := range []string{"", "   ", strings.Repeat("a", MaxDisplayNameRunes+1), "Dana\x00", "Da\nna", "Dana\u200f\x7f"} {
				if _, err := svc.UpdateProfile(ctx, updated, &bad, nil); !errors.Is(err, ErrInvalidDisplayName) {
					t.Errorf("name %q: %v", bad, err)
				}
			}
			for _, bad := range []string{"", "2E86AB", "#2E86A", "#2E86ABC", "#GGGGGG", "red", "#2E86AB\n"} {
				if _, err := svc.UpdateProfile(ctx, updated, nil, &bad); !errors.Is(err, ErrInvalidAvatarColor) {
					t.Errorf("color %q: %v", bad, err)
				}
			}
			if _, err := svc.UpdateProfile(ctx, User{ID: 999_999_999}, &name, nil); !errors.Is(err, ErrUserNotFound) {
				t.Errorf("missing user: %v", err)
			}
		})
	}
}
