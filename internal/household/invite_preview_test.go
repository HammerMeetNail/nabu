package household_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

// previewFixture gives Dana (named, purple) and Sam on either store. db is nil
// for the memory store.
func previewFixture(t *testing.T, postgres bool) (*household.Service, household.Store, *sql.DB, auth.User, auth.User) {
	t.Helper()
	var users auth.Store = auth.NewMemoryStore()
	var households household.Store = household.NewMemoryStore()
	var db *sql.DB
	if postgres {
		db = testdb.New(t)
		if err := database.Migrate(context.Background(), db); err != nil {
			t.Fatal(err)
		}
		users, households = auth.NewPostgresStore(db), household.NewPostgresStore(db)
	}
	ctx := context.Background()
	authService := auth.NewService(users)
	authService.SetAuditLogger(nil)
	dana, err := users.CreateUser(ctx, "dana@example.invalid", "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	sam, err := users.CreateUser(ctx, "sam@example.invalid", "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	name, color := "Dana O'Connor", "#7B5EA7"
	if dana, err = authService.UpdateProfile(ctx, dana, &name, &color); err != nil {
		t.Fatal(err)
	}
	return household.NewService(households, authService), households, db, dana, sam
}

func TestPreviewInvite(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[postgres], func(t *testing.T) {
			svc, store, db, dana, sam := previewFixture(t, postgres)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			hh, err := svc.CreateHousehold(ctx, "The O'Connors' home", "TOH", dana.ID)
			if err != nil {
				t.Fatal(err)
			}
			fromDana := household.InvitePreview{
				Household: household.InvitePreviewHousehold{Name: "The O'Connors' home", Initials: "TOH"},
				Inviter:   &household.InvitePreviewPerson{DisplayName: "Dana O'Connor", AvatarColor: "#7B5EA7"},
			}
			expect := func(code string, want household.InvitePreview) {
				t.Helper()
				got, err := svc.PreviewInvite(ctx, code)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("PreviewInvite = %+v (inviter %+v), %v; want %+v (inviter %+v)", got, got.Inviter, err, want, want.Inviter)
				}
			}
			expectInvalid := func(code string) {
				t.Helper()
				if got, err := svc.PreviewInvite(ctx, code); !errors.Is(err, household.ErrInviteNotFound) {
					t.Fatalf("PreviewInvite(%q) = %+v, %v; want ErrInviteNotFound", code, got, err)
				}
			}

			// The permanent code names the household's only owner, and a
			// single-use invite names the member who made it.
			expect(hh.InviteCode, fromDana)
			invite, err := svc.CreateInvite(ctx, dana.ID)
			if err != nil {
				t.Fatal(err)
			}
			expect(invite.Code, fromDana)

			// The answer carries no IDs, emails or codes.
			preview, _ := svc.PreviewInvite(ctx, invite.Code)
			body, _ := json.Marshal(preview)
			if string(body) != `{"household":{"name":"The O'Connors' home","initials":"TOH"},"inviter":{"displayName":"Dana O'Connor","avatarColor":"#7B5EA7"}}` {
				t.Fatalf("preview JSON = %s", body)
			}

			// Previewing uses nothing: the invite still admits Sam, and then,
			// used up, previews like an unknown code (as a join would refuse it).
			if _, err := svc.JoinHousehold(ctx, sam.ID, invite.Code); err != nil {
				t.Fatalf("join after preview: %v", err)
			}
			expectInvalid(invite.Code)
			for _, code := range []string{"", "NOPE234567", strings.Repeat("A", 65)} {
				expectInvalid(code)
			}

			// The service keeps one owner. Should a household have two, the
			// permanent code names no one; a single-use invite still names
			// its maker.
			if err := store.UpdateMemberRole(ctx, hh.ID, sam.ID, household.RoleOwner); err != nil {
				t.Fatal(err)
			}
			expect(hh.InviteCode, household.InvitePreview{Household: fromDana.Household})
			second, err := svc.CreateInvite(ctx, dana.ID)
			if err != nil {
				t.Fatal(err)
			}
			expect(second.Code, fromDana)

			// A maker whose access is disabled is not named.
			if err := store.SetMemberSuspended(ctx, hh.ID, dana.ID, true); err != nil {
				t.Fatal(err)
			}
			expect(second.Code, household.InvitePreview{Household: fromDana.Household})

			if db != nil {
				// An expired invite reads as unknown.
				if err := store.SetMemberSuspended(ctx, hh.ID, dana.ID, false); err != nil {
					t.Fatal(err)
				}
				expect(second.Code, fromDana)
				if _, err := db.ExecContext(ctx, `UPDATE invites SET expires_at = now() - interval '1 minute' WHERE code = $1`, second.Code); err != nil {
					t.Fatal(err)
				}
				expectInvalid(second.Code)
			}
		})
	}
}
