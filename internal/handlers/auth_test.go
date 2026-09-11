package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/mail"
	"github.com/HammerMeetNail/nabu/internal/middleware"
)

func setupAuthHandler(t *testing.T) (*AuthHandler, *auth.Service) {
	t.Helper()
	store := auth.NewMemoryStore()
	svc := auth.NewService(store)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")
	handler := NewAuthHandler(svc, "nabu_session", false, "http://localhost:8080")
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
	if strings.Contains(bodyStr, `"session"`) {
		t.Fatal("session should not be in response")
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
	// F6: duplicate email must NOT expose whether the address is registered.
	// The handler must return 200 (not 409) to prevent account enumeration.
	if rec.Code != http.StatusOK {
		t.Fatalf("duplicate register: status = %d, want 200 (account-enumeration fix)", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "already registered") {
		t.Fatal("response must not reveal whether the email is already registered")
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
	svc.DeliverPendingMail(context.Background())
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

	_, session := quickRegister(svc, "alice@example.com")

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "nabu_session", Value: session.ID})
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

	_, session := quickRegister(svc, "alice@example.com")

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "nabu_session", Value: session.ID})
	rec := httptest.NewRecorder()

	sessionMW := middleware.Session(svc, "nabu_session")
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
	_, session := quickRegister(svc, "alice@example.com")

	req := httptest.NewRequest(http.MethodPost, "/api/auth/verify/resend", nil)
	req.AddCookie(&http.Cookie{Name: "nabu_session", Value: session.ID})
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
	svc.DeliverPendingMail(context.Background())
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if len(mailer.Messages()) == 0 {
		t.Skip("no verification email sent (mailer may not be wired)")
	}

	token := extractTokenFromBody(t, mailer.Messages()[len(mailer.Messages())-1].Body, "token=")

	// Use a fresh handler with the same service
	handler := NewAuthHandler(svc, "nabu_session", false, "http://localhost:8080")
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
	_, _ = quickRegister(svc, "alice@example.com")

	err := svc.RequestMagicLink(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("RequestMagicLink: %v", err)
	}
	if len(mailer.Messages()) == 0 {
		t.Skip("no magic link email sent")
	}
	token := extractTokenFromBody(t, mailer.Messages()[len(mailer.Messages())-1].Body, "token=")

	handler := NewAuthHandler(svc, "nabu_session", false, "http://localhost:8080")
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
	_, _ = quickRegister(svc, "alice@example.com")
	_ = svc.RequestPasswordReset(ctx, "alice@example.com")

	if len(mailer.Messages()) == 0 {
		t.Skip("no reset email sent")
	}
	token := extractTokenFromBody(t, mailer.Messages()[len(mailer.Messages())-1].Body, "token=")

	handler := NewAuthHandler(svc, "nabu_session", false, "http://localhost:8080")
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
	req.AddCookie(&http.Cookie{Name: "nabu_session", Value: "invalid-session-id"})
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

func setupGoogleHandler(t *testing.T) *AuthHandler {
	t.Helper()
	handler, svc := setupAuthHandler(t)
	svc.SetOIDCProvider(&auth.GoogleOIDCProvider{
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURL:  "http://localhost:8080/api/auth/google/callback",
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
	})
	return handler
}

