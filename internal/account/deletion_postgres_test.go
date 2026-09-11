package account

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/HammerMeetNail/nabu/internal/schedule"
	"github.com/HammerMeetNail/nabu/internal/testsync"
	"sync"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func TestPostgresDeleteAccountRollsBackEveryHousehold(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	users, households := auth.NewPostgresStore(db), household.NewPostgresStore(db)
	u := mustCreateUser(t, users, "deleting@example.invalid")
	first, err := households.CreateHousehold(ctx, "First", "F", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := households.CreateHousehold(ctx, "Second", "S", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE FUNCTION reject_second_household() RETURNS trigger AS $$
		BEGIN IF OLD.name = 'Second' THEN RAISE EXCEPTION 'synthetic deletion failure'; END IF; RETURN OLD; END; $$ LANGUAGE plpgsql;
		CREATE TRIGGER reject_second_household BEFORE DELETE ON households FOR EACH ROW EXECUTE FUNCTION reject_second_household()`)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewService(users, households).DeleteAccount(ctx, u.ID); err == nil {
		t.Fatal("expected injected failure")
	}
	for _, id := range []int64{first.ID, second.ID} {
		if _, err := households.GetHousehold(ctx, id); err != nil {
			t.Errorf("household %d partially deleted: %v", id, err)
		}
		if _, err := households.GetMembershipForHousehold(ctx, u.ID, id); err != nil {
			t.Errorf("membership partially deleted: %v", err)
		}
	}
	if _, err := users.GetUserByID(ctx, u.ID); err != nil {
		t.Fatalf("account partially deleted: %v", err)
	}
}

func TestPostgresDeleteMixedAccountPreservesOtherUsersAndSharedContent(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	users, households := auth.NewPostgresStore(db), household.NewPostgresStore(db)
	u := mustCreateUser(t, users, "deleting@example.invalid")
	other := mustCreateUser(t, users, "remaining@example.invalid")
	solo, err := households.CreateHousehold(ctx, "Solo", "S", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := households.CreateHousehold(ctx, "Shared", "S", other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := households.AddMember(ctx, shared.ID, u.ID, household.RoleMember); err != nil {
		t.Fatal(err)
	}
	if err := households.SetActiveHousehold(ctx, u.ID, solo.ID); err != nil {
		t.Fatal(err)
	}
	var choreID int64
	if err := db.QueryRow(`INSERT INTO chores (household_id,name,created_by,visibility) VALUES ($1,'Retained private chore',$2,'admins') RETURNING id`, shared.ID, u.ID).Scan(&choreID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chore_schedules (household_id,chore_id,assigned_to_user_id) VALUES ($1,$2,$3)`, shared.ID, choreID, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := households.CreateInvite(ctx, shared.ID, u.ID, "SYNTHETIC-CODE", 1); err != nil {
		t.Fatal(err)
	}
	if err := NewService(users, households).DeleteAccount(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := users.GetUserByID(ctx, u.ID); err == nil {
		t.Fatal("deleted account remains")
	}
	if _, err := users.GetUserByID(ctx, other.ID); err != nil {
		t.Fatal("another user was deleted")
	}
	if _, err := households.GetHousehold(ctx, solo.ID); err == nil {
		t.Fatal("sole-member household remains")
	}
	var retained bool
	if err := db.QueryRow(`SELECT created_by IS NULL AND visibility='admins' FROM chores WHERE id=$1`, choreID).Scan(&retained); err != nil || !retained {
		t.Fatalf("shared chore not safely retained: %v", err)
	}
	if err := db.QueryRow(`SELECT assigned_to_user_id IS NULL FROM chore_schedules WHERE chore_id=$1`, choreID).Scan(&retained); err != nil || !retained {
		t.Fatalf("assignment not cleared: %v", err)
	}
}

// Holding an advisory gate in a trigger gives tests an exact transaction
// boundary without relying on execution speed or a sleeping goroutine.
func holdGate(t *testing.T, db *sql.DB, key int64) (int, func()) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	if err := conn.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() { _, _ = conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, key); _ = conn.Close() })
	}
	t.Cleanup(release)
	return pid, release
}

