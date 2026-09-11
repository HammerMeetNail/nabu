package middleware

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/auth"
)

func TestCSRFMiddlewareRejectsMissingHeader(t *testing.T) {
	handler := CSRF("csrf", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFExemptsSnoozeButNotOtherRoutes(t *testing.T) {
	handler := CSRF("csrf", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	// The SW-invoked snooze route is exempt: a POST with no CSRF header passes.
	exempt := httptest.NewRequest(http.MethodPost, "/api/reminders/snooze", nil)
	recExempt := httptest.NewRecorder()
	handler.ServeHTTP(recExempt, exempt)
	if recExempt.Code != http.StatusNoContent {
		t.Fatalf("snooze without CSRF: status = %d, want %d", recExempt.Code, http.StatusNoContent)
	}

	// A different state-changing /api/ route with no CSRF header is still rejected.
	other := httptest.NewRequest(http.MethodPost, "/api/day-notes/2026-07-01", nil)
	recOther := httptest.NewRecorder()
	handler.ServeHTTP(recOther, other)
	if recOther.Code != http.StatusForbidden {
		t.Fatalf("day-notes without CSRF: status = %d, want %d", recOther.Code, http.StatusForbidden)
	}

	// The exemption must not extend to a path that merely has the snooze prefix
	// as a substring elsewhere — it's an exact-match allowlist.
	sneaky := httptest.NewRequest(http.MethodPost, "/api/reminders/snooze/evil", nil)
	recSneaky := httptest.NewRecorder()
	handler.ServeHTTP(recSneaky, sneaky)
	if recSneaky.Code != http.StatusForbidden {
		t.Fatalf("snooze-prefixed path without CSRF: status = %d, want %d", recSneaky.Code, http.StatusForbidden)
	}

	// Apple's form_post callback is a cross-site POST that can't carry our
	// header token; it is exempt (protected by the OAuth state cookie).
	apple := httptest.NewRequest(http.MethodPost, "/api/auth/apple/web/callback", nil)
	recApple := httptest.NewRecorder()
	handler.ServeHTTP(recApple, apple)
	if recApple.Code != http.StatusNoContent {
		t.Fatalf("apple callback without CSRF: status = %d, want %d", recApple.Code, http.StatusNoContent)
	}

	// But the sibling login route is not exempt.
	appleLogin := httptest.NewRequest(http.MethodPost, "/api/auth/apple/web/login", nil)
	recAppleLogin := httptest.NewRecorder()
	handler.ServeHTTP(recAppleLogin, appleLogin)
	if recAppleLogin.Code != http.StatusForbidden {
		t.Fatalf("apple login without CSRF: status = %d, want %d", recAppleLogin.Code, http.StatusForbidden)
	}
}

func TestRateLimiterBlocksRequestsAboveLimit(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute)
	handler := limiter.Middleware("/api/auth")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req1 := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req1.RemoteAddr = "127.0.0.1:1234"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusNoContent {
		t.Fatalf("first status = %d, want %d", rec1.Code, http.StatusNoContent)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req2.RemoteAddr = "127.0.0.1:1234"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", rec2.Code, http.StatusTooManyRequests)
	}
}

func TestSessionMiddlewareInjectsCurrentUser(t *testing.T) {
	svc := auth.NewService(auth.NewMemoryStore())
	user, session, err := svc.Register(context.Background(), "alice@example.com", "password12345678")
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	handler := Session(svc, "nabu_session")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		currentUser, ok := CurrentUser(r.Context())
		if !ok {
			t.Fatal("expected current user in context")
		}
		if currentUser.Email != user.Email {
			t.Fatalf("email = %q, want %q", currentUser.Email, user.Email)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "nabu_session", Value: session.ID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	for _, header := range []string{"X-Nabu-User-ID", "X-Nabu-Household-ID"} {
		mismatch := httptest.NewRequest(http.MethodPost, "/api/logs", nil)
		mismatch.AddCookie(&http.Cookie{Name: "nabu_session", Value: session.ID})
		mismatch.Header.Set(header, "999999")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, mismatch)
		if response.Code != http.StatusConflict || response.Header().Get("X-Nabu-Context-Changed") != "true" {
			t.Fatalf("%s mismatch: status=%d context_changed=%q", header, response.Code, response.Header().Get("X-Nabu-Context-Changed"))
		}
	}

}

func TestSecurityHeadersMiddlewareSetsHeaders(t *testing.T) {
	handler := SecurityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("expected Content-Security-Policy header")
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", rec.Header().Get("X-Content-Type-Options"))
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("X-Frame-Options = %q", rec.Header().Get("X-Frame-Options"))
	}
}

func TestSecurityHeadersHSTSDrivenByConfigNotForwardedProto(t *testing.T) {
	// HSTS must be emitted when the deployment is configured secure...
	secureHandler := SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	secureHandler.ServeHTTP(rec, req)
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("expected HSTS header when secure=true")
	}

	// ...but a spoofed X-Forwarded-Proto must NOT be enough to trigger it.
	insecureHandler := SecurityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("X-Forwarded-Proto", "https")
	rec2 := httptest.NewRecorder()
	insecureHandler.ServeHTTP(rec2, req2)
	if rec2.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS must not be driven by client-supplied X-Forwarded-Proto")
	}
}

