package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/household"
	logsvc "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/readlimit"
	"github.com/HammerMeetNail/nabu/internal/schedule"
	"github.com/HammerMeetNail/nabu/internal/testsync"
)

func TestExportRefusesOversizedResultBeforeCSVHeaders(t *testing.T) {
	for _, rows := range []int{exportRowLimit, exportRowLimit + 1} {
		s := logsvc.NewMemoryStore()
		for i := 0; i < rows; i++ {
			if _, err := s.CreateLog(context.Background(), logsvc.ChoreLog{HouseholdID: 1, UserID: 1, ChoreID: 1, CompletedAt: time.Now(), Note: "synthetic"}); err != nil {
				t.Fatal(err)
			}
		}
		hid := int64(1)
		r := httptest.NewRequest(http.MethodGet, "/api/logs/export", nil)
		r = r.WithContext(middleware.WithUser(r.Context(), auth.User{ID: 1, HouseholdID: &hid}))
		w := httptest.NewRecorder()
		NewLogHandler(logsvc.NewService(s)).Export(w, r)
		if rows == exportRowLimit {
			if w.Code != 200 || w.Header().Get("Content-Length") == "" || strings.Count(w.Body.String(), "\n") != rows+1 {
				t.Fatal("exactly-at-limit export was truncated or refused")
			}
		} else if w.Code != 413 || w.Header().Get("Content-Disposition") != "" || strings.Contains(w.Body.String(), "synthetic") {
			t.Fatalf("oversized result became a partial success: %d %s", w.Code, w.Header())
		}
	}
}

