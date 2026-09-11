package handlers

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	Health(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/json; charset=utf-8", contentType)
	}
}

func TestReadinessFailureIsSanitizedAndRecovers(t *testing.T) {
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(old) })
	var fail error
	h := Readiness(func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > ReadinessTimeout {
			t.Error("probe has no bounded deadline")
		}
		return fail
	})
	for _, code := range []int{200, 503, 200} {
		fail = nil
		if code == 503 {
			fail = errors.New("postgres://secret@example.invalid/PRIVATE-DATA")
		}
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
		if w.Code != code {
			t.Fatalf("ready status = %d, want %d", w.Code, code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("readiness can be cached")
		}
		if strings.Contains(w.Body.String(), "PRIVATE-DATA") {
			t.Fatal("leaked private dependency error")
		}
		if code == 503 && (w.Header().Get("X-Request-ID") == "" || w.Header().Get("Retry-After") != "1") {
			t.Fatal("missing recovery/correlation headers")
		}
	}
	if strings.Contains(logs.String(), "PRIVATE-DATA") || !strings.Contains(logs.String(), "error_class=internal") {
		t.Fatal("unsafe or missing readiness diagnostic")
	}
}

func TestReadinessPostgresPoolExhaustionRecoveryAndClosedDatabase(t *testing.T) {
	db := testdb.New(t)
	db.SetMaxOpenConns(1)
	h := Readiness(db.PingContext)
	request := func(want int) {
		t.Helper()
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
		if w.Code != want {
			t.Fatalf("ready status=%d, want %d", w.Code, want)
		}
	}
	request(200)
	held, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	start := time.Now()
	request(503)
	if time.Since(start) > 3*ReadinessTimeout {
		t.Fatal("exhausted pool exceeded readiness bound")
	}
	if db.Stats().WaitCount < 1 {
		t.Fatal("probe did not exercise pool wait")
	}
	if err := held.Close(); err != nil {
		t.Fatal(err)
	}
	request(200)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	request(503)
	w := httptest.NewRecorder()
	Health(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != 200 {
		t.Fatal("database failure removed liveness")
	}
}

func TestReadinessRespectsRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	Readiness(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })(w, httptest.NewRequest(http.MethodGet, "/ready", nil).WithContext(ctx))
	if w.Code != 503 {
		t.Fatalf("canceled request status = %d", w.Code)
	}
}

func TestReady(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	Ready(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body == "" {
		t.Fatal("expected response body")
	}
}
