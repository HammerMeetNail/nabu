package log_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/household"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func checkRecentAmounts(t *testing.T, store chorelog.Store) {
	t.Helper()
	ctx := context.Background()
	for i, amount := range []int{60, 90, 120, 120, 0} {
		_, err := store.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: 1, ChoreID: 100, UserID: 10, CompletedAt: time.Date(2026, 9, 9, 12, i, 0, 0, time.UTC), VolumeML: &amount})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, hid := range []int64{1, 2} {
		amounts, err := store.RecentAmounts(ctx, hid, 100)
		want := []int{}
		if hid == 1 {
			want = []int{120, 90, 60}
		}
		if err != nil || !reflect.DeepEqual(amounts, want) {
			t.Fatalf("household %d: %v %v", hid, amounts, err)
		}
	}
	_, err := store.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: 1, ChoreID: 100, UserID: 10, CompletedAt: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC), IndicatorVolumes: map[string]int{"A": 150, "B": 90, "C": 0}})
	if err != nil {
		t.Fatal(err)
	}
	amounts, err := store.RecentAmounts(ctx, 1, 100)
	if err != nil || !reflect.DeepEqual(amounts, []int{150, 90, 120}) {
		t.Fatalf("indicator amounts %v: %v", amounts, err)
	}
}

func TestRecentAmountsMemory(t *testing.T) { checkRecentAmounts(t, chorelog.NewMemoryStore()) }
func TestRecentAmountsPostgres(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO households(id,name,invite_code) VALUES(1,'Synthetic','RECENTS')`,
		`INSERT INTO users(id,email,password_hash,display_name) VALUES(10,'recents@example.invalid','','Test')`,
		`INSERT INTO chores(id,household_id,name) VALUES(100,1,'Amount')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	checkRecentAmounts(t, chorelog.NewPostgresStore(db))
}

func TestRecentAmountsRequiresCurrentChoreVisibility(t *testing.T) {
	ctx := context.Background()
	cs := chore.NewMemoryStore()
	hs := household.NewMemoryStore()
	hh, err := hs.CreateHousehold(ctx, "Synthetic", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if err = hs.AddMember(ctx, hh.ID, 11, household.RoleMember); err != nil {
		t.Fatal(err)
	}
	c, err := cs.CreateChore(ctx, chore.Chore{HouseholdID: hh.ID, Name: "Private", Visibility: chore.VisibilityAdmins})
	if err != nil {
		t.Fatal(err)
	}
	ls := chorelog.NewMemoryStore()
	amount := 120
	if _, err = ls.CreateLog(ctx, chorelog.ChoreLog{HouseholdID: hh.ID, ChoreID: c.ID, UserID: 10, VolumeML: &amount}); err != nil {
		t.Fatal(err)
	}
	s := chorelog.NewService(ls).WithAccess(cs, hs)
	got, err := s.RecentAmounts(ctx, 10, hh.ID, c.ID)
	if err != nil || !reflect.DeepEqual(got, []int{120}) {
		t.Fatalf("owner: %v %v", got, err)
	}
	for _, actor := range []int64{11, 12} {
		if values, err := s.RecentAmounts(ctx, actor, hh.ID, c.ID); err == nil || len(values) != 0 {
			t.Fatalf("actor %d leaked %v", actor, values)
		}
	}
	if values, err := s.RecentAmounts(ctx, 10, hh.ID+1, c.ID); err == nil || len(values) != 0 {
		t.Fatalf("foreign household leaked %v", values)
	}
	if err = hs.RemoveMember(ctx, hh.ID, 10); err != nil {
		t.Fatal(err)
	}
	if values, err := s.RecentAmounts(ctx, 10, hh.ID, c.ID); err == nil || len(values) != 0 {
		t.Fatalf("removed owner leaked %v", values)
	}
}
