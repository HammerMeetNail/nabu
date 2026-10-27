package household

import (
	"context"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
)

// The Postgres store joins members' names, emails and colors from users; the
// in-memory store (local runs and E2E) fills them in from the auth service,
// so members never show as "Unknown" there.
func TestMemoryMembersCarryProfiles(t *testing.T) {
	ctx := context.Background()
	authService := auth.NewService(auth.NewMemoryStore())
	authService.SetAuditLogger(nil)
	svc := NewService(NewMemoryStore(), authService)
	user, _, err := authService.RegisterWithHash(ctx, "dana@example.com", "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateHousehold(ctx, "Home", "", user.ID); err != nil {
		t.Fatal(err)
	}
	name, color := "Dana", "#7B5EA7"
	if _, err := authService.UpdateProfile(ctx, user, &name, &color); err != nil {
		t.Fatal(err)
	}
	_, members, err := svc.GetHousehold(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].DisplayName != "Dana" || members[0].AvatarColor != "#7B5EA7" ||
		members[0].Email != "dana@example.com" || members[0].Role != "owner" {
		t.Fatalf("members %+v", members)
	}
}
