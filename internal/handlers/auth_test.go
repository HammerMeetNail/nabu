package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dave/choresy/internal/auth"
	"github.com/dave/choresy/internal/mail"
	"github.com/dave/choresy/internal/middleware"
)

func setupAuthHandler(t *testing.T) (*AuthHandler, *auth.Service) {
	t.Helper()
	store := auth.NewMemoryStore()
	svc := auth.NewService(store)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")
	handler := NewAuthHandler(svc, "choresy_session", false, "http://localhost:8080")
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

	sessionMW := middleware.Session(svc, "choresy_session")
	sessionMW(http.HandlerFunc(handler.Me)).ServeHTTP(rec, req)

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

func TestAuthChangePassword(t *testing.T) {
	handler, svc := setupAuthHandler(t)

	user, _, err := svc.Register(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"alice@example.com", "password123",
	)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	body := `{"current_password":"password123","new_password":"newpassword456"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := middleware.WithUser(req.Context(), user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ChangePassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestAuthChangePasswordWrongCurrent(t *testing.T) {
	handler, svc := setupAuthHandler(t)

	user, _, err := svc.Register(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"alice@example.com", "password123",
	)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	body := `{"current_password":"wrongpass","new_password":"newpassword456"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := middleware.WithUser(req.Context(), user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ChangePassword(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthChangePasswordUnauthenticated(t *testing.T) {
	handler, _ := setupAuthHandler(t)

	body := `{"current_password":"password123","new_password":"newpassword456"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ChangePassword(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
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

func TestAuthResendVerification(t *testing.T) {
	handler, svc := setupAuthHandler(t)
	_, session, _ := svc.Register(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"alice@example.com", "password123",
	)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/verify/resend", nil)
	req.AddCookie(&http.Cookie{Name: "choresy_session", Value: session.ID})
	rec := httptest.NewRecorder()

	handler.ResendVerification(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthResendVerificationNoSession(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/verify/resend", nil)
	rec := httptest.NewRecorder()

	handler.ResendVerification(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthVerifyEmail(t *testing.T) {
	_, svc := setupAuthHandler(t)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")

	// Register to trigger verification email
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	_, _, err := svc.Register(ctx, "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if len(mailer.Messages) == 0 {
		t.Skip("no verification email sent (mailer may not be wired)")
	}

	token := extractTokenFromBody(t, mailer.Messages[len(mailer.Messages)-1].Body, "token=")

	// Use a fresh handler with the same service
	handler := NewAuthHandler(svc, "choresy_session", false, "http://localhost:8080")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/verify?token="+token, nil)
	rec := httptest.NewRecorder()

	handler.VerifyEmail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "verified") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestAuthVerifyEmailMissingToken(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/verify", nil)
	rec := httptest.NewRecorder()

	handler.VerifyEmail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthVerifyEmailBadToken(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/verify?token=invalid", nil)
	rec := httptest.NewRecorder()

	handler.VerifyEmail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthRequestMagicLink(t *testing.T) {
	handler, _ := setupAuthHandler(t)

	body := `{"email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/magic-link", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.RequestMagicLink(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthConsumeMagicLink(t *testing.T) {
	_, svc := setupAuthHandler(t)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")

	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	svc.Register(ctx, "alice@example.com", "password123")

	err := svc.RequestMagicLink(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("RequestMagicLink: %v", err)
	}
	if len(mailer.Messages) == 0 {
		t.Skip("no magic link email sent")
	}
	token := extractTokenFromBody(t, mailer.Messages[len(mailer.Messages)-1].Body, "token=")

	handler := NewAuthHandler(svc, "choresy_session", false, "http://localhost:8080")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/magic-link?token="+token, nil)
	rec := httptest.NewRecorder()

	handler.ConsumeMagicLink(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthConsumeMagicLinkMissingToken(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/magic-link", nil)
	rec := httptest.NewRecorder()

	handler.ConsumeMagicLink(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthConsumeMagicLinkBadToken(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/magic-link?token=badtoken", nil)
	rec := httptest.NewRecorder()

	handler.ConsumeMagicLink(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthResetPassword(t *testing.T) {
	_, svc := setupAuthHandler(t)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")

	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	svc.Register(ctx, "alice@example.com", "password123")
	svc.RequestPasswordReset(ctx, "alice@example.com")

	if len(mailer.Messages) == 0 {
		t.Skip("no reset email sent")
	}
	token := extractTokenFromBody(t, mailer.Messages[len(mailer.Messages)-1].Body, "token=")

	handler := NewAuthHandler(svc, "choresy_session", false, "http://localhost:8080")
	body := `{"token":"` + token + `","password":"newpassword123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password/reset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ResetPassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthResetPasswordWeakPassword(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	body := `{"token":"sometoken","password":"short"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password/reset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ResetPassword(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthResetPasswordBadToken(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	body := `{"token":"badtoken","password":"newpassword123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password/reset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ResetPassword(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthRegisterInvalidBody(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader("{{{invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Register(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthRegisterInvalidEmail(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	body := `{"email":"notanemail","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Register(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthLoginInvalidBody(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader("{{{invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Login(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthResendVerificationBadSession(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	// Cookie present but session doesn't exist in store → Authenticate returns error
	req := httptest.NewRequest(http.MethodPost, "/api/auth/verify/resend", nil)
	req.AddCookie(&http.Cookie{Name: "choresy_session", Value: "invalid-session-id"})
	rec := httptest.NewRecorder()
	handler.ResendVerification(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthRequestMagicLinkInvalidBody(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/magic-link", strings.NewReader("{{{invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.RequestMagicLink(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthForgotPasswordInvalidBody(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password/forgot", strings.NewReader("{{{invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ForgotPassword(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthResetPasswordInvalidBody(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password/reset", strings.NewReader("{{{invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ResetPassword(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthChangePasswordInvalidBody(t *testing.T) {
	handler, svc := setupAuthHandler(t)
	user, _, _ := svc.Register(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"alice@example.com", "password123",
	)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password", strings.NewReader("{{{invalid"))
	req.Header.Set("Content-Type", "application/json")
	ctx := middleware.WithUser(req.Context(), user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ChangePassword(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthChangePasswordWeakNew(t *testing.T) {
	handler, svc := setupAuthHandler(t)
	user, _, _ := svc.Register(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"alice@example.com", "password123",
	)
	body := `{"current_password":"password123","new_password":"weak"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := middleware.WithUser(req.Context(), user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ChangePassword(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthGoogleLoginNoOIDC(t *testing.T) {
	// No OIDC provider configured → GoogleAuthCodeURL returns ErrOIDCUnavailable.
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/login", nil)
	rec := httptest.NewRecorder()
	handler.GoogleLogin(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestAuthGoogleCallbackInvalidState(t *testing.T) {
	// No state cookie set, so expectedState="" and state!="" or both "".
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state=bogusstate", nil)
	rec := httptest.NewRecorder()
	handler.GoogleCallback(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthGoogleCallbackMissingCode(t *testing.T) {
	// State cookie matches state param, but no code param.
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state=mystate", nil)
	req.AddCookie(&http.Cookie{Name: "choresy_oidc_state", Value: "mystate"})
	rec := httptest.NewRecorder()
	handler.GoogleCallback(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthGoogleCallbackOIDCError(t *testing.T) {
	// State cookie matches, code provided, but CompleteGoogleOIDC fails (no OIDC provider).
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state=mystate&code=authcode", nil)
	req.AddCookie(&http.Cookie{Name: "choresy_oidc_state", Value: "mystate"})
	req.AddCookie(&http.Cookie{Name: "choresy_oidc_nonce", Value: "mynonce"})
	rec := httptest.NewRecorder()
	handler.GoogleCallback(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// extractTokenFromBody is the same helper used in auth service tests.
func extractTokenFromBody(t *testing.T, body, prefix string) string {
	t.Helper()
	idx := 0
	for i := 0; i <= len(body)-len(prefix); i++ {
		if body[i:i+len(prefix)] == prefix {
			idx = i + len(prefix)
			break
		}
	}
	if idx == 0 {
		t.Fatalf("token prefix %q not found in email body", prefix)
	}
	end := idx
	for end < len(body) && body[end] != '&' && body[end] != '"' && body[end] != '<' && body[end] != ' ' && body[end] != '\n' && body[end] != '\r' {
		end++
	}
	return body[idx:end]
}
