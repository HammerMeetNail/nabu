package household

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/testdb"
	"github.com/HammerMeetNail/nabu/internal/testsync"
)

func TestRevocationWaitsForEnteredDelivery(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		for _, operation := range []string{"remove", "demote"} {
			t.Run(operation+map[bool]string{false: "/memory", true: "/postgres"}[postgres], func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				var store Store
				var home Household
				var ownerID, userID int64
				if postgres {
					_, pg, _, owner, member, hh := postgresLifecycle(t)
					store, home, ownerID, userID = pg, hh, owner.ID, member.ID
				} else {
					memory := NewMemoryStore()
					store = memory
					ownerID, userID = 1, 2
					var err error
					home, err = store.CreateHousehold(ctx, "Test", "T", ownerID)
					if err != nil {
						t.Fatal(err)
					}
					if err := store.AddMember(ctx, home.ID, userID, RoleMember); err != nil {
						t.Fatal(err)
					}
				}
				if err := store.UpdateMemberRole(ctx, home.ID, userID, RoleAdmin); err != nil {
					t.Fatal(err)
				}
				entered := make(chan int, 1)
				resume := make(chan struct{})
				delivery := make(chan error, 1)
				go func() {
					delivery <- WithMember(ctx, store, userID, home.ID, func(role string) error {
						if role != RoleAdmin {
							return ErrNotAuthorized
						}
						pid := 0
						if pg, ok := store.(*PostgresStore); ok {
							if err := pg.db.QueryRowContext(ctx, `SELECT pid FROM pg_stat_activity WHERE application_name=current_setting('application_name') AND state='idle in transaction' AND query LIKE '%SELECT role FROM user_households%'`).Scan(&pid); err != nil {
								return err
							}
						}
						entered <- pid
						testsync.Pause(ctx, resume)
						return ctx.Err()
					})
				}()
				pid := testsync.Receive(t, ctx, entered)
				if memory, ok := store.(*MemoryStore); ok && memory.mu.TryLock() {
					memory.mu.Unlock()
					t.Fatal("delivery callback did not retain the membership guard")
				}
				mutation := make(chan error, 1)
				svc := NewService(store, nil)
				go func() {
					if operation == "remove" {
						mutation <- svc.RemoveMember(ctx, ownerID, userID)
					} else {
						mutation <- svc.UpdateMemberRole(ctx, ownerID, userID, RoleMember)
					}
				}()
				if pg, ok := store.(*PostgresStore); ok {
					testdb.WaitBlocked(t, pg.db, pid, "pg_advisory_xact_lock")
				} else {
					memory := store.(*MemoryStore)
					// An existing delivery reader plus a pending writer denies new readers.
					// This observes the removal at the mutex, not a guessed goroutine delay.
					testsync.Eventually(t, ctx, func() bool {
						if memory.mu.TryRLock() {
							memory.mu.RUnlock()
							return false
						}
						return true
					})
				}
				close(resume)
				if err := testsync.Receive(t, ctx, delivery); err != nil {
					t.Fatal(err)
				}
				if err := testsync.Receive(t, ctx, mutation); err != nil {
					t.Fatal(err)
				}
				allowed := false
				err := WithMember(ctx, store, userID, home.ID, func(role string) error {
					if role != RoleOwner && role != RoleAdmin {
						return ErrNotAuthorized
					}
					allowed = true
					return nil
				})
				if allowed || (!errors.Is(err, ErrNotMember) && !errors.Is(err, ErrNotAuthorized)) {
					t.Fatalf("delivery authorized after %s: %v", operation, err)
				}
			})
		}
	}
}
