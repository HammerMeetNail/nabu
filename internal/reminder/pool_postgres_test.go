package reminder

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/lifecycle"
	"github.com/HammerMeetNail/nabu/internal/testsync"
)

// Every delivery worker holds what it can at once: each push worker a
// recipient's membership guard and, nested in it, a device's session guard
// through a paused provider; the fan-out worker a membership guard while it
// writes on a second connection. The reminder tick still claims with the one
// connection left, and HTTP keeps its own pool.
func TestReservedDeliveryPoolKeepsHTTPAvailableAndReleasesLeader(t *testing.T) {
	httpDB, _ := schedulerDatabase(t, 3, 1, 1)
	httpDB.SetMaxOpenConns(2) // smallest HTTP share of the 10-connection budget.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	deliveryDB, metrics, err := database.ForkDeliveryPool(ctx, httpDB)
	if err != nil {
		t.Fatal(err)
	}
	defer deliveryDB.Close()
	if _, err := httpDB.Exec(`INSERT INTO sessions(id,user_id,token_hash,expires_at,last_seen_at) SELECT 'id-'||id,id,'session-'||id,now()+interval '1 hour',now() FROM users`); err != nil {
		t.Fatal(err)
	}
	if _, err := httpDB.Exec(`UPDATE chore_schedules SET specific_time='12:00',assigned_to_user_id=3 WHERE id=3; UPDATE chore_schedules SET is_active=false WHERE id<>3`); err != nil {
		t.Fatal(err)
	}
	leader := NewPostgresAdvisoryLock(deliveryDB, LeaderLockKey)
	if ok, err := leader.TryAcquire(ctx); err != nil || !ok {
		t.Fatalf("acquire=%v %v", ok, err)
	}
	defer func() {
		cleanup, cancelCleanup := context.WithTimeout(context.Background(), time.Second)
		defer cancelCleanup()
		if err := leader.Release(cleanup); err != nil {
			t.Errorf("release leader: %v", err)
		}
	}()
	followerDB, _, err := database.ForkDeliveryPool(ctx, httpDB)
	if err != nil {
		t.Fatal(err)
	}
	defer followerDB.Close()
	follower := NewPostgresAdvisoryLock(followerDB, LeaderLockKey)
	defer func() {
		cleanup, cancelCleanup := context.WithTimeout(context.Background(), time.Second)
		defer cancelCleanup()
		if err := follower.Release(cleanup); err != nil {
			t.Errorf("release follower: %v", err)
		}
	}()
	if ok, err := follower.TryAcquire(ctx); err != nil || ok {
		t.Fatalf("two leaders=%v %v", ok, err)
	}

	sendCtx, stopSends := context.WithCancel(ctx)
	defer stopSends()
	entered := make(chan int64, database.PushWorkers+1)
	workersDone := make(chan error, database.PushWorkers+1)
	for uid := range int64(database.PushWorkers) {
		uid++
		go func() {
			workersDone <- household.WithMember(sendCtx, household.NewPostgresStore(deliveryDB), uid, uid, func(string) error {
				return lifecycle.WithSQLSession(sendCtx, deliveryDB, uid, fmt.Sprintf("session-%d", uid), func(*sql.Tx) error {
					entered <- uid
					<-sendCtx.Done()
					return sendCtx.Err()
				})
			})
		}()
	}
	go func() {
		workersDone <- household.WithMember(sendCtx, household.NewPostgresStore(deliveryDB), 3, 3, func(string) error {
			conn, err := deliveryDB.Conn(sendCtx)
			if err != nil {
				return err
			}
			defer conn.Close()
			entered <- 3
			<-sendCtx.Done()
			return sendCtx.Err()
		})
	}()
	for range database.PushWorkers + 1 {
		testsync.Receive(t, ctx, entered)
	}
	if got := deliveryDB.Stats().InUse; got != 2*database.PushWorkers+3 {
		t.Fatalf("expected leader, push guards and a fan-out, in use=%d", got)
	}
	s := postgresScheduler(deliveryDB)
	s.now = func() time.Time { return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC) }
	var report TickReport
	s.observe = func(r TickReport) { report = r }
	if err := s.tick(ctx); err != nil || report.Reminded != 1 {
		t.Fatalf("tick with the spare connection: %v %+v", err, report)
	}
	if deliveryDB.Stats().WaitCount != 0 {
		t.Fatal("reserved delivery pool starved")
	}
	// Real HTTP requests issue database reads while all three providers wait.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var one int
		if err := httpDB.QueryRowContext(r.Context(), `SELECT 1`).Scan(&one); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	before := metrics.Snapshot().Count
	client := &http.Client{Timeout: time.Second}
	var maxLatency time.Duration
	for range 30 {
		start := time.Now()
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("HTTP read status=%d", resp.StatusCode)
		}
		maxLatency = max(maxLatency, time.Since(start))
	}
	if httpDB.Stats().WaitCount != 0 || metrics.Snapshot().Count != before {
		t.Fatal("HTTP used delivery connections or waited for provider")
	}
	t.Logf("30 HTTP/database reads while every delivery worker held its connections: max=%s HTTP pool waits=0 delivery pool waits=0", maxLatency)
	stopSends()
	for range database.PushWorkers + 1 {
		if err := testsync.Receive(t, ctx, workersDone); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled worker: %v", err)
		}
	}
	if err := leader.Release(ctx); err != nil {
		t.Fatal(err)
	}
	// A canceled guard's transaction is rolled back by database/sql's own
	// goroutine, which can return the connection just after the caller has
	// seen the cancellation. Wait for that release: a leaked connection never
	// comes back, so the deadline still catches one.
	testsync.Eventually(t, ctx, func() bool { return deliveryDB.Stats().InUse == 0 })
	if ok, err := follower.TryAcquire(ctx); err != nil || !ok {
		t.Fatalf("leader handover=%v %v", ok, err)
	}
}

func TestReminderDedupPageExcludesHistoricalBacklog(t *testing.T) {
	db, _ := schedulerDatabase(t, 1, 1, 1)
	if _, err := db.Exec(`INSERT INTO schedule_reminders(schedule_id,user_id,scheduled_date)
 SELECT 1,1,'2000-01-01'::date+n FROM generate_series(1,2000)n;
 INSERT INTO schedule_reminders(schedule_id,user_id,scheduled_date) VALUES(1,1,'2026-09-10'),(1,1,'2040-01-01')`); err != nil {
		t.Fatal(err)
	}
	page, err := NewPostgresStore(db).CandidatePage(context.Background(), candidateCursor{}, 256, time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC))
	if err != nil || len(page) != 1 || len(page[0].SentDates) != 1 || page[0].SentDates[0] != "2026-09-10" {
		t.Fatalf("unbounded dedup dates: %+v %v", page, err)
	}
}
