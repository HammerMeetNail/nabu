package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

// A user's sign-in list holds only their live sessions, marks the current
// one, and never a token or hash.
func TestListUserSessionsShowsOnlyLiveSignIns(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		name := "memory"
		if postgres {
			name = "postgres"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var store Store = NewMemoryStore()
			var db *sql.DB
			if postgres {
				db = testdb.New(t)
				if err := database.Migrate(ctx, db); err != nil {
					t.Fatal(err)
				}
				store = NewPostgresStore(db)
			}
			ann, err := store.CreateUser(ctx, "ann@example.invalid", "hash")
			if err != nil {
				t.Fatal(err)
			}
			bob, err := store.CreateUser(ctx, "bob@example.invalid", "hash")
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			for _, s := range []struct {
				user    User
				hash    string
				expires time.Time
			}{
				{ann, "current", now.Add(time.Hour)},
				{ann, "phone", now.Add(time.Hour)},
				{ann, "lapsed", now.Add(time.Hour)},
				{ann, "idle", now.Add(time.Hour)},
				{bob, "bob", now.Add(time.Hour)},
			} {
				if _, err := store.CreateSession(ctx, s.user.ID, s.user.AuthVersion, s.hash, s.expires, now); err != nil {
					t.Fatal(err)
				}
			}
			if postgres {
				// Ended by the database's clock, which the list compares against.
				for _, query := range []string{
					`UPDATE sessions SET expires_at = clock_timestamp() - INTERVAL '1 second' WHERE token_hash = 'lapsed'`,
					`UPDATE sessions SET last_seen_at = clock_timestamp() - ` + lifecycle.SessionIdleSQL + ` - INTERVAL '1 minute' WHERE token_hash = 'idle'`,
				} {
					if _, err := db.ExecContext(ctx, query); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				memory := store.(*MemoryStore)
				memory.mu.Lock()
				lapsed := memory.sessionsByHash["lapsed"]
				lapsed.ExpiresAt = now.Add(-time.Second)
				memory.sessionsByHash["lapsed"] = lapsed
				memory.mu.Unlock()
				if err := store.TouchSession(ctx, "idle", now.Add(-lifecycle.SessionIdleTimeout-time.Minute)); err != nil {
					t.Fatal(err)
				}
			}

			sessions, err := store.ListUserSessions(ctx, ann.ID, "current")
			if err != nil {
				t.Fatal(err)
			}
			if len(sessions) != 2 {
				t.Fatalf("sessions = %+v, want the two live ones", sessions)
			}
			current := 0
			for _, s := range sessions {
				if s.Current {
					current++
				}
				if s.CreatedAt.IsZero() || s.LastSeenAt.IsZero() || !s.ExpiresAt.After(s.CreatedAt) {
					t.Fatalf("session times = %+v", s)
				}
			}
			if current != 1 {
				t.Fatalf("current sign-ins = %d, want 1", current)
			}
			none, err := store.ListUserSessions(ctx, ann.ID+bob.ID+100, "")
			if err != nil || none == nil || len(none) != 0 {
				t.Fatalf("unknown user's sessions = %+v, %v; want []", none, err)
			}
		})
	}
}