func TestExportEncodingLimitNeverCommitsPartialFile(t *testing.T) {
	b := &exportBuffer{ctx: context.Background()}
	chunk := []byte(strings.Repeat("x", 1024))
	for b.Len() < exportByteLimit {
		if _, err := b.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.Write([]byte("last")); !errors.Is(err, readlimit.ErrExceeded) || b.Len() != exportByteLimit {
		t.Fatal("buffer exceeded its byte budget")
	}
	w := httptest.NewRecorder()
	exportError(w, readlimit.ErrExceeded)
	if w.Code != 413 || w.Header().Get("Content-Disposition") != "" {
		t.Fatal("size failure started a download")
	}
}

func TestExportHandlerByteLimitAndVisibilityLimitAreActionable(t *testing.T) {
	for _, mode := range []string{"encoded bytes", "chore metadata"} {
		t.Run(mode, func(t *testing.T) {
			logs := logsvc.NewMemoryStore()
			h := NewLogHandler(logsvc.NewService(logs))
			if mode == "encoded bytes" {
				for range 9000 {
					if _, err := logs.CreateLog(context.Background(), logsvc.ChoreLog{HouseholdID: 1, UserID: 1, ChoreID: 1, CompletedAt: time.Now(), Note: strings.Repeat("x", 2000)}); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				chores := chore.NewMemoryStore()
				for i := 0; i < exportRowLimit+1; i++ {
					if _, err := chores.CreateChore(context.Background(), chore.Chore{HouseholdID: 1, Name: fmt.Sprintf("Synthetic %d", i)}); err != nil {
						t.Fatal(err)
					}
				}
				h.WithChoreStore(chores, nil)
			}
			hid := int64(1)
			r := httptest.NewRequest(http.MethodGet, "/api/logs/export", nil)
			r = r.WithContext(middleware.WithUser(r.Context(), auth.User{ID: 1, HouseholdID: &hid}))
			w := httptest.NewRecorder()
			h.Export(w, r)
			if w.Code != 413 || w.Header().Get("Content-Disposition") != "" || !strings.Contains(w.Body.String(), "smaller date range") {
				t.Fatalf("limit became partial CSV or generic error: %d %s", w.Code, w.Header())
			}
		})
	}
}

func TestHouseholdExportUsesOneAggregateRecordBudget(t *testing.T) {
	f := newExportFixture(t)
	for i := 0; i < 5000; i++ {
		if _, err := f.handler.choreStore.CreateChore(context.Background(), chore.Chore{HouseholdID: f.household.ID, Name: fmt.Sprintf("Synthetic %d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	for range 5000 {
		if _, err := f.handler.scheduleStore.Create(context.Background(), schedule.ChoreSchedule{HouseholdID: f.household.ID, ChoreID: 1, IsActive: true}); err != nil {
			t.Fatal(err)
		}
	}
	r := withUser(httptest.NewRequest(http.MethodGet, "/api/household/data", nil), f.authService, f.ownerSession.ID)
	w := httptest.NewRecorder()
	f.handler.Data(w, r)
	if w.Code != 413 || w.Header().Get("Content-Disposition") != "" {
		t.Fatalf("aggregate limit was applied independently to collections: %d", w.Code)
	}
}

func TestExportGateBoundsConcurrencyWithoutWaiting(t *testing.T) {
	gate := NewExportGate()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entered := make(chan struct{}, 2)
	resume := make(chan struct{})
	wrapped := gate.Wrap(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		testsync.Pause(r.Context(), resume)
		w.WriteHeader(200)
	})
	request := func(id int64) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/export", nil)
		return r.WithContext(middleware.WithUser(ctx, auth.User{ID: id, HouseholdID: &id}))
	}
	done := make(chan int, 2)
	go func() { w := httptest.NewRecorder(); wrapped(w, request(1)); done <- w.Code }()
	testsync.Receive(t, ctx, entered)
	w := httptest.NewRecorder()
	wrapped(w, request(1))
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("same household occupied multiple export slots")
	}
	go func() { w := httptest.NewRecorder(); wrapped(w, request(2)); done <- w.Code }()
	testsync.Receive(t, ctx, entered)
	w = httptest.NewRecorder()
	wrapped(w, request(3))
	if w.Code != 429 {
		t.Fatal("third export bypassed capacity limit")
	}
	close(resume)
	for range 2 {
		if testsync.Receive(t, ctx, done) != 200 {
			t.Fatal("accepted export failed")
		}
	}
	// After completion the same household can retry immediately.
	w = httptest.NewRecorder()
	gate.Wrap(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })(w, request(1))
	if w.Code != 204 {
		t.Fatal("completed export retained its slot")
	}
}

func TestExportWriteDeadlineBoundsUnreadSocketThroughMiddleware(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entered, done := make(chan struct{}), make(chan struct{})
	server := httptest.NewUnstartedServer(middleware.RequestLogger(log.New(io.Discard, "", 0))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := &exportBuffer{ctx: context.Background()}
		if _, err := b.Write([]byte(strings.Repeat("x", exportByteLimit))); err != nil {
			return
		}
		writeCtx, writeCancel := context.WithTimeout(r.Context(), 150*time.Millisecond)
		defer writeCancel()
		b.ctx = writeCtx
		close(entered)
		sendExport(w, r.WithContext(writeCtx), b, "synthetic.csv")
		close(done)
	})))
	server.Start()
	defer server.Close()
	address := server.Listener.Addr().(*net.TCPAddr)
	conn, err := net.DialTCP("tcp", nil, address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetReadBuffer(1024); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	testsync.Receive(t, ctx, entered)
	testsync.Receive(t, ctx, done) // client intentionally never reads the body.
}

func TestSmallExportsKeepDeadlineThroughHTTPFinalFlush(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	closed := make(chan struct{})
	var closeOnce sync.Once
	var requests atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeCtx, writeCancel := context.WithTimeout(r.Context(), 200*time.Millisecond)
		defer writeCancel()
		b := &exportBuffer{ctx: writeCtx}
		_, _ = b.Write([]byte("date,note\n2026-09-10,synthetic\n"))
		sendExport(w, r.WithContext(writeCtx), b, "synthetic.csv")
		requests.Add(1)
	}))
	server.Config.WriteTimeout = time.Second
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closeOnce.Do(func() { close(closed) })
		}
	}
	server.Start()
	defer server.Close()
	conn, err := net.DialTCP("tcp", nil, server.Listener.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetReadBuffer(1024); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	written := make(chan struct{})
	go func() {
		_, _ = io.WriteString(conn, strings.Repeat("GET / HTTP/1.1\r\nHost: localhost\r\n\r\n", 100000))
		close(written)
	}()
	// Never read responses. Small CSVs first fit net/http's buffer; eventually
	// a final flush blocks after the handler (and export gate) has returned.
	testsync.Receive(t, ctx, closed)
	_ = conn.Close()
	testsync.Receive(t, ctx, written)
	if requests.Load() < 2 {
		t.Fatal("fixture did not exercise pipelined buffered responses")
	}
}