func TestPostgresAccountDeletionAndLogEffectsCompleteWithConcurrentNewChore(t *testing.T) {
	for _, phantom := range []bool{false, true} {
		t.Run(fmt.Sprintf("new_chore=%t", phantom), func(t *testing.T) {
			db := testdb.New(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := database.Migrate(ctx, db); err != nil {
				t.Fatal(err)
			}
			users, households := auth.NewPostgresStore(db), household.NewPostgresStore(db)
			deleting := mustCreateUser(t, users, "deleting@example.invalid")
			owner := mustCreateUser(t, users, "remaining@example.invalid")
			hh, err := households.CreateHousehold(ctx, "Shared", "S", owner.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := households.AddMember(ctx, hh.ID, deleting.ID, household.RoleMember); err != nil {
				t.Fatal(err)
			}
			var resumeDelete func()
			deleted := make(chan error, 1)
			if phantom {
				gatePID, release := holdGate(t, db, 9191001)
				resumeDelete = release
				if _, err := db.Exec(`CREATE FUNCTION pause_member_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(9191001); RETURN OLD; END $$;
    CREATE TRIGGER pause_member_delete BEFORE DELETE ON user_households FOR EACH ROW EXECUTE FUNCTION pause_member_delete()`); err != nil {
					t.Fatal(err)
				}
				go func() { deleted <- NewService(users, households).DeleteAccount(ctx, deleting.ID) }()
				testdb.WaitBlocked(t, db, gatePID, "DELETE FROM user_households")
			}
			// In the phantom case these rows arrive after the account's chore prelock.
			var choreID, logID int64
			if err := db.QueryRowContext(ctx, `INSERT INTO chores(household_id,name,created_by) VALUES($1,'Retained',$2) RETURNING id`, hh.ID, deleting.ID).Scan(&choreID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO chore_schedules(household_id,chore_id,assigned_to_user_id,is_follow_up) VALUES($1,$2,$3,TRUE)`, hh.ID, choreID, deleting.ID); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRowContext(ctx, `INSERT INTO chore_logs(household_id,chore_id,user_id) VALUES($1,$2,$3) RETURNING id`, hh.ID, choreID, owner.ID).Scan(&logID); err != nil {
				t.Fatal(err)
			}
			effectGate, resumeEffects := holdGate(t, db, 9191002)
			if _, err := db.Exec(`CREATE FUNCTION pause_effect() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(9191002); RETURN NEW; END $$;
   CREATE TRIGGER pause_effect BEFORE INSERT ON chore_log_effects FOR EACH ROW EXECUTE FUNCTION pause_effect()`); err != nil {
				t.Fatal(err)
			}
			applied := make(chan error, 1)
			go func() {
				_, err := schedule.NewPostgresStore(db).ApplyLogEffects(ctx, schedule.LogEffects{LogID: logID, HouseholdID: hh.ID, ChoreID: choreID, UpdateFollowUp: true, FollowUp: &schedule.ChoreSchedule{SpecificTime: "12:00"}})
				applied <- err
			}()
			effectPID := testdb.WaitBlocked(t, db, effectGate, "INSERT INTO chore_log_effects")
			if phantom {
				resumeDelete()
				testdb.WaitBlocked(t, db, effectPID, "UPDATE chores SET created_by")
			} else {
				go func() { deleted <- NewService(users, households).DeleteAccount(ctx, deleting.ID) }()
				testdb.WaitBlocked(t, db, effectPID, "SELECT id FROM chores")
			}
			resumeEffects()
			if err := testsync.Receive(t, ctx, applied); err != nil {
				t.Fatalf("log effects failed: %v", err)
			}
			if err := testsync.Receive(t, ctx, deleted); err != nil {
				t.Fatalf("account deletion failed: %v", err)
			}
			var clean bool
			if err := db.QueryRowContext(ctx, `SELECT created_by IS NULL FROM chores WHERE id=$1`, choreID).Scan(&clean); err != nil || !clean {
				t.Fatalf("authorship survived deletion: %v", err)
			}
			var assigned int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chore_schedules WHERE assigned_to_user_id=$1`, deleting.ID).Scan(&assigned); err != nil || assigned != 0 {
				t.Fatalf("assignment survived: %d %v", assigned, err)
			}
			if _, err := users.GetUserByID(ctx, owner.ID); err != nil {
				t.Fatal("remaining user was deleted")
			}
		})
	}
}

func TestPostgresJoinWaitsForSoleHouseholdDeletion(t *testing.T) {
	db := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	users, store := auth.NewPostgresStore(db), household.NewPostgresStore(db)
	owner := mustCreateUser(t, users, "sole@example.invalid")
	joining := mustCreateUser(t, users, "joining@example.invalid")
	hh, err := store.CreateHousehold(ctx, "Sole", "S", owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	gate, resume := holdGate(t, db, 9191003)
	if _, err := db.Exec(`CREATE FUNCTION pause_household_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(9191003); RETURN OLD; END $$;
  CREATE TRIGGER pause_household_delete BEFORE DELETE ON households FOR EACH ROW EXECUTE FUNCTION pause_household_delete()`); err != nil {
		t.Fatal(err)
	}
	deleted := make(chan error, 1)
	go func() { deleted <- NewService(users, store).DeleteAccount(ctx, owner.ID) }()
	deletingPID := testdb.WaitBlocked(t, db, gate, "DELETE FROM households")
	joined := make(chan error, 1)
	go func() {
		_, err := household.NewService(store, nil).JoinHousehold(ctx, joining.ID, hh.InviteCode)
		joined <- err
	}()
	testdb.WaitBlocked(t, db, deletingPID, "pg_advisory_xact_lock")
	resume()
	if err := testsync.Receive(t, ctx, deleted); err != nil {
		t.Fatal(err)
	}
	if err := testsync.Receive(t, ctx, joined); err == nil {
		t.Fatal("joined a household being deleted")
	}
	if _, err := users.GetUserByID(ctx, joining.ID); err != nil {
		t.Fatal("joining user's account was cascaded")
	}
}
