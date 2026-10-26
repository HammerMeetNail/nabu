package account

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/household"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

type sharedHousehold struct {
	db                    *sql.DB
	users                 *auth.PostgresStore
	households            *household.PostgresStore
	ann, ben              auth.User
	home                  household.Household
	annLog, benA, benB    int64
	benOther, benOtherHH  int64
	otherHome             household.Household
	benNote, annNote      string
	choreID, otherChoreID int64
}

// newSharedHousehold builds two households Ann and Ben share. Ben logged in
// both, plus one log Ann recorded and one Ben recorded on Ann's behalf.
func newSharedHousehold(t *testing.T) *sharedHousehold {
	t.Helper()
	ctx := context.Background()
	db := testdb.New(t)
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	f := &sharedHousehold{db: db, users: auth.NewPostgresStore(db), households: household.NewPostgresStore(db),
		benNote: "Ben's note about the 3am feed", annNote: "Ann's note"}
	f.ann = mustCreateUser(t, f.users, "ann@example.invalid")
	f.ben = mustCreateUser(t, f.users, "ben@example.invalid")
	var err error
	if f.home, err = f.households.CreateHousehold(ctx, "Home", "H", f.ann.ID); err != nil {
		t.Fatal(err)
	}
	if f.otherHome, err = f.households.CreateHousehold(ctx, "Grandma's", "G", f.ann.ID); err != nil {
		t.Fatal(err)
	}
	for _, hh := range []int64{f.home.ID, f.otherHome.ID} {
		if err := f.households.AddMember(ctx, hh, f.ben.ID, household.RoleMember); err != nil {
			t.Fatal(err)
		}
	}
	mustScan := func(dest *int64, query string, args ...any) {
		t.Helper()
		if err := db.QueryRow(query, args...).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	mustScan(&f.choreID, `INSERT INTO chores (household_id,name) VALUES ($1,'Feed Baby') RETURNING id`, f.home.ID)
	mustScan(&f.otherChoreID, `INSERT INTO chores (household_id,name) VALUES ($1,'Water Plants') RETURNING id`, f.otherHome.ID)
	insert := `INSERT INTO chore_logs (household_id,chore_id,user_id,note,idempotency_actor_id) VALUES ($1,$2,$3,$4,$5) RETURNING id`
	mustScan(&f.annLog, insert, f.home.ID, f.choreID, f.ann.ID, f.annNote, f.ann.ID)
	mustScan(&f.benA, insert, f.home.ID, f.choreID, f.ben.ID, f.benNote, f.ben.ID)
	mustScan(&f.benB, insert, f.home.ID, f.choreID, f.ben.ID, "", f.ben.ID)
	mustScan(&f.benOther, insert, f.otherHome.ID, f.otherChoreID, f.ben.ID, f.benNote, f.ben.ID)
	// Ben logged this one on Ann's behalf.
	mustScan(&f.benOtherHH, insert, f.home.ID, f.choreID, f.ann.ID, "", f.ben.ID)
	return f
}

func (f *sharedHousehold) logUser(t *testing.T, id int64) (int64, string, bool) {
	t.Helper()
	var user int64
	var note string
	err := f.db.QueryRow(`SELECT user_id, COALESCE(note,'') FROM chore_logs WHERE id=$1`, id).Scan(&user, &note)
	if err == sql.ErrNoRows {
		return 0, "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return user, note, true
}

// assertNoTrace fails if any user-reference column holds the deleted user's
// ID, or any text column holds their email.
func assertNoTrace(t *testing.T, db *sql.DB, userID int64, email string) {
	t.Helper()
	rows, err := db.Query(`SELECT table_name, column_name, data_type FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name <> 'schema_migrations'`)
	if err != nil {
		t.Fatal(err)
	}
	type column struct{ table, name, kind string }
	var columns []column
	for rows.Next() {
		var c column
		if err := rows.Scan(&c.table, &c.name, &c.kind); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, c)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	isUserRef := func(name string) bool {
		return strings.Contains(name, "user_id") || strings.HasSuffix(name, "_by") ||
			strings.Contains(name, "actor") || strings.Contains(name, "owner")
	}
	for _, c := range columns {
		var query string
		var arg any
		switch {
		case (c.kind == "bigint" || c.kind == "integer") && isUserRef(c.name):
			query, arg = fmt.Sprintf(`SELECT count(*) FROM %q WHERE %q = $1`, c.table, c.name), userID
		case c.kind == "text" || c.kind == "character varying" || c.kind == "jsonb":
			query, arg = fmt.Sprintf(`SELECT count(*) FROM %q WHERE %q::text ILIKE $1`, c.table, c.name), "%"+email+"%"
		default:
			continue
		}
		var n int
		if err := db.QueryRow(query, arg).Scan(&n); err != nil {
			t.Fatalf("%s.%s: %v", c.table, c.name, err)
		}
		if n > 0 {
			t.Errorf("%s.%s still references the deleted user (%d rows)", c.table, c.name, n)
		}
	}
}

func TestPostgresDeleteAccountKeepsSharedLogsUnderFormerMember(t *testing.T) {
	f := newSharedHousehold(t)
	ctx := context.Background()
	if err := NewService(f.users, f.households).DeleteAccount(ctx, f.ben.ID, auth.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	userA, noteA, ok := f.logUser(t, f.benA)
	if !ok {
		t.Fatal("Ben's shared-household log was deleted")
	}
	userB, _, _ := f.logUser(t, f.benB)
	userOther, _, _ := f.logUser(t, f.benOther)
	if userA >= 0 || userA == -f.ben.ID {
		t.Fatalf("retained log user_id = %d, want an unlinkable negative tombstone", userA)
	}
	if userB != userA {
		t.Fatalf("one household's former member split across tombstones %d and %d", userA, userB)
	}
	if userOther >= 0 || userOther == userA {
		t.Fatalf("other household tombstone = %d, want a distinct negative ID (first was %d)", userOther, userA)
	}
	if noteA != f.benNote {
		t.Fatalf("note = %q, want it kept by default", noteA)
	}
	if user, note, ok := f.logUser(t, f.annLog); !ok || user != f.ann.ID || note != f.annNote {
		t.Fatalf("Ann's log changed: %d %q %v", user, note, ok)
	}
	if user, _, ok := f.logUser(t, f.benOtherHH); !ok || user != f.ann.ID {
		t.Fatalf("log Ben recorded for Ann changed owner: %d %v", user, ok)
	}

	// The household now lists the tombstone as an anonymous former member.
	if err := f.households.SetActiveHousehold(ctx, f.ann.ID, f.home.ID); err != nil {
		t.Fatal(err)
	}
	svc := household.NewService(f.households, noResolver{})
	svc.WithHistoricalMembers(chorelog.NewPostgresStore(f.db), f.users)
	historical, err := svc.GetHistoricalMembers(ctx, f.ann.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(historical) != 1 || historical[0].UserID != userA || historical[0].DisplayName != household.FormerMemberName {
		t.Fatalf("historical members = %+v", historical)
	}

	assertNoTrace(t, f.db, f.ben.ID, f.ben.Email)
}

func TestPostgresDeleteAccountCanDeleteLogsOrNotes(t *testing.T) {
	t.Run("everything I logged", func(t *testing.T) {
		f := newSharedHousehold(t)
		if err := NewService(f.users, f.households).DeleteAccount(context.Background(), f.ben.ID, auth.DeleteOptions{DeleteLogs: true}); err != nil {
			t.Fatal(err)
		}
		for _, id := range []int64{f.benA, f.benB, f.benOther} {
			if _, _, ok := f.logUser(t, id); ok {
				t.Errorf("log %d by the deleted user remains", id)
			}
		}
		for _, id := range []int64{f.annLog, f.benOtherHH} {
			if user, _, ok := f.logUser(t, id); !ok || user != f.ann.ID {
				t.Errorf("Ann's log %d lost or reassigned: %d %v", id, user, ok)
			}
		}
		assertNoTrace(t, f.db, f.ben.ID, f.ben.Email)
	})
	t.Run("notes I wrote", func(t *testing.T) {
		f := newSharedHousehold(t)
		if err := NewService(f.users, f.households).DeleteAccount(context.Background(), f.ben.ID, auth.DeleteOptions{DeleteNotes: true}); err != nil {
			t.Fatal(err)
		}
		for _, id := range []int64{f.benA, f.benOther} {
			user, note, ok := f.logUser(t, id)
			if !ok || user >= 0 || note != "" {
				t.Errorf("log %d: user %d note %q present %v, want kept with the note stripped", id, user, note, ok)
			}
		}
		if _, note, _ := f.logUser(t, f.annLog); note != f.annNote {
			t.Errorf("Ann's note stripped: %q", note)
		}
		assertNoTrace(t, f.db, f.ben.ID, f.ben.Email)
	})
}

type noResolver struct{}

func (noResolver) SetMembershipResolver(auth.MembershipResolver) {}

func TestPostgresListUserLogsCoversEveryHousehold(t *testing.T) {
	f := newSharedHousehold(t)
	logs, err := chorelog.NewPostgresStore(f.db).ListUserLogs(context.Background(), f.ben.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]bool{}
	for _, l := range logs {
		got[l.ID] = true
	}
	if len(logs) != 3 || !got[f.benA] || !got[f.benB] || !got[f.benOther] {
		t.Fatalf("Ben's logs = %v, want %d, %d and %d", got, f.benA, f.benB, f.benOther)
	}
}
