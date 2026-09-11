package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ─── allow() ─────────────────────────────────────────────────────────────────

func TestRateLimiter_AllowsUpToLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	defer rl.Stop()

	for i := 0; i < 3; i++ {
		if !rl.allow("1.2.3.4", "/api/auth/login") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	// 4th must be rejected
	if rl.allow("1.2.3.4", "/api/auth/login") {
		t.Fatal("4th request should be rate-limited")
	}
}

func TestRateLimiter_DifferentIPsIndependent(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	defer rl.Stop()

	if !rl.allow("1.1.1.1", "/api/auth/login") {
		t.Fatal("first request for 1.1.1.1 should be allowed")
	}
	if rl.allow("1.1.1.1", "/api/auth/login") {
		t.Fatal("second request for 1.1.1.1 should be blocked")
	}
	// Different IP should be unaffected
	if !rl.allow("2.2.2.2", "/api/auth/login") {
		t.Fatal("first request for 2.2.2.2 should be allowed")
	}
}

func TestRateLimiter_DifferentPathsShareBudget(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	defer rl.Stop()

	if !rl.allow("1.1.1.1", "/api/auth/login") {
		t.Fatal("login should be allowed")
	}
	if rl.allow("1.1.1.1", "/api/auth/register") {
		t.Fatal("different paths must share this limiter's IP budget")
	}
}

func TestRateLimiter_RawResourceIDsCannotAllocateBuckets(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	t.Cleanup(rl.Stop)
	handler := rl.Middleware("/api/")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for i := range 10000 {
		r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/nonexistent/%d", i), nil)
		r.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if i < 3 && w.Code != http.StatusNoContent {
			t.Fatalf("request %d: %d", i, w.Code)
		}
		if i >= 3 && (w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "") {
			t.Fatalf("new path bypassed aggregate budget: %d", w.Code)
		}
	}
	if len(rl.entries) != 1 {
		t.Fatalf("allocated %d buckets for one client", len(rl.entries))
	}
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	rl := NewRateLimiter(1, 50*time.Millisecond)
	defer rl.Stop()

	// Advance the clock via the now field
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rl.now = func() time.Time { return base }

	if !rl.allow("1.1.1.1", "/api/auth/login") {
		t.Fatal("first should be allowed")
	}
	if rl.allow("1.1.1.1", "/api/auth/login") {
		t.Fatal("second within window should be blocked")
	}

	// Advance past the window
	rl.now = func() time.Time { return base.Add(100 * time.Millisecond) }

	if !rl.allow("1.1.1.1", "/api/auth/login") {
		t.Fatal("should be allowed after window expires")
	}
}

// ─── Middleware() ─────────────────────────────────────────────────────────────

func TestRateLimiter_MiddlewarePassesWhenAllowed(t *testing.T) {
	rl := NewRateLimiter(10, time.Minute)
	defer rl.Stop()

	called := false
	handler := rl.Middleware("/api/auth")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "1.2.3.4:5000"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !called {
		t.Error("next handler was not called")
	}
}

func TestRateLimiter_MiddlewareBlocks429(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	defer rl.Stop()

	handler := rl.Middleware("/api/auth")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		req.RemoteAddr = "5.6.7.8:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if i == 0 && w.Code != http.StatusOK {
			t.Errorf("first request: status = %d, want 200", w.Code)
		}
		if i == 1 && w.Code != http.StatusTooManyRequests {
			t.Errorf("second request: status = %d, want 429", w.Code)
		}
	}
}

func TestRateLimiter_MiddlewareSkipsNonMatchingPaths(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	defer rl.Stop()

	callCount := 0
	handler := rl.Middleware("/api/auth")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust the rate limit on /api/auth/login
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		req.RemoteAddr = "9.9.9.9:80"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	// A different path should pass through regardless
	req := httptest.NewRequest(http.MethodGet, "/api/chores", nil)
	req.RemoteAddr = "9.9.9.9:80"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("non-rate-limited path: status = %d, want 200", w.Code)
	}
}

