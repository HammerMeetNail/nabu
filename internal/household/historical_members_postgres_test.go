package household_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/household"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

// The people who left a household but whose logs remain are read in one
// query however many there are; GET /api/household read them one by one.
func TestHistoricalMembersReadInOneQuery(t *testing.T) {
	metrics := database.NewQueryMetrics()
	db := testdb.NewWithTracer(t, metrics)
	ctx := context.Background()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	users := auth.NewPostgresStore(db)
	authService := auth.NewService(users)
	authService.SetAuditLogger(nil)
	svc := household.NewService(household.NewPostgresStore(db), authService)
	svc.WithHistoricalMembers(chorelog.NewPostgresStore(db), authService)

	owner, err := users.CreateUser(ctx, "owner@example.invalid", "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	hh, err := svc.CreateHousehold(ctx, "Home", "H", owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	var choreID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO chores(household_id, name) VALUES ($1, 'Feed') RETURNING id`, hh.ID).Scan(&choreID); err != nil {
		t.Fatal(err)
	}
	departed := 0
	// Each new member logs once, then the owner removes them.
	leave := func(n int) {
		for i := 0; i < n; i++ {
			departed++
			member, err := users.CreateUser(ctx, fmt.Sprintf("member%d@example.invalid", departed), "synthetic")
			if err != nil {
				t.Fatal(err)
			}
			current, _, err := svc.GetHousehold(ctx, owner.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.JoinHousehold(ctx, member.ID, current.InviteCode); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO chore_logs(household_id, user_id, chore_id) VALUES ($1, $2, $3)`, hh.ID, member.ID, choreID); err != nil {
				t.Fatal(err)
			}
			if err := svc.RemoveMember(ctx, owner.ID, member.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	queries := func() (int, uint64) {
		before := metrics.Snapshot().Count
		historical, err := svc.GetHistoricalMembers(ctx, owner.ID)
		if err != nil {
			t.Fatal(err)
		}
		return len(historical), metrics.Snapshot().Count - before
	}

	leave(1)
	one, oneQueries := queries()
	leave(4)
	five, fiveQueries := queries()
	if one != 1 || five != 5 {
		t.Fatalf("historical members = %d then %d, want 1 then 5", one, five)
	}
	if fiveQueries != oneQueries {
		t.Fatalf("queries for 1 former member = %d, for 5 = %d; want the same", oneQueries, fiveQueries)
	}
}
