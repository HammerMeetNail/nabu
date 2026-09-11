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

// Hold the actual nested authorization transaction through a paused provider.
// Both completion and cancellation are observable; the parent joins each send.
type sessionHeldPush struct {
	db      *sql.DB
	entered chan int64
}

func (p *sessionHeldPush) SendPushToUser(ctx context.Context, uid int64, _, _ string) error {
	return lifecycle.WithSQLSession(ctx, p.db, uid, fmt.Sprintf("session-%d", uid), func(*sql.Tx) error {
		p.entered <- uid
		<-ctx.Done()
		return ctx.Err()
	})
}

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
	push := &sessionHeldPush{db: deliveryDB, entered: make(chan int64, 3)}
	notificationDone := make(chan error, 2)
	for _, uid := range []int64{1, 2} {
		go func() {
			notificationDone <- household.WithMember(sendCtx, household.NewPostgresStore(deliveryDB), uid, uid, func(string) error {
				return push.SendPushToUser(sendCtx, uid, "", "")
			})
		}()
	}
	s := postgresScheduler(deliveryDB, push)
	s.now = func() time.Time { return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC) }
	done := make(chan error, 1)
	go func() { done <- s.tick(sendCtx) }()
	for range 3 {
		testsync.Receive(t, ctx, push.entered)
	}
	if got := deliveryDB.Stats().InUse; got != 7 {
		t.Fatalf("expected leader + three nested guards, in use=%d", got)
	}
	if deliveryDB.Stats().WaitCount != 0 {
		t.Fatal("reserved delivery pool already starved")
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
	t.Logf("30 HTTP/database reads while three providers held guards: max=%s HTTP pool waits=0 delivery pool waits=0", maxLatency)
	stopSends()
	for range 2 {
		if err := testsync.Receive(t, ctx, notificationDone); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled notification: %v", err)
		}
	}
	if err := testsync.Receive(t, ctx, done); err == nil {
		t.Fatal("canceled scheduler reported success")
	}
	if err := leader.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if got := deliveryDB.Stats().InUse; got != 0 {
		t.Fatalf("shutdown retained %d connections", got)
	}
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