func TestRateLimiter_MiddlewareEmptyPrefix(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	defer rl.Stop()

	handler := rl.Middleware("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request: allowed
	req := httptest.NewRequest(http.MethodGet, "/any/path", nil)
	req.RemoteAddr = "1.2.3.4:80"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("first: status = %d, want 200", w.Code)
	}

	// Second request (same IP + path): blocked
	req2 := httptest.NewRequest(http.MethodGet, "/any/path", nil)
	req2.RemoteAddr = "1.2.3.4:80"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("second: status = %d, want 429", w2.Code)
	}
}

// ─── SetTrustedProxies / clientIP ────────────────────────────────────────────

func TestRateLimiter_NoTrustedCIDRs_IgnoresXForwardedFor(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	defer rl.Stop()

	// No trusted proxies configured → RemoteAddr is the key, XFF never read.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "8.8.8.8:80"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := rl.clientIP(req); got != "8.8.8.8" {
		t.Errorf("clientIP = %q, want %q", got, "8.8.8.8")
	}
}

func TestRateLimiter_TrustedProxy_SpoofedXFF_RealWins(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	defer rl.Stop()
	if err := rl.SetTrustedProxies("10.0.0.0/8"); err != nil {
		t.Fatal(err)
	}

	// Attacker sends X-Forwarded-For: spoofed; the edge appends the real IP.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:80"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 192.168.1.1")
	if got := rl.clientIP(req); got != "192.168.1.1" {
		t.Errorf("clientIP = %q, want %q (rightmost untrusted entry)", got, "192.168.1.1")
	}
}

func TestRateLimiter_TrustedProxy_SingleXFFEntry(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	defer rl.Stop()
	if err := rl.SetTrustedProxies("10.0.0.0/8"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:80"
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	if got := rl.clientIP(req); got != "192.168.1.1" {
		t.Errorf("clientIP = %q, want %q", got, "192.168.1.1")
	}
}

func TestRateLimiter_TrustedProxy_TwoHops_LeftmostUntrusted(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	defer rl.Stop()
	if err := rl.SetTrustedProxies("10.0.0.0/8"); err != nil {
		t.Fatal(err)
	}

	// Two trusted hops: 10.0.0.2 (inner) appended by 10.0.0.1 (our proxy).
	// The rightmost non-trusted entry is the client at 172.16.0.5.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:80"
	req.Header.Set("X-Forwarded-For", "172.16.0.5, 10.0.0.2")
	if got := rl.clientIP(req); got != "172.16.0.5" {
		t.Errorf("clientIP = %q, want %q", got, "172.16.0.5")
	}
}

func TestRateLimiter_TrustedProxy_EmptyAndMalformedXFFEntries(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	defer rl.Stop()
	if err := rl.SetTrustedProxies("10.0.0.0/8"); err != nil {
		t.Fatal(err)
	}

	// Empty and whitespace entries are skipped; all-trusted chain falls back
	// to RemoteAddr.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:80"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, , 5.6.7.8 ,   ")
	if got := rl.clientIP(req); got != "5.6.7.8" {
		t.Errorf("clientIP = %q, want %q (empty entries skipped)", got, "5.6.7.8")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req2.RemoteAddr = "10.0.0.1:80"
	req2.Header.Set("X-Forwarded-For", "10.0.0.2, 10.0.0.3")
	if got := rl.clientIP(req2); got != "10.0.0.1" {
		t.Errorf("clientIP = %q, want %q (all-trusted chain → RemoteAddr)", got, "10.0.0.1")
	}
}

func TestRateLimiter_TrustedProxy_UsesXForwardedFor(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	defer rl.Stop()
	if err := rl.SetTrustedProxies("10.0.0.0/8"); err != nil {
		t.Fatal(err)
	}

	// First request from real client 192.168.1.1 via trusted proxy
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:80"
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	if !rl.allow(rl.clientIP(req), req.URL.Path) {
		t.Fatal("first request should be allowed")
	}

	// Second request: same real client, same path → blocked
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req2.RemoteAddr = "10.0.0.1:80"
	req2.Header.Set("X-Forwarded-For", "192.168.1.1")
	if rl.allow(rl.clientIP(req2), req2.URL.Path) {
		t.Fatal("second request from same real IP should be blocked")
	}
}

func TestRateLimiter_UntrustedProxy_UsesRemoteAddr(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	defer rl.Stop()
	if err := rl.SetTrustedProxies("10.0.0.0/8"); err != nil {
		t.Fatal(err)
	}

	// Proxy not in trusted list — XFF is ignored
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "99.99.99.99:80"
	req.Header.Set("X-Forwarded-For", "1.1.1.1")

	// First: allowed (key is 99.99.99.99)
	if !rl.allow(rl.clientIP(req), req.URL.Path) {
		t.Fatal("first request should be allowed")
	}
	// Second: blocked (same remote addr, proxy XFF ignored)
	if rl.allow(rl.clientIP(req), req.URL.Path) {
		t.Fatal("second from same untrusted proxy should be blocked")
	}
}

func TestRateLimiter_SetTrustedProxies_EmptyString(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	defer rl.Stop()
	// Should not panic
	if err := rl.SetTrustedProxies(""); err != nil {
		t.Fatal(err)
	}
	if err := rl.SetTrustedProxies("  ,  "); err == nil {
		t.Fatal("malformed list accepted")
	}
}

func TestRateLimiter_SetTrustedProxies_IPv4WithoutCIDR(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	defer rl.Stop()
	if err := rl.SetTrustedProxies("10.0.0.1"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:80"
	req.Header.Set("X-Forwarded-For", "3.3.3.3")

	ip := rl.clientIP(req)
	if ip != "3.3.3.3" {
		t.Errorf("expected 3.3.3.3 (from XFF), got %q", ip)
	}
}

// ─── F8/F23: Retry-After header ───────────────────────────────────────────────

func TestRateLimiter_RetryAfterHeader(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	defer rl.Stop()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rl.now = func() time.Time { return base }

	handler := rl.Middleware("/api/auth")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request is allowed.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "5.5.5.5:80"
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first request: status = %d, want 200", rr.Code)
	}

	// Second request is rate-limited and must carry Retry-After.
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: status = %d, want 429", rr.Code)
	}
	retryAfter := rr.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("expected Retry-After header on 429 response")
	}
}