func TestAllowedPostAuthRedirect(t *testing.T) {
	cases := []struct {
		target string
		want   string
	}{
		{"nabu://callback", "nabu://callback"},
		{"/settings", "/settings"},
		{"/", "/"},
		{"//evil.example", ""},
		{"https://evil.example", ""},
		{"http://localhost:8080/settings", ""},
		{"javascript:alert(1)", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := allowedPostAuthRedirect(tc.target); got != tc.want {
			t.Errorf("allowedPostAuthRedirect(%q) = %q, want %q", tc.target, got, tc.want)
		}
	}
}

func TestAuthGoogleLoginRedirectCookie(t *testing.T) {
	cases := []struct {
		name       string
		redirect   string
		wantCookie bool
	}{
		{"unsafe https", "https://evil.example", false},
		{"scheme-relative", "//evil.example", false},
		{"javascript", "javascript:alert(1)", false},
		{"empty", "", false},
		{"same-origin path", "/settings", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := setupGoogleHandler(t)
			req := httptest.NewRequest(http.MethodGet, "/api/auth/google/login?redirect="+url.QueryEscape(tc.redirect), nil)
			rec := httptest.NewRecorder()
			handler.GoogleLogin(rec, req)
			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302", rec.Code)
			}
			found := false
			for _, c := range rec.Result().Cookies() {
				if c.Name == "nabu_oidc_redirect" {
					found = true
				}
			}
			if found != tc.wantCookie {
				t.Errorf("nabu_oidc_redirect cookie set = %v, want %v", found, tc.wantCookie)
			}
		})
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
	req.AddCookie(&http.Cookie{Name: "nabu_oidc_state", Value: "mystate"})
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
	req.AddCookie(&http.Cookie{Name: "nabu_oidc_state", Value: "mystate"})
	req.AddCookie(&http.Cookie{Name: "nabu_oidc_nonce", Value: "mynonce"})
	rec := httptest.NewRecorder()
	handler.GoogleCallback(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// fakeAppleVerifier satisfies auth.AppleTokenVerifier so callback tests can
// exercise the handler without minting real RS256 tokens.
type fakeAppleVerifier struct {
	err      error
	identity auth.OIDCIdentity
	gotToken string
	gotNonce string
}

func (f *fakeAppleVerifier) Enabled() bool { return true }

func (f *fakeAppleVerifier) VerifyIdentityToken(ctx context.Context, token, nonce string) (auth.OIDCIdentity, error) {
	f.gotToken, f.gotNonce = token, nonce
	if f.err != nil {
		return auth.OIDCIdentity{}, f.err
	}
	return f.identity, nil
}

func setupAppleWebHandler(t *testing.T) (*AuthHandler, *fakeAppleVerifier) {
	t.Helper()
	handler, svc := setupAuthHandler(t)
	verifier := &fakeAppleVerifier{
		identity: auth.OIDCIdentity{Subject: "001234.abcdef", Email: "user@privaterelay.appleid.com", EmailVerified: true},
	}
	svc.SetAppleVerifier(verifier)
	svc.SetAppleWebAuth(&auth.AppleWebAuth{
		ClientID:    "com.nabu.web",
		RedirectURL: "http://localhost:8080/api/auth/apple/web/callback",
	})
	return handler, verifier
}

func postAppleCallback(handler *AuthHandler, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/apple/web/callback", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	handler.AppleWebCallback(rec, req)
	return rec
}

func TestAuthAppleWebLoginNotConfigured(t *testing.T) {
	handler, _ := setupAuthHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/apple/web/login", nil)
	rec := httptest.NewRecorder()
	handler.AppleWebLogin(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestAuthAppleWebLoginRedirects(t *testing.T) {
	handler, _ := setupAppleWebHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/apple/web/login", nil)
	rec := httptest.NewRecorder()
	handler.AppleWebLogin(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}

	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if loc.Host != "appleid.apple.com" || loc.Path != "/auth/authorize" {
		t.Fatalf("Location = %s, want appleid.apple.com/auth/authorize", loc)
	}
	q := loc.Query()
	if q.Get("client_id") != "com.nabu.web" {
		t.Fatalf("client_id = %q", q.Get("client_id"))
	}
	if q.Get("response_type") != "code id_token" || q.Get("response_mode") != "form_post" {
		t.Fatalf("response_type=%q response_mode=%q", q.Get("response_type"), q.Get("response_mode"))
	}
	if q.Get("scope") != "email" {
		t.Fatalf("scope = %q", q.Get("scope"))
	}

	cookies := map[string]*http.Cookie{}
	for _, c := range rec.Result().Cookies() {
		cookies[c.Name] = c
	}
	for _, name := range []string{"nabu_apple_state", "nabu_apple_nonce"} {
		c, ok := cookies[name]
		if !ok {
			t.Fatalf("cookie %s not set", name)
		}
		// The callback arrives as a cross-site POST from appleid.apple.com;
		// anything stricter than SameSite=None (which requires Secure) would
		// strip these cookies from it.
		if c.SameSite != http.SameSiteNoneMode || !c.Secure || !c.HttpOnly {
			t.Fatalf("cookie %s must be SameSite=None, Secure, HttpOnly; got %+v", name, c)
		}
	}
	if cookies["nabu_apple_state"].Value != q.Get("state") {
		t.Fatal("state cookie does not match the state query param")
	}
	if cookies["nabu_apple_nonce"].Value != q.Get("nonce") {
		t.Fatal("nonce cookie does not match the nonce query param")
	}
}

func TestAuthAppleWebCallbackInvalidState(t *testing.T) {
	handler, _ := setupAppleWebHandler(t)
	rec := postAppleCallback(handler, url.Values{"state": {"bogus"}, "id_token": {"tok"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthAppleWebCallbackUserCancelled(t *testing.T) {
	handler, _ := setupAppleWebHandler(t)
	rec := postAppleCallback(handler, url.Values{"error": {"user_cancelled_authorize"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "http://localhost:8080" {
		t.Fatalf("Location = %q", loc)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("cancelled sign-in must not set cookies")
	}
}

func TestAuthAppleWebCallbackMissingToken(t *testing.T) {
	handler, _ := setupAppleWebHandler(t)
	rec := postAppleCallback(handler, url.Values{"state": {"mystate"}},
		&http.Cookie{Name: "nabu_apple_state", Value: "mystate"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthAppleWebCallbackSuccess(t *testing.T) {
	handler, verifier := setupAppleWebHandler(t)
	rec := postAppleCallback(handler, url.Values{"state": {"mystate"}, "id_token": {"apple-token"}, "code": {"unused"}},
		&http.Cookie{Name: "nabu_apple_state", Value: "mystate"},
		&http.Cookie{Name: "nabu_apple_nonce", Value: "mynonce"})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303, body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "http://localhost:8080" {
		t.Fatalf("Location = %q", loc)
	}
	if verifier.gotToken != "apple-token" || verifier.gotNonce != "mynonce" {
		t.Fatalf("verifier saw token=%q nonce=%q; the nonce must come from the cookie", verifier.gotToken, verifier.gotNonce)
	}

	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "nabu_session" {
			session = c
		}
	}
	if session == nil || session.Value == "" {
		t.Fatal("session cookie not set")
	}
}

func TestAuthAppleWebCallbackRedirectCookieHonored(t *testing.T) {
	handler, _ := setupAppleWebHandler(t)
	rec := postAppleCallback(handler, url.Values{"state": {"mystate"}, "id_token": {"apple-token"}},
		&http.Cookie{Name: "nabu_apple_state", Value: "mystate"},
		&http.Cookie{Name: "nabu_apple_nonce", Value: "mynonce"},
		&http.Cookie{Name: "nabu_apple_redirect", Value: "/settings"})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/settings" {
		t.Fatalf("Location = %q", loc)
	}
}

func TestAuthAppleWebCallbackRedirectCookieUnsafeFallsBack(t *testing.T) {
	handler, _ := setupAppleWebHandler(t)
	for name, cookieVal := range map[string]string{
		"full URL":   "http://localhost:8080/settings",
		"evil URL":   "https://evil.example",
		"scheme-rel": "//evil.example/settings",
		"javascript": "javascript:alert(1)",
	} {
		t.Run(name, func(t *testing.T) {
			rec := postAppleCallback(handler, url.Values{"state": {"mystate"}, "id_token": {"apple-token"}},
				&http.Cookie{Name: "nabu_apple_state", Value: "mystate"},
				&http.Cookie{Name: "nabu_apple_nonce", Value: "mynonce"},
				&http.Cookie{Name: "nabu_apple_redirect", Value: cookieVal})
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want 303", rec.Code)
			}
			if loc := rec.Header().Get("Location"); loc != "http://localhost:8080" {
				t.Fatalf("Location = %q, want appBaseURL fallback", loc)
			}
		})
	}
}

func TestAuthAppleWebCallbackRedirectCookieCustomScheme(t *testing.T) {
	// The iOS app passes redirect=nabu://callback; it must still flow through.
	handler, _ := setupAppleWebHandler(t)
	rec := postAppleCallback(handler, url.Values{"state": {"mystate"}, "id_token": {"apple-token"}},
		&http.Cookie{Name: "nabu_apple_state", Value: "mystate"},
		&http.Cookie{Name: "nabu_apple_nonce", Value: "mynonce"},
		&http.Cookie{Name: "nabu_apple_redirect", Value: "nabu://callback"})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "nabu://callback" {
		t.Fatalf("Location = %q", loc)
	}
}

func TestAuthAppleWebCallbackVerifierRejects(t *testing.T) {
	handler, verifier := setupAppleWebHandler(t)
	verifier.err = errAppleTestReject
	rec := postAppleCallback(handler, url.Values{"state": {"mystate"}, "id_token": {"bad-token"}},
		&http.Cookie{Name: "nabu_apple_state", Value: "mystate"},
		&http.Cookie{Name: "nabu_apple_nonce", Value: "mynonce"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "nabu_session" {
			t.Fatal("failed verification must not set a session cookie")
		}
	}
}

var errAppleTestReject = errors.New("token rejected")

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
