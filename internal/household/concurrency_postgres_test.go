package household

import (
	"context"
	"database/sql"
	"errors"
	"github.com/HammerMeetNail/nabu/internal/testsync"
	"sync"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

type membershipBarrier struct {
	Store
	target  int64
	entered chan int
	resume  chan struct{}
	once    *sync.Once
}

func (s *membershipBarrier) InTransaction(ctx context.Context, fn func(Store) error) error {
	return s.Store.InTransaction(ctx, func(store Store) error { copy := *s; copy.Store = store; return fn(&copy) })
}
func (s *membershipBarrier) GetMembershipForHousehold(ctx context.Context, userID, householdID int64) (string, error) {
	role, err := s.Store.GetMembershipForHousehold(ctx, userID, householdID)
	if userID == s.target {
		s.once.Do(func() {
			var pid int
			_ = s.Store.(*PostgresStore).q.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid)
			s.entered <- pid
			testsync.Pause(ctx, s.resume)
		})
	}
	return role, err
}

func postgresLifecycle(t *testing.T) (*sql.DB, *PostgresStore, *auth.PostgresStore, auth.User, auth.User, Household) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db := testdb.New(t)
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	users := auth.NewPostgresStore(db)
	store := NewPostgresStore(db)
	owner, err := users.CreateUser(ctx, "owner@example.invalid", "")
	if err != nil {
		t.Fatal(err)
	}
	member, err := users.CreateUser(ctx, "member@example.invalid", "")
	if err != nil {
		t.Fatal(err)
	}
	hh, err := store.CreateHousehold(ctx, "Shared", "S", owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddMember(ctx, hh.ID, member.ID, RoleMember); err != nil {
		t.Fatal(err)
	}
	return db, store, users, owner, member, hh
}