func TestRateLimiter_CardinalityIsBoundedWithoutResettingActiveClients(t *testing.T) {
	l := NewRateLimiter(1, time.Minute)
	t.Cleanup(l.Stop)
	l.SetMaxClients(2)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }
	if !l.allow("first", "/api/a") || !l.allow("second", "/api/b") {
		t.Fatal("initial clients denied")
	}
	for i := range 10000 {
		if l.allow(fmt.Sprint(i), "/api/new") {
			t.Fatal("admitted unbounded client")
		}
	}
	if l.allow("first", "/api/another") {
		t.Fatal("churn evicted active allowance")
	}
	if len(l.entries) != 2 || len(l.expiry) != 2 {
		t.Fatalf("unbounded state: %d/%d", len(l.entries), len(l.expiry))
	}
	l.mu.Lock()
	now = now.Add(time.Minute)
	l.mu.Unlock()
	if !l.allow("third", "/api/new") {
		t.Fatal("expired entries did not free capacity")
	}
	if len(l.entries) != 1 || len(l.expiry) != 1 {
		t.Fatalf("expiry did not remove old state: %d/%d", len(l.entries), len(l.expiry))
	}
}

func TestRateLimiter_StopIsIdempotentAndWaitsForCleanup(t *testing.T) {
	l := NewRateLimiter(1, time.Minute)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); l.Stop() }()
	}
	wg.Wait()
	select {
	case <-l.cleanupDone:
	default:
		t.Fatal("cleanup still running")
	}
}

func TestRateLimiter_RetryAfterUsesLimiterClock(t *testing.T) {
	l := NewRateLimiter(1, time.Minute)
	t.Cleanup(l.Stop)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }
	handler := l.Middleware("/api/")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	request := httptest.NewRequest(http.MethodGet, "/api/chores", nil)
	handler.ServeHTTP(httptest.NewRecorder(), request)
	l.mu.Lock()
	now = now.Add(14500 * time.Millisecond)
	l.mu.Unlock()
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, request)
	if denied.Header().Get("Retry-After") != "46" {
		t.Fatalf("retry delay %q", denied.Header().Get("Retry-After"))
	}
}
