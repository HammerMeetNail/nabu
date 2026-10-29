package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func sharedCounter(t *testing.T) (*PostgresCounter, *sql.DB) {
	t.Helper()
	db := testdb.New(t)
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return NewPostgresCounter(db), db
}

func limitedStatus(limiter *RateLimiter, ip string) (int, string) {
	handler := limiter.Middleware("/api/auth")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	r.RemoteAddr = ip + ":40000"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w.Code, w.Header().Get("Retry-After")
}

// Two instances share one database: a client's sign-in attempts count in one
// window however they are spread between the instances.
func TestSharedLimitHoldsAcrossInstances(t *testing.T) {
	counter, db := sharedCounter(t)
	ctx := context.Background()
	first, second := NewRateLimiter(3, time.Minute), NewRateLimiter(3, time.Minute)
	defer first.Stop()
	defer second.Stop()
	first.Share(counter, "auth")
	second.Share(counter, "auth")
	for i, limiter := range []*RateLimiter{first, second, first} {
		if code, _ := limitedStatus(limiter, "198.51.100.7"); code != http.StatusNoContent {
			t.Fatalf("attempt %d = %d, want 204", i+1, code)
		}
	}
	if code, retry := limitedStatus(second, "198.51.100.7"); code != http.StatusTooManyRequests || retry == "" {
		t.Fatalf("fourth attempt on the other instance = %d, Retry-After %q; want 429 with Retry-After", code, retry)
	}
	if code, _ := limitedStatus(first, "198.51.100.8"); code != http.StatusNoContent {
		t.Fatalf("another client = %d, want 204", code)
	}
	join := NewRateLimiter(3, time.Minute)
	defer join.Stop()
	join.Share(counter, "join")
	if code, _ := limitedStatus(join, "198.51.100.7"); code != http.StatusNoContent {
		t.Fatalf("the join scope shared the sign-in count: %d", code)
	}

	// The table holds the hashed client that request logs carry, never the
	// address itself.
	var raw, hashed int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FILTER (WHERE client = $1), COUNT(*) FILTER (WHERE client = $2) FROM rate_limits`,
		"198.51.100.7", hashClientIP("198.51.100.7")).Scan(&raw, &hashed); err != nil {
		t.Fatal(err)
	}
	if raw != 0 || hashed != 2 {
		t.Fatalf("rows keyed by the raw address = %d, by its hash = %d; want 0 and 2", raw, hashed)
	}

	// Once the window ends, the client starts a new one.
	if _, err := db.ExecContext(ctx, `UPDATE rate_limits SET window_end = now() - INTERVAL '1 second' WHERE scope = 'auth' AND client = $1`,
		hashClientIP("198.51.100.7")); err != nil {
		t.Fatal(err)
	}
	if code, _ := limitedStatus(second, "198.51.100.7"); code != http.StatusNoContent {
		t.Fatalf("attempt after the window ended = %d, want 204", code)
	}

	// Expire deletes ended windows and keeps the live ones.
	if _, err := db.ExecContext(ctx, `UPDATE rate_limits SET window_end = now() - INTERVAL '1 second' WHERE scope = 'join'`); err != nil {
		t.Fatal(err)
	}
	if err := counter.Expire(ctx); err != nil {
		t.Fatal(err)
	}
	var joinRows, authRows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FILTER (WHERE scope = 'join'), COUNT(*) FILTER (WHERE scope = 'auth') FROM rate_limits`).Scan(&joinRows, &authRows); err != nil {
		t.Fatal(err)
	}
	if joinRows != 0 || authRows != 2 {
		t.Fatalf("after Expire: %d join rows, %d auth rows; want 0 and 2", joinRows, authRows)
	}
}

type failingCounter struct{}

func (failingCounter) Hit(context.Context, string, string, time.Duration) (int, time.Time, error) {
	return 0, time.Time{}, errors.New("database unavailable")
}
func (failingCounter) Expire(context.Context) error { return errors.New("database unavailable") }

// Without the shared counter, the limit still holds on each instance.
func TestSharedLimitCountsInProcessWhenTheCounterFails(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)
	defer limiter.Stop()
	limiter.Share(failingCounter{}, "auth")
	for i := range 2 {
		if code, _ := limitedStatus(limiter, "198.51.100.9"); code != http.StatusNoContent {
			t.Fatalf("attempt %d = %d, want 204", i+1, code)
		}
	}
	if code, _ := limitedStatus(limiter, "198.51.100.9"); code != http.StatusTooManyRequests {
		t.Fatalf("third attempt = %d, want 429", code)
	}
}
