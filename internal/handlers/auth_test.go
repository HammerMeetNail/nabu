package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dave/choresy/internal/auth"
	"github.com/dave/choresy/internal/mail"
)

func setupAuthHandler(t *testing.T) (*AuthHandler, *auth.Service) {
	t.Helper()
	store := auth.NewMemoryStore()
	svc := auth.NewService(store)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")
	handler := NewAuthHandler(svc, "choresy_session")
	return handler, svc
}

func TestAuthRegister(t *testing.T) {
	handler, _ := setupAuthHandler(t)

	body := `{"email":"alice@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Register(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	bodyStr := rec.Body.String()
	if !strings.Contains(bodyStr, `"user"`) {
		t.Fatal("expected user in response")
	}
	if !strings.Contains(bodyStr, `"session"`) {
		t.Fatal("expected session in response")
	}
}

func TestAuthRegisterDuplicate(t *testing.T) {
	handler, _ := setupAuthHandler(t)

	body := `{"email":"bob@example.com","password":"password123"}`
	makeReq := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.Register(rec, req)
		return rec
	}
	if makeReq().Code != http.StatusCreated {
		t.Fatal("first register should succeed")
	}
	rec := makeReq()
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestAuthRegisterWeakPassword(t *testing.T) {
	handler, _ := setupAuthHandler(t)

	body := `{"email":"alice@example.com","password":"123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAuthLogin(t *testing.T) {
	handler, svc := setupAuthHandler(t)

	_, _, err := svc.Register(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	body := `{"email":"alice@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestAuthLoginInvalid(t *testing.T) {
	handler, _ := setupAuthHandler(t)

	body := `{"email":"nobody@example.com","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthLogout(t *testing.T) {
	handler, svc := setupAuthHandler(t)

	_, session, _ := svc.Register(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "alice@example.com", "password123")

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "choresy_session", Value: session.ID})
	rec := httptest.NewRecorder()

	handler.Logout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	_, err := svc.Authenticate(httptest.NewRequest(http.MethodGet, "/", nil).Context(), session.ID)
	if err == nil {
		t.Fatal("session should be invalidated")
	}
}

func TestAuthMe(t *testing.T) {
	handler, svc := setupAuthHandler(t)

	_, session, _ := svc.Register(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "alice@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "choresy_session", Value: session.ID})
	rec := httptest.NewRecorder()

	handler.Me(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "alice@example.com") {
		t.Fatalf("body = %s", body)
	}
}

func TestAuthMeUnauthenticated(t *testing.T) {
	handler, _ := setupAuthHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()

	handler.Me(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"user":null`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestAuthForgotPassword(t *testing.T) {
	handler, _ := setupAuthHandler(t)

	body := `{"email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password/forgot", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ForgotPassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
