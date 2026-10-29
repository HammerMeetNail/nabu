package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/household"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/testdb"
	"github.com/HammerMeetNail/nabu/internal/testsync"
)

// Two instances share one database: a household's export on one keeps the
// other from starting a second, while other households export freely.
func TestExportGateHoldsOneExportPerHouseholdAcrossInstances(t *testing.T) {
	db := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	first, second := NewExportGate(), NewExportGate()
	first.ShareHouseholds(PostgresHouseholdLock(db))
	second.ShareHouseholds(PostgresHouseholdLock(db))
	entered := make(chan struct{}, 1)
	resume := make(chan struct{})
	slow := first.Wrap(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		testsync.Pause(r.Context(), resume)
		w.WriteHeader(200)
	})
	quick := second.Wrap(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	request := func(id int64) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/export", nil)
		return r.WithContext(middleware.WithUser(ctx, auth.User{ID: id, HouseholdID: &id}))
	}
	done := make(chan int, 1)
	go func() { w := httptest.NewRecorder(); slow(w, request(7)); done <- w.Code }()
	testsync.Receive(t, ctx, entered)

	w := httptest.NewRecorder()
	quick(w, request(7))
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("the second instance started a household's second export: %d", w.Code)
	}
	w = httptest.NewRecorder()
	quick(w, request(8))
	if w.Code != 204 {
		t.Fatalf("another household's export on the second instance = %d", w.Code)
	}
	close(resume)
	if code := testsync.Receive(t, ctx, done); code != 200 {
		t.Fatalf("first export = %d", code)
	}
	w = httptest.NewRecorder()
	quick(w, request(7))
	if w.Code != 204 {
		t.Fatalf("the household's next export = %d; its lock outlived the first", w.Code)
	}
	var held int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pg_locks WHERE locktype = 'advisory' AND granted
 AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`).Scan(&held); err != nil {
		t.Fatal(err)
	}
	if held != 0 {
		t.Fatalf("advisory locks held after every export finished: %d", held)
	}
}

// An export's lock never meets the household's delivery lock
// (household.PostgresStore.WithMember, which a removal also waits for): a push
// send doesn't turn an export away, and an export doesn't hold up a send.
func TestHouseholdExportLockLeavesDeliveryAlone(t *testing.T) {
	db := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	owner, err := auth.NewPostgresStore(db).CreateUser(ctx, "owner@example.invalid", "")
	if err != nil {
		t.Fatal(err)
	}
	households := household.NewPostgresStore(db)
	hh, err := households.CreateHousehold(ctx, "Shared", "S", owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	gate := NewExportGate()
	gate.ShareHouseholds(PostgresHouseholdLock(db))
	request := httptest.NewRequest(http.MethodGet, "/export", nil)
	request = request.WithContext(middleware.WithUser(ctx, auth.User{ID: owner.ID, HouseholdID: &hh.ID}))

	sending, sent := make(chan struct{}, 1), make(chan struct{})
	delivered := make(chan error, 1)
	go func() {
		delivered <- households.WithMember(ctx, owner.ID, hh.ID, func(string) error {
			sending <- struct{}{}
			testsync.Pause(ctx, sent)
			return nil
		})
	}()
	testsync.Receive(t, ctx, sending)
	w := httptest.NewRecorder()
	gate.Wrap(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })(w, request)
	if w.Code != 204 {
		t.Fatalf("an export during a push send = %d", w.Code)
	}
	close(sent)
	if err := testsync.Receive(t, ctx, delivered); err != nil {
		t.Fatal(err)
	}

	entered, resume := make(chan struct{}, 1), make(chan struct{})
	done := make(chan int, 1)
	go func() {
		w := httptest.NewRecorder()
		gate.Wrap(func(w http.ResponseWriter, r *http.Request) {
			entered <- struct{}{}
			testsync.Pause(r.Context(), resume)
			w.WriteHeader(200)
		})(w, request)
		done <- w.Code
	}()
	testsync.Receive(t, ctx, entered)
	sendCtx, sendCancel := context.WithTimeout(ctx, 3*time.Second)
	defer sendCancel()
	if err := households.WithMember(sendCtx, owner.ID, hh.ID, func(string) error { return nil }); err != nil {
		t.Fatalf("a push send during an export: %v", err)
	}
	close(resume)
	if code := testsync.Receive(t, ctx, done); code != 200 {
		t.Fatalf("export = %d", code)
	}
}