func TestCSRFMiddlewareSecureCookieDrivenByConfig(t *testing.T) {
	// Secure cookie when the deployment is configured secure.
	handler := CSRF("csrf", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	found := false
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "csrf" {
			found = true
			if !cookie.Secure {
				t.Fatal("expected secure csrf cookie when secure=true")
			}
		}
	}
	if !found {
		t.Fatal("expected csrf cookie")
	}
}

func TestCSRFMiddlewareIgnoresForwardedProtoForSecureFlag(t *testing.T) {
	// A spoofed X-Forwarded-Proto must not make the cookie Secure when the
	// deployment is configured insecure (otherwise a client could downgrade or
	// upgrade the flag at will).
	handler := CSRF("csrf", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "csrf" && cookie.Secure {
			t.Fatal("csrf cookie Secure flag must not be driven by X-Forwarded-Proto")
		}
	}
}

func TestRequestLoggerEmitsStructuredLine(t *testing.T) {
	var output bytes.Buffer
	logger := log.New(&output, "", 0)
	handler := RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/logs", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := output.String()
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if !bytes.Contains([]byte(body), []byte(`"method":"POST"`)) || !bytes.Contains([]byte(body), []byte(`"path":"/api/logs"`)) || !bytes.Contains([]byte(body), []byte(`"status":201`)) {
		t.Fatalf("log body = %s", body)
	}
}

func TestRequireAuthRejectsUnauthenticated(t *testing.T) {
	handler := RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireHouseholdRejectsUnauthenticated(t *testing.T) {
	handler := RequireHousehold(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireHouseholdRejectsNoHousehold(t *testing.T) {
	handler := RequireHousehold(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	user := auth.User{ID: 1, Email: "alice@example.com", HouseholdID: nil}
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req = req.WithContext(WithUser(req.Context(), user))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRequireHouseholdPassesWithHousehold(t *testing.T) {
	handler := RequireHousehold(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	householdID := int64(5)
	user := auth.User{ID: 1, Email: "alice@example.com", HouseholdID: &householdID}
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req = req.WithContext(WithUser(req.Context(), user))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestWithUser_SetsAndRetrieves(t *testing.T) {
	householdID := int64(42)
	user := auth.User{ID: 99, Email: "bob@example.com", HouseholdID: &householdID}
	ctx := WithUser(context.Background(), user)
	got, ok := CurrentUser(ctx)
	if !ok {
		t.Fatal("expected user in context after WithUser")
	}
	if got.ID != 99 {
		t.Errorf("user ID = %d, want 99", got.ID)
	}
	if got.HouseholdID == nil || *got.HouseholdID != 42 {
		t.Errorf("household ID = %v, want 42", got.HouseholdID)
	}
}

func TestSessionMiddlewareSkipsStaticPaths(t *testing.T) {
	called := false
	svc := auth.NewService(auth.NewMemoryStore())
	handler := Session(svc, "nabu_session")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/static/js/app.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("handler should have been called for static path")
	}
	// No user should be in context for static path
	_, ok := CurrentUser(req.Context())
	if ok {
		t.Fatal("should not set user for static path")
	}
}