func TestPostgresSwitchRemovalSerializesBothOrders(t *testing.T) {
	for _, first := range []string{"switch", "remove"} {
		t.Run(first, func(t *testing.T) {
			db, store, users, owner, member, hh := postgresLifecycle(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			other, err := store.CreateHousehold(ctx, "Other", "O", member.ID)
			if err != nil {
				t.Fatal(err)
			}
			blocked := &membershipBarrier{Store: store, target: member.ID, entered: make(chan int, 1), resume: make(chan struct{}), once: &sync.Once{}}
			svc := NewService(blocked, nil)
			plain := NewService(store, nil)
			early, late := make(chan error, 1), make(chan error, 1)
			if first == "switch" {
				go func() { early <- svc.SwitchHousehold(ctx, member.ID, hh.ID) }()
			} else {
				go func() { early <- svc.RemoveMember(ctx, owner.ID, member.ID) }()
			}
			pid := testsync.Receive(t, ctx, blocked.entered)
			if first == "switch" {
				go func() { late <- plain.RemoveMember(ctx, owner.ID, member.ID) }()
			} else {
				go func() { late <- plain.SwitchHousehold(ctx, member.ID, hh.ID) }()
			}
			testdb.WaitBlocked(t, db, pid, "pg_advisory_xact_lock")
			close(blocked.resume)
			if err := testsync.Receive(t, ctx, early); err != nil {
				t.Fatal(err)
			}
			err = testsync.Receive(t, ctx, late)
			if first == "switch" && err != nil {
				t.Fatal(err)
			}
			if first == "remove" && !errors.Is(err, ErrNotMember) {
				t.Fatalf("switch after removal: %v", err)
			}
			u, err := users.GetUserByID(ctx, member.ID)
			if err != nil || u.HouseholdID == nil || *u.HouseholdID != other.ID {
				t.Fatalf("canonical identity restored revoked household: %+v %v", u, err)
			}
			if _, err := store.GetMembershipForHousehold(ctx, member.ID, hh.ID); !errors.Is(err, ErrNotMember) {
				t.Fatalf("revoked membership: %v", err)
			}
		})
	}
}

func TestPostgresActivationWriteFailureDoesNotChangeContext(t *testing.T) {
	db, store, users, _, member, hh := postgresLifecycle(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	other, err := store.CreateHousehold(ctx, "Other", "O", member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE FUNCTION reject_activation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic pointer failure'; END $$;
 CREATE TRIGGER reject_activation BEFORE UPDATE OF active_household_id ON users FOR EACH ROW EXECUTE FUNCTION reject_activation()`); err != nil {
		t.Fatal(err)
	}
	if err := NewService(store, nil).SwitchHousehold(ctx, member.ID, hh.ID); err == nil {
		t.Fatal("expected write failure")
	}
	u, err := users.GetUserByID(ctx, member.ID)
	if err != nil || u.HouseholdID == nil || *u.HouseholdID != other.ID {
		t.Fatalf("partial pointer change: %+v %v", u, err)
	}
}

func TestPostgresOwnershipTransferRollbackAndConcurrency(t *testing.T) {
	db, store, users, owner, member, hh := postgresLifecycle(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	svc := NewService(store, nil)
	if _, err := db.Exec(`CREATE FUNCTION reject_promotion() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.role='owner' THEN RAISE EXCEPTION 'synthetic promotion failure'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER reject_promotion BEFORE UPDATE OF role ON user_households FOR EACH ROW EXECUTE FUNCTION reject_promotion()`); err != nil {
		t.Fatal(err)
	}
	if err := svc.TransferOwnership(ctx, owner.ID, member.ID); err == nil {
		t.Fatal("expected promotion failure")
	}
	if role, err := store.GetMembershipForHousehold(ctx, owner.ID, hh.ID); err != nil || role != RoleOwner {
		t.Fatalf("owner demotion escaped rollback: %s %v", role, err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_promotion ON user_households`); err != nil {
		t.Fatal(err)
	}
	third, err := users.CreateUser(ctx, "third@example.invalid", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddMember(ctx, hh.ID, third.ID, RoleMember); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, id := range []int64{member.ID, third.ID} {
		go func(id int64) { <-start; results <- svc.TransferOwnership(ctx, owner.ID, id) }(id)
	}
	close(start)
	successes := 0
	for range 2 {
		if err := testsync.Receive(t, ctx, results); err == nil {
			successes++
		} else if !errors.Is(err, ErrNotAuthorized) {
			t.Fatal(err)
		}
	}
	members, err := store.GetMembers(ctx, hh.ID)
	if err != nil {
		t.Fatal(err)
	}
	owners := 0
	for _, m := range members {
		if m.Role == RoleOwner {
			owners++
		}
	}
	if successes != 1 || owners != 1 {
		t.Fatalf("concurrent transfers: successes=%d owners=%d", successes, owners)
	}
}

func TestPostgresInviteFailedJoinRetainsUseAndConcurrentJoinUsesOnce(t *testing.T) {
	db, store, users, owner, _, hh := postgresLifecycle(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	svc := NewService(store, nil)
	inv, err := svc.CreateInvite(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	joining, err := users.CreateUser(ctx, "joining@example.invalid", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE FUNCTION reject_join() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic member failure'; END $$;
 CREATE TRIGGER reject_join BEFORE INSERT ON user_households FOR EACH ROW EXECUTE FUNCTION reject_join()`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinHousehold(ctx, joining.ID, inv.Code); err == nil {
		t.Fatal("expected membership failure")
	}
	saved, err := store.GetInviteByID(ctx, inv.ID)
	if err != nil || saved.UsedCount != 0 {
		t.Fatalf("invite consumed on failed join: %+v %v", saved, err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_join ON user_households`); err != nil {
		t.Fatal(err)
	}
	other, err := users.CreateUser(ctx, "other@example.invalid", "")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, id := range []int64{joining.ID, other.ID} {
		go func(id int64) { <-start; _, err := svc.JoinHousehold(ctx, id, inv.Code); done <- err }(id)
	}
	close(start)
	successes := 0
	for range 2 {
		if err := testsync.Receive(t, ctx, done); err == nil {
			successes++
		}
	}
	members, err := store.GetMembers(ctx, hh.ID)
	if err != nil {
		t.Fatal(err)
	}
	if successes != 1 || len(members) != 3 {
		t.Fatalf("one-use join: successes=%d members=%d", successes, len(members))
	}
}
