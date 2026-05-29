package middleware

import (
	"net/http"
	"net/http/httptest"
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

func TestRateLimiter_DifferentPathsIndependent(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	defer rl.Stop()

	if !rl.allow("1.1.1.1", "/api/auth/login") {
		t.Fatal("login should be allowed")
	}
	// Same IP, different path → separate bucket
	if !rl.allow("1.1.1.1", "/api/auth/register") {
		t.Fatal("register should be allowed (separate path bucket)")
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

func TestRateLimiter_TrustedProxy_UsesXForwardedFor(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	defer rl.Stop()
	rl.SetTrustedProxies("10.0.0.0/8")

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
	rl.SetTrustedProxies("10.0.0.0/8")

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
	rl.SetTrustedProxies("")
	rl.SetTrustedProxies("  ,  ")
}

func TestRateLimiter_SetTrustedProxies_IPv4WithoutCIDR(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	defer rl.Stop()
	rl.SetTrustedProxies("10.0.0.1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:80"
	req.Header.Set("X-Forwarded-For", "3.3.3.3")

	ip := rl.clientIP(req)
	if ip != "3.3.3.3" {
		t.Errorf("expected 3.3.3.3 (from XFF), got %q", ip)
	}
}
