package schedule

import (
	"context"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func TestPostgresLogEffectsRollbackAndRetry(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO households (id,name,invite_code) VALUES (1,'Test','SYNTHETIC')`,
		`INSERT INTO users (id,email,password_hash,display_name) VALUES (1,'synthetic@example.invalid','','Test')`,
		`INSERT INTO chores (id,household_id,name) VALUES (1,1,'Test')`,
		`INSERT INTO chore_logs (id,household_id,user_id,chore_id) VALUES (1,1,1,1)`,
		`INSERT INTO chore_schedules (household_id,chore_id,is_follow_up) VALUES (1,1,TRUE)`,
		`CREATE FUNCTION fail_schedule_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic insert failure'; END $$`,
		`CREATE TRIGGER fail_insert BEFORE INSERT ON chore_schedules FOR EACH ROW EXECUTE FUNCTION fail_schedule_insert()`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	store := NewPostgresStore(db)
	day := DateOnly{Time: time.Now().UTC()}
	e := LogEffects{LogID: 1, HouseholdID: 1, ChoreID: 1, UpdateFollowUp: true, LastFollowUpMinutes: 60, FollowUp: &ChoreSchedule{SpecificTime: "15:00", StartDate: &day}}
	if applied, err := store.ApplyLogEffects(ctx, e); err == nil || applied {
		t.Fatalf("injected failure applied=%v err=%v", applied, err)
	}
	var effects, remaining int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chore_log_effects`).Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chore_schedules`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if effects != 0 || remaining != 1 {
		t.Fatalf("partial transaction: effects=%d schedules=%d", effects, remaining)
	}
	if _, err := db.ExecContext(ctx, `DROP TRIGGER fail_insert ON chore_schedules`); err != nil {
		t.Fatal(err)
	}
	if applied, err := store.ApplyLogEffects(ctx, e); err != nil || !applied {
		t.Fatalf("retry applied=%v err=%v", applied, err)
	}
	first, err := store.ListByHousehold(ctx, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("schedules=%v err=%v", first, err)
	}
	if applied, err := store.ApplyLogEffects(ctx, e); err != nil || applied {
		t.Fatalf("replay applied=%v err=%v", applied, err)
	}
	last, err := store.ListByHousehold(ctx, 1)
	if err != nil || len(last) != 1 || last[0].ID != first[0].ID {
		t.Fatalf("duplicate effect: %v err=%v", last, err)
	}
}
