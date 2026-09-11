package log_test

import (
	"context"
	chorelog "github.com/HammerMeetNail/nabu/internal/log"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func TestPostgresConcurrentIdempotencyReplay(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		db := testdb.New(t)
		ctx := context.Background()
		if err := database.Migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
		for _, query := range []string{
			`INSERT INTO households (id,name,invite_code) VALUES (1,'Test','SYNTHETIC')`,
			`INSERT INTO users (id,email,password_hash,display_name) VALUES (10,'synthetic@example.invalid','','Test')`,
			`INSERT INTO chores (id,household_id,name) VALUES (100,1,'Test')`,
		} {
			if _, err := db.ExecContext(ctx, query); err != nil {
				t.Fatal(err)
			}
		}
		checkConcurrentReplay(t, chorelog.NewPostgresStore(db), chorelog.CreateInput{HouseholdID: 1, ActorID: 10, UserID: 10, ChoreID: 100, Note: "original", IdempotencyKey: "race"}, mismatch)
	}
}
