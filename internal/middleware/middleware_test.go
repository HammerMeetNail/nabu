package middleware

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dave/choresy/internal/auth"
)

type fakeSessionService struct {
	authenticate func(ctx context.Context, token string) (auth.User, error)
}

func (f *fakeSessionService) Authenticate(ctx context.Context, token string) (auth.User, error) {
	return f.authenticate(ctx, token)
}

func TestCSRFMiddlewareRejectsMissingHeader(t *testing.T) {
	handler := CSRF("csrf")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
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

	handler := Session(svc, "choresy_session")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	req.AddCookie(&http.Cookie{Name: "choresy_session", Value: session.ID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestSecurityHeadersMiddlewareSetsHeaders(t *testing.T) {
	handler := SecurityHeaders()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestCSRFMiddlewareUsesSecureCookieWhenForwardedHTTPS(t *testing.T) {
	handler := CSRF("csrf")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	found := false
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "csrf" {
			found = true
			if !cookie.Secure {
				t.Fatal("expected secure csrf cookie")
			}
		}
	}
	if !found {
		t.Fatal("expected csrf cookie")
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
