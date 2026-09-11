package household_test

import (
	"context"
	"fmt"
	"github.com/HammerMeetNail/nabu/internal/account"
	"github.com/HammerMeetNail/nabu/internal/testsync"
	"sync"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func lifecycleFixture(t *testing.T, postgres bool) (*household.Service, household.Store, auth.Store, *auth.Service, auth.User, auth.User) {
	t.Helper()
	var users auth.Store = auth.NewMemoryStore()
	var households household.Store = household.NewMemoryStore()
	if postgres {
		db := testdb.New(t)
		if err := database.Migrate(context.Background(), db); err != nil {
			t.Fatal(err)
		}
		users, households = auth.NewPostgresStore(db), household.NewPostgresStore(db)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	owner, err := users.CreateUser(ctx, "owner@example.invalid", "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	member, err := users.CreateUser(ctx, "member@example.invalid", "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	authService := auth.NewService(users)
	return household.NewService(households, authService), households, users, authService, owner, member
}

func TestRemovalRevokesPreviouslyDisclosedInvites(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[postgres], func(t *testing.T) {
			svc, store, _, _, owner, member := lifecycleFixture(t, postgres)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			hh, err := svc.CreateHousehold(ctx, "Test", "T", owner.ID)
			if err != nil {
				t.Fatal(err)
			}
			known, err := svc.CreateInvite(ctx, owner.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.JoinHousehold(ctx, member.ID, hh.InviteCode); err != nil {
				t.Fatal(err)
			}
			if err := svc.RemoveMember(ctx, owner.ID, member.ID); err != nil {
				t.Fatal(err)
			}
			for _, code := range []string{hh.InviteCode, known.Code} {
				if _, err := svc.JoinHousehold(ctx, member.ID, code); err == nil {
					t.Errorf("removed member rejoined with old code")
				}
			}
			if _, err := store.GetMembershipForHousehold(ctx, member.ID, hh.ID); err == nil {
				t.Fatal("revoked membership was restored")
			}
			fresh, err := svc.CreateInvite(ctx, owner.ID)
			if err != nil {
				t.Fatal(err)
			}
			if fresh.MaxUses != 1 || fresh.ExpiresAt == nil {
				t.Fatal("new invite must be one-use and expiring")
			}
			if _, err := svc.JoinHousehold(ctx, member.ID, fresh.Code); err != nil {
				t.Fatalf("fresh invite rejected: %v", err)
			}
		})
	}
}

func TestOwnerCanManageMemberActiveInAnotherHousehold(t *testing.T) {
	for _, operation := range []string{"remove", "demote"} {
		for _, postgres := range []bool{false, true} {
			t.Run(operation+map[bool]string{false: "/memory", true: "/postgres"}[postgres], func(t *testing.T) {
				svc, store, _, _, owner, member := lifecycleFixture(t, postgres)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				hh, err := svc.CreateHousehold(ctx, "A", "A", owner.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err := store.AddMember(ctx, hh.ID, member.ID, household.RoleAdmin); err != nil {
					t.Fatal(err)
				}
				if _, err := svc.CreateHousehold(ctx, "B", "B", member.ID); err != nil {
					t.Fatal(err)
				}
				if operation == "remove" {
					if err := svc.RemoveMember(ctx, owner.ID, member.ID); err != nil {
						t.Fatal(err)
					}
					if _, err := store.GetMembershipForHousehold(ctx, member.ID, hh.ID); err == nil {
						t.Fatal("membership remains")
					}
				} else {
					if err := svc.UpdateMemberRole(ctx, owner.ID, member.ID, household.RoleMember); err != nil {
						t.Fatal(err)
					}
					if role, err := store.GetMembershipForHousehold(ctx, member.ID, hh.ID); err != nil || role != household.RoleMember {
						t.Fatalf("role unchanged: %s %v", role, err)
					}
				}
			})
		}
	}
}

type pauseMembershipRead struct {
	household.Store
	read   chan struct{}
	resume chan struct{}
	once   sync.Once
}

func (s *pauseMembershipRead) GetMembership(ctx context.Context, uid int64) (int64, string, error) {
	id, role, err := s.Store.GetMembership(ctx, uid)
	s.once.Do(func() { close(s.read); testsync.Pause(ctx, s.resume) })
	return id, role, err
}

func TestRemovedOwnerCannotReadNewInviteCapabilities(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		for _, kind := range []string{"household", "invites"} {
			t.Run(fmt.Sprintf("%s/postgres=%t", kind, postgres), func(t *testing.T) {
				svc, store, _, _, owner, departing := lifecycleFixture(t, postgres)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				hh, err := svc.CreateHousehold(ctx, "Shared", "S", owner.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err := store.AddMember(ctx, hh.ID, departing.ID, household.RoleOwner); err != nil {
					t.Fatal(err)
				}
				paused := &pauseMembershipRead{Store: store, read: make(chan struct{}), resume: make(chan struct{})}
				reader := household.NewService(paused, nil)
				done := make(chan error, 1)
				go func() {
					if kind == "household" {
						_, _, err := reader.GetHousehold(ctx, departing.ID)
						done <- err
					} else {
						_, err := reader.GetInvites(ctx, departing.ID)
						done <- err
					}
				}()
				testsync.Receive(t, ctx, paused.read)
				if err := svc.RemoveMember(ctx, owner.ID, departing.ID); err != nil {
					close(paused.resume)
					t.Fatal(err)
				}
				if _, err := svc.CreateInvite(ctx, owner.ID); err != nil {
					close(paused.resume)
					t.Fatal(err)
				}
				close(paused.resume)
				if err := testsync.Receive(t, ctx, done); err == nil {
					t.Fatal("removed owner received freshly issued invitation capability")
				}
			})
		}
	}
}

func TestDeletedUserCannotResumeHouseholdCreationOrJoin(t *testing.T) {
	for _, operation := range []string{"create", "join"} {
		t.Run(operation, func(t *testing.T) {
			svc, store, users, _, owner, deleted := lifecycleFixture(t, false)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			hh, err := svc.CreateHousehold(ctx, "Shared", "S", owner.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := account.NewService(users, store).DeleteAccount(ctx, deleted.ID); err != nil {
				t.Fatal(err)
			}
			if operation == "create" {
				_, err = svc.CreateHousehold(ctx, "Orphan", "O", deleted.ID)
			} else {
				_, err = svc.JoinHousehold(ctx, deleted.ID, hh.InviteCode)
			}
			if err == nil {
				t.Fatal("stale authenticated request recreated deleted-user membership")
			}
			memberships, err := store.ListUserHouseholds(ctx, deleted.ID)
			if err != nil || len(memberships) != 0 {
				t.Fatalf("orphan membership remains: %v %v", memberships, err)
			}
		})
	}
}
