package push_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/apns"
	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/push"
	"github.com/HammerMeetNail/nabu/internal/testdb"
	"github.com/HammerMeetNail/nabu/internal/testsync"
)

type deviceFixture struct {
	auth   auth.Store
	web    push.Store
	native apns.Store
	db     *sql.DB
}

func deviceStores(t *testing.T, postgres bool) deviceFixture {
	t.Helper()
	if postgres {
		db := testdb.New(t)
		if err := database.Migrate(context.Background(), db); err != nil {
			t.Fatal(err)
		}
		return deviceFixture{auth.NewPostgresStore(db), push.NewPostgresStore(db), apns.NewPostgresStore(db), db}
	}
	a, w, n := auth.NewMemoryStore(), push.NewMemoryStore(), apns.NewMemoryStore()
	w.BindSessions(a)
	n.BindSessions(a)
	return deviceFixture{a, w, n, nil}
}
func newDeviceUser(t *testing.T, f deviceFixture, email string, hashes ...string) auth.User {
	t.Helper()
	u, err := f.auth.CreateUser(context.Background(), email, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	for _, hash := range hashes {
		if _, err := f.auth.CreateSession(context.Background(), u.ID, u.AuthVersion, hash, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	return u
}
func saveDevices(t *testing.T, f deviceFixture, userID int64, session, endpoint string) (push.Subscription, apns.Device) {
	t.Helper()
	ctx := context.Background()
	w := push.Subscription{Endpoint: endpoint, P256DH: "key", Auth: "key", SessionHash: session, BindingID: "12345678-1234-1234-1234-123456789abc"}
	n := apns.Device{UserID: userID, Token: endpoint, Environment: apns.EnvironmentSandbox, BundleID: "com.nabu.test", SessionHash: session}
	if err := f.web.SaveSubscription(ctx, userID, w); err != nil {
		t.Fatal(err)
	}
	if err := f.native.RegisterDevice(ctx, n); err != nil {
		t.Fatal(err)
	}
	web, err := f.web.GetSubscriptions(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	devices, err := f.native.DevicesForUser(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range web {
		if s.Endpoint == endpoint {
			w = s
		}
	}
	for _, d := range devices {
		if d.Token == endpoint {
			n = d
		}
	}
	if w.RegistrationID == "" || n.RegistrationID == "" {
		t.Fatal("missing registration generation")
	}
	return w, n
}
func checkDeviceCount(t *testing.T, f deviceFixture, userID int64, count int) {
	t.Helper()
	w, err := f.web.GetSubscriptions(context.Background(), userID)
	if err != nil || len(w) != count {
		t.Fatalf("web count=%d want=%d error=%v", len(w), count, err)
	}
	n, err := f.native.DevicesForUser(context.Background(), userID)
	if err != nil || len(n) != count {
		t.Fatalf("native count=%d want=%d error=%v", len(n), count, err)
	}
}

func TestDeviceSessionOwnership(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		name := "memory"
		if postgres {
			name = "postgres"
		}
		t.Run(name, func(t *testing.T) {
			f := deviceStores(t, postgres)
			ctx := context.Background()
			a := newDeviceUser(t, f, "a@example.invalid", "a-one", "a-two")
			b := newDeviceUser(t, f, "b@example.invalid", "b-one")
			oldWeb, oldNative := saveDevices(t, f, a.ID, "a-one", "browser-one")
			saveDevices(t, f, a.ID, "a-two", "browser-two")
			if err := f.auth.DeleteSession(ctx, "a-one"); err != nil {
				t.Fatal(err)
			}
			checkDeviceCount(t, f, a.ID, 1)
			if err := f.web.SaveSubscription(ctx, a.ID, oldWeb); err == nil {
				t.Fatal("delayed web registration revived revoked session")
			}
			if err := f.native.RegisterDevice(ctx, oldNative); err == nil {
				t.Fatal("delayed native registration revived revoked session")
			}
			sent := false
			_ = f.web.WithSubscription(ctx, a.ID, oldWeb, func() error { sent = true; return nil })
			_ = f.native.WithDevice(ctx, oldNative, func() error { sent = true; return nil })
			if sent {
				t.Fatal("old snapshot delivered after logout")
			}
			oldWeb, oldNative = saveDevices(t, f, a.ID, "a-two", "browser-two")
			saveDevices(t, f, b.ID, "b-one", "browser-two")
			checkDeviceCount(t, f, a.ID, 0)
			checkDeviceCount(t, f, b.ID, 1)
			_ = f.web.DeleteSubscription(ctx, a.ID, oldWeb.Endpoint, "a-two")
			_ = f.native.UnregisterDevice(ctx, a.ID, oldNative.Token, "a-two")
			_ = f.web.WithSubscription(ctx, a.ID, oldWeb, func() error { sent = true; return nil })
			_ = f.native.WithDevice(ctx, oldNative, func() error { sent = true; return nil })
			if sent {
				t.Fatal("old snapshot delivered after takeover")
			}
			checkDeviceCount(t, f, b.ID, 1)
			if err := f.auth.UpdatePassword(ctx, b.ID, "new-hash"); err != nil {
				t.Fatal(err)
			}
			checkDeviceCount(t, f, b.ID, 0) // even before stale sessions are physically deleted
		})
	}
}

func TestStaleDeviceCleanupAndUnregisterPreserveReplacement(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		name := "memory"
		if postgres {
			name = "postgres"
		}
		t.Run(name, func(t *testing.T) {
			f := deviceStores(t, postgres)
			ctx := context.Background()
			u := newDeviceUser(t, f, "renew@example.invalid", "old", "new")
			w, n := saveDevices(t, f, u.ID, "old", "browser")
			saveDevices(t, f, u.ID, "old", "browser") // same-session renewal, new generation
			if err := f.web.DeleteSubscriptionIfCurrent(ctx, u.ID, w); err != nil {
				t.Fatal(err)
			}
			if err := f.native.DeleteDeviceIfCurrent(ctx, n); err != nil {
				t.Fatal(err)
			}
			checkDeviceCount(t, f, u.ID, 1)
			saveDevices(t, f, u.ID, "new", "browser")
			if err := f.web.DeleteSubscription(ctx, u.ID, "browser", "old"); err != nil {
				t.Fatal(err)
			}
			if err := f.native.UnregisterDevice(ctx, u.ID, "browser", "old"); err != nil {
				t.Fatal(err)
			}
			checkDeviceCount(t, f, u.ID, 1)
			if err := f.auth.TouchSession(ctx, "new", time.Now().Add(-25*time.Hour)); err != nil {
				t.Fatal(err)
			}
			checkDeviceCount(t, f, u.ID, 0)
		})
	}
}

func TestPostgresRegistrationCannotPassCommittedLogout(t *testing.T) {
	f := deviceStores(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	u := newDeviceUser(t, f, "racing@example.invalid", "racing")
	w, n := saveDevices(t, f, u.ID, "racing", "browser")
	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	var pid int
	if err := tx.QueryRowContext(ctx, `SELECT pg_backend_pid() FROM sessions WHERE token_hash='racing' FOR UPDATE`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	doneWeb, doneNative := make(chan error, 1), make(chan error, 1)
	go func() { doneWeb <- f.web.SaveSubscription(ctx, u.ID, w) }()
	go func() { doneNative <- f.native.RegisterDevice(ctx, n) }()
	testdb.WaitBlocked(t, f.db, pid, "SELECT id FROM sessions")
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash='racing'`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := testsync.Receive(t, ctx, doneWeb); err == nil {
		t.Fatal("web registration survived concurrent logout")
	}
	if err := testsync.Receive(t, ctx, doneNative); err == nil {
		t.Fatal("native registration survived concurrent logout")
	}
	checkDeviceCount(t, f, u.ID, 0)
}

func TestPostgresRevocationWaitsForDeviceDelivery(t *testing.T) {
	for _, native := range []bool{false, true} {
		name := "web"
		if native {
			name = "native"
		}
		t.Run(name, func(t *testing.T) {
			f := deviceStores(t, true)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			u := newDeviceUser(t, f, "entered@example.invalid", "entered")
			w, n := saveDevices(t, f, u.ID, "entered", "browser")
			entered, resume := make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			callback := func() error { close(entered); testsync.Pause(ctx, resume); return ctx.Err() }
			go func() {
				if native {
					done <- f.native.WithDevice(ctx, n, callback)
				} else {
					done <- f.web.WithSubscription(ctx, u.ID, w, callback)
				}
			}()
			testsync.Receive(t, ctx, entered)
			var pid int
			if err := f.db.QueryRowContext(ctx, `SELECT pid FROM pg_stat_activity WHERE application_name=current_setting('application_name')
 AND state='idle in transaction' AND query LIKE '%registration_id%FOR SHARE%' LIMIT 1`).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			revoked := make(chan error, 1)
			go func() { revoked <- f.auth.DeleteSession(ctx, "entered") }()
			testdb.WaitBlocked(t, f.db, pid, "DELETE FROM sessions")
			close(resume)
			if err := testsync.Receive(t, ctx, done); err != nil {
				t.Fatal(err)
			}
			if err := testsync.Receive(t, ctx, revoked); err != nil {
				t.Fatal(err)
			}
			checkDeviceCount(t, f, u.ID, 0)
		})
	}
}

func TestPostgresDeviceGuardChecksExpiryAfterLockWait(t *testing.T) {
	for _, operation := range []string{"web-register", "native-register", "web-deliver", "native-deliver"} {
		for _, deadline := range []string{"expires_at", "last_seen_at"} {
			t.Run(operation+"/"+deadline, func(t *testing.T) {
				f := deviceStores(t, true)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				u := newDeviceUser(t, f, "expiry@example.invalid", "expiry")
				w, n := saveDevices(t, f, u.ID, "expiry", "browser")
				tx, err := f.db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = tx.Rollback() }()
				var pid int
				if err := tx.QueryRowContext(ctx, `SELECT pg_backend_pid() FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&pid); err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				called := false
				go func() {
					switch operation {
					case "web-register":
						done <- f.web.SaveSubscription(ctx, u.ID, w)
					case "native-register":
						done <- f.native.RegisterDevice(ctx, n)
					case "web-deliver":
						done <- f.web.WithSubscription(ctx, u.ID, w, func() error { called = true; return nil })
					case "native-deliver":
						done <- f.native.WithDevice(ctx, n, func() error { called = true; return nil })
					}
				}()
				testdb.WaitBlocked(t, f.db, pid, "SELECT auth_version FROM users")
				// The guard transaction has started. Move its deadline to the
				// present while it is blocked: transaction-start NOW() is older,
				// but the deadline has passed by the time we release the lock.
				query := `UPDATE sessions SET expires_at=clock_timestamp() WHERE token_hash='expiry'`
				if deadline == "last_seen_at" {
					query = `UPDATE sessions SET last_seen_at=clock_timestamp()-INTERVAL '24 hours' WHERE token_hash='expiry'`
				}
				if _, err := tx.ExecContext(ctx, query); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
				if err := testsync.Receive(t, ctx, done); err == nil || called {
					t.Fatalf("expired device operation authorized: callback=%v err=%v", called, err)
				}
			})
		}
	}
}
