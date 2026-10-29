package operator

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func at(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}

// dailyFixture records a known week around 2026-03-10 (UTC):
//
//   - two accounts sign up on the 10th; one logs within ten minutes, one later
//   - household A (created the 3rd, a week earlier) logs on the 10th, by both
//   - household B (also created the 3rd) logged last just before the week
//   - household C (created 8 February, thirty days earlier) logs on the 10th,
//     and earlier in the week by a second person
func dailyFixture(t *testing.T, db *sql.DB) (owner int64) {
	t.Helper()
	user := func(email, created string) int64 {
		var id int64
		if err := db.QueryRow(`INSERT INTO users(email,password_hash,display_name,email_verified,created_at)
 VALUES($1,'hash',$1,true,$2) RETURNING id`, email, at(created)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	household := func(name, created string) (int64, int64) {
		var id, chore int64
		if err := db.QueryRow(`INSERT INTO households(name,invite_code,created_at) VALUES($1,$1,$2) RETURNING id`, name, at(created)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`INSERT INTO chores(household_id,name) VALUES($1,'Feed') RETURNING id`, id).Scan(&chore); err != nil {
			t.Fatal(err)
		}
		return id, chore
	}
	logged := func(household, chore, actor int64, created string) {
		mustExec(t, db, `INSERT INTO chore_logs(household_id,user_id,chore_id,completed_at,created_at,idempotency_actor_id)
 VALUES($1,$2,$3,$4,$4,$2)`, household, actor, chore, at(created))
	}
	quick := user("quick@example.invalid", "2026-03-10T09:00:00Z")
	slow := user("slow@example.invalid", "2026-03-10T12:00:00Z")
	earlier := user("earlier@example.invalid", "2026-03-09T08:00:00Z")
	a, aChore := household("A", "2026-03-03T10:00:00Z")
	b, bChore := household("B", "2026-03-03T11:00:00Z")
	c, cChore := household("C", "2026-02-08T10:00:00Z")
	logged(a, aChore, quick, "2026-03-10T09:05:00Z")
	logged(a, aChore, slow, "2026-03-10T12:10:01Z") // just past ten minutes
	logged(c, cChore, earlier, "2026-03-10T15:00:00Z")
	logged(c, cChore, quick, "2026-03-05T08:00:00Z")
	logged(b, bChore, quick, "2026-03-03T23:59:59Z") // the day before the week begins
	logged(b, bChore, slow, "2026-03-03T23:59:59Z")
	return quick
}

func dailyDB(t *testing.T) *sql.DB {
	t.Helper()
	db := testdb.New(t)
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func storedDay(t *testing.T, db *sql.DB, day string) DailyTotal {
	t.Helper()
	var got DailyTotal
	if err := db.QueryRow(`SELECT signups, activated, logs, logging_households, shared_households_7d,
  cohort_d7, returned_d7, cohort_d30, returned_d30 FROM daily_totals WHERE day = $1::date`, day).Scan(
		&got.Signups, &got.Activated, &got.Logs, &got.LoggingHouseholds, &got.SharedHouseholds7Days,
		&got.CohortD7, &got.ReturnedD7, &got.CohortD30, &got.ReturnedD30); err != nil {
		t.Fatal(err)
	}
	return got
}

var march10 = DailyTotal{Signups: 2, Activated: 1, Logs: 3, LoggingHouseholds: 2,
	SharedHouseholds7Days: 2, CohortD7: 2, ReturnedD7: 1, CohortD30: 1, ReturnedD30: 1}

func TestDailyRollupCountsAFinishedDay(t *testing.T) {
	db := dailyDB(t)
	dailyFixture(t, db)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := rollUpDay(ctx, tx, at("2026-03-10T00:00:00Z")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := storedDay(t, db, "2026-03-10"); got != march10 {
		t.Fatalf("2026-03-10 = %+v\nwant         %+v", got, march10)
	}
}

func TestDailyRollupFillsEachFinishedDayOnce(t *testing.T) {
	db := dailyDB(t)
	owner := dailyFixture(t, db)
	ctx := context.Background()
	var first, yesterday time.Time
	if err := db.QueryRow(`SELECT '2026-03-09'::date, (now() AT TIME ZONE 'UTC')::date - 1`).Scan(&first, &yesterday); err != nil {
		t.Fatal(err)
	}
	want := int(yesterday.Sub(first).Hours()/24) + 1

	// Another instance holds the rollup: this one writes nothing.
	other, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, dailyLockKey); err != nil {
		t.Fatal(err)
	}
	rollup := NewDailyRollup(db)
	if days, err := rollup.RollUp(ctx); err != nil || days != 0 {
		t.Fatalf("second instance rolled up %d days, %v", days, err)
	}
	_ = other.Rollback()

	// The first rollup counts from the first sign-up to yesterday.
	if days, err := rollup.RollUp(ctx); err != nil || days != want {
		t.Fatalf("first rollup = %d days, %v; want %d", days, err, want)
	}
	if got := storedDay(t, db, "2026-03-10"); got != march10 {
		t.Fatalf("2026-03-10 = %+v", got)
	}
	// Later ones only recount the latest day.
	if days, err := rollup.RollUp(ctx); err != nil || days != 1 {
		t.Fatalf("next rollup = %d days, %v; want 1", days, err)
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM daily_totals`).Scan(&rows); err != nil || rows != want {
		t.Fatalf("stored days = %d, %v; want %d", rows, err, want)
	}

	// The owner's activity window carries each finished day's totals; today
	// has none yet.
	service := NewService(db, owner)
	access, err := service.AuthorizeSession(auth.User{ID: owner, EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	var now time.Time
	if err := db.QueryRow(`SELECT now()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now.UTC() }
	activity, err := service.Activity(ctx, access, 30)
	if err != nil || len(activity.Days) != 30 {
		t.Fatalf("activity = %d days, %v", len(activity.Days), err)
	}
	last, previous := activity.Days[29], activity.Days[28]
	if last.Totals != nil || previous.Date != yesterday.Format("2006-01-02") || previous.Totals == nil {
		t.Fatalf("today = %+v, yesterday = %+v", last, previous)
	}
}