type exportChoreBarrier struct {
	chore.Store
	calls            int
	entered, release chan struct{}
}

func (s *exportChoreBarrier) ListChores(ctx context.Context, hid int64) ([]chore.Chore, error) {
	s.calls++
	if s.calls == 2 {
		close(s.entered)
		testsync.Pause(ctx, s.release)
	}
	return s.Store.ListChores(ctx, hid)
}

func TestLogExportRejectsMetadataReadAfterVisibilityRevocation(t *testing.T) {
	f := newExportFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	member, _ := quickRegister(f.authService, "export-member@example.com")
	if err := f.householdStore.AddMember(ctx, f.household.ID, member.ID, household.RoleMember); err != nil {
		t.Fatal(err)
	}
	chores, err := f.handler.choreStore.ListChores(ctx, f.household.ID)
	if err != nil {
		t.Fatal(err)
	}
	barrier := &exportChoreBarrier{Store: f.handler.choreStore, entered: make(chan struct{}), release: make(chan struct{})}
	h := NewLogHandler(f.handler.logService).WithChoreStore(barrier, f.householdStore)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/logs/export", nil)
	r = r.WithContext(middleware.WithUser(ctx, auth.User{ID: member.ID, HouseholdID: &f.household.ID}))
	done := make(chan struct{})
	go func() { defer close(done); h.Export(w, r) }()
	testsync.Receive(t, ctx, barrier.entered)
	changed := chores[0]
	changed.Visibility = chore.VisibilityAdmins
	changed.Name = "NEW-PRIVATE-METADATA-CANARY"
	err = f.handler.choreStore.UpdateChore(ctx, changed)
	close(barrier.release)
	testsync.Receive(t, ctx, done)
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusForbidden || w.Header().Get("Content-Disposition") != "" || strings.Contains(w.Body.String(), changed.Name) {
		t.Fatalf("revoked metadata became CSV: status=%d headers=%v", w.Code, w.Header())
	}
}

type exportMembershipFailure struct {
	household.Store
	calls, failAt int
	err           error
}

func (s *exportMembershipFailure) GetMembership(ctx context.Context, userID int64) (int64, string, error) {
	s.calls++
	if s.calls == s.failAt {
		return 0, "", s.err
	}
	return s.Store.GetMembership(ctx, userID)
}
func TestHouseholdExportOperationalAccessErrorsRemainActionable(t *testing.T) {
	for _, stage := range []int{1, 2} {
		for _, problem := range []struct {
			err    error
			status int
		}{{context.DeadlineExceeded, 504}, {errors.New("private database canary"), 500}, {household.ErrNotFound, 0}} {
			if problem.status == 0 {
				if stage == 1 {
					problem.status = 401
				} else {
					problem.status = 403
				}
			}
			f := newExportFixture(t)
			store := &exportMembershipFailure{Store: f.householdStore, failAt: stage, err: problem.err}
			f.handler.householdService = household.NewService(store, f.authService)
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/api/household/data", nil)
			r = r.WithContext(middleware.WithUser(r.Context(), auth.User{ID: f.owner.ID, HouseholdID: &f.household.ID}))
			f.handler.Data(w, r)
			if w.Code != problem.status || w.Header().Get("Content-Disposition") != "" || strings.Contains(w.Body.String(), "canary") {
				t.Fatalf("access lookup stage %d: status %d, want %d", stage, w.Code, problem.status)
			}
		}
	}
}
func TestHouseholdExportCannotChangeTheReservedHousehold(t *testing.T) {
	f := newExportFixture(t)
	w := httptest.NewRecorder()
	reserved := f.household.ID + 100
	r := httptest.NewRequest(http.MethodGet, "/api/household/data", nil)
	r = r.WithContext(middleware.WithUser(r.Context(), auth.User{ID: f.owner.ID, HouseholdID: &reserved}))
	// The auth snapshot/gate reserve A. The current active household is B.
	NewExportGate().Wrap(f.handler.Data)(w, r)
	if w.Code != http.StatusForbidden || w.Header().Get("Content-Disposition") != "" {
		t.Fatalf("export escaped reserved household: %d", w.Code)
	}
}
