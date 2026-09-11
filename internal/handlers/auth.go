package handlers

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/middleware"
)

type AuthHandler struct {
	authService *auth.Service
	cookieName  string
	secure      bool
	appBaseURL  string
}

func NewAuthHandler(authService *auth.Service, cookieName string, secure bool, appBaseURL string) *AuthHandler {
	return &AuthHandler{authService: authService, cookieName: cookieName, secure: secure, appBaseURL: appBaseURL}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, session, err := h.authService.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		switch err {
		case auth.ErrDuplicateEmail:
			// Return 200 with a generic message to prevent account enumeration.
			// The client should tell the user to check their email.
			// Accepted oracle (product decision, audit finding 6a): a fresh
			// email returns 201 + user JSON + Set-Cookie, which is observable
			// network traffic distinct from this 200. We keep auto-login on
			// registration for UX; the tradeoff is documented in AGENTS.md.
			writeJSON(w, http.StatusOK, map[string]string{"status": "if this email is new, check your inbox"})
		case auth.ErrInvalidEmail:
			writeError(w, http.StatusBadRequest, "invalid email")
		case auth.ErrWeakPassword:
			writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		case auth.ErrPasswordTooLong:
			writeError(w, http.StatusBadRequest, "password must be 72 characters or fewer")
		default:
			writeError(w, http.StatusInternalServerError, "registration failed")
		}
		return
	}

	h.SetSessionCookie(w, session.ID)
	writeJSON(w, http.StatusCreated, h.authResponse(user, session))
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, session, err := h.authService.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	h.SetSessionCookie(w, session.ID)
	writeJSON(w, http.StatusOK, h.authResponse(user, session))
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(h.cookieName)
	if err == nil {
		if err := h.authService.Logout(r.Context(), cookie.Value); err != nil {
			writeServerError(w, "could not sign out; please retry", err)
			return
		}
	}
	h.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"user": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(h.cookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	user, err := h.authService.Authenticate(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := h.authService.ResendVerification(r.Context(), user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resend verification")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "verification email queued"})
}

func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	_, session, err := h.authService.VerifyEmailAndLogin(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}

	h.SetSessionCookie(w, session.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "email verified"})
}

func (h *AuthHandler) RequestMagicLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	_ = h.authService.RequestMagicLink(r.Context(), req.Email)
	writeJSON(w, http.StatusOK, map[string]string{"status": "if an account exists, a magic link has been sent"})
}

func (h *AuthHandler) ConsumeMagicLink(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	user, session, err := h.authService.ConsumeMagicLink(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}

	h.SetSessionCookie(w, session.ID)
	writeJSON(w, http.StatusOK, h.authResponse(user, session))
}

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	_ = h.authService.RequestPasswordReset(r.Context(), req.Email)
	writeJSON(w, http.StatusOK, map[string]string{"status": "if an account exists, a reset link has been sent"})
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, session, err := h.authService.ResetPassword(r.Context(), req.Token, req.Password)
	if err != nil {
		switch err {
		case auth.ErrInvalidToken:
			writeError(w, http.StatusBadRequest, "invalid or expired token")
		case auth.ErrWeakPassword:
			writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		case auth.ErrPasswordTooLong:
			writeError(w, http.StatusBadRequest, "password must be 72 characters or fewer")
		default:
			writeError(w, http.StatusInternalServerError, "password reset failed")
		}
		return
	}

	h.SetSessionCookie(w, session.ID)
	writeJSON(w, http.StatusOK, h.authResponse(user, session))
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updatedUser, session, err := h.authService.ChangePassword(r.Context(), user.ID, req.CurrentPassword, req.NewPassword)
	if err != nil {
		switch err {
		case auth.ErrInvalidCredentials:
			writeError(w, http.StatusUnauthorized, "current password is incorrect")
		case auth.ErrWeakPassword:
			writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		case auth.ErrPasswordTooLong:
			writeError(w, http.StatusBadRequest, "password must be 72 characters or fewer")
		default:
			writeError(w, http.StatusInternalServerError, "password change failed")
		}
		return
	}

	h.SetSessionCookie(w, session.ID)
	writeJSON(w, http.StatusOK, h.authResponse(updatedUser, session))
}

// allowedPostAuthRedirect reports whether a post-login redirect target is
// safe: same-origin absolute paths (but not protocol-relative "//...") or
// the native app's custom scheme callback. Anything else returns "" so the
// caller falls back to the app base URL.
func allowedPostAuthRedirect(target string) string {
	if target == "nabu://callback" {
		return target
	}
	if strings.HasPrefix(target, "/") && !strings.HasPrefix(target, "//") {
		return target
	}
	return ""
}

func (h *AuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := auth.GenerateState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate state")
		return
	}
	nonce, err := auth.GenerateNonce()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate nonce")
		return
	}

	url, err := h.authService.GoogleAuthCodeURL(state, nonce)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "google oidc is not configured")
		return
	}

	h.setOIDCCookie(w, "nabu_oidc_state", state, 600)
	h.setOIDCCookie(w, "nabu_oidc_nonce", nonce, 600)

	if redirect := allowedPostAuthRedirect(r.URL.Query().Get("redirect")); redirect != "" {
		h.setOIDCCookie(w, "nabu_oidc_redirect", redirect, 600)
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func (h *AuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	expectedState := h.getOIDCCookie(r, "nabu_oidc_state")
	if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(expectedState)) != 1 {
		writeError(w, http.StatusBadRequest, "invalid state parameter")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing authorization code")
		return
	}

	expectedNonce := h.getOIDCCookie(r, "nabu_oidc_nonce")

	user, session, err := h.authService.CompleteGoogleOIDC(r.Context(), code, expectedNonce)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "google authentication failed")
		return
	}

	h.SetSessionCookie(w, session.ID)

	redirectURL := h.appBaseURL
	if stored := h.getOIDCCookie(r, "nabu_oidc_redirect"); stored != "" {
		if target := allowedPostAuthRedirect(stored); target != "" {
			redirectURL = target
		}
	}
	_ = user
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// AppleNative handles POST /api/auth/apple/native: the native app obtains an
// identity token from ASAuthorizationController and exchanges it here for a
// session. The nonce is the same value the app set on the authorization
// request; the verifier requires it to match the token's nonce claim.
func (h *AuthHandler) AppleNative(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IdentityToken string `json:"identityToken"`
		Nonce         string `json:"nonce"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.IdentityToken == "" || req.Nonce == "" {
		writeError(w, http.StatusBadRequest, "identityToken and nonce are required")
		return
	}

	user, session, err := h.authService.LoginWithApple(r.Context(), req.IdentityToken, req.Nonce)
	if err != nil {
		switch err {
		case auth.ErrAppleUnavailable:
			writeError(w, http.StatusServiceUnavailable, "sign in with apple is not configured")
		case auth.ErrAppleNoEmail:
			writeError(w, http.StatusUnauthorized, "apple account must provide a verified email")
		default:
			writeError(w, http.StatusUnauthorized, "apple authentication failed")
		}
		return
	}

	h.SetSessionCookie(w, session.ID)
	writeJSON(w, http.StatusOK, h.authResponse(user, session))
}

// AppleWebLogin handles GET /api/auth/apple/web/login: it stashes a fresh
// state + nonce in cookies and redirects to appleid.apple.com. Mirrors
// GoogleLogin, except the callback is a cross-site form POST (Apple requires
// response_mode=form_post when scopes are requested), so the cookies must be
// SameSite=None to be sent with it — see setAppleWebCookie.
func (h *AuthHandler) AppleWebLogin(w http.ResponseWriter, r *http.Request) {
	state, err := auth.GenerateState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate state")
		return
	}
	nonce, err := auth.GenerateNonce()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate nonce")
		return
	}

	url, err := h.authService.AppleWebAuthCodeURL(state, nonce)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "sign in with apple is not configured")
		return
	}

	h.setAppleWebCookie(w, "nabu_apple_state", state, 600)
	h.setAppleWebCookie(w, "nabu_apple_nonce", nonce, 600)

	if redirect := allowedPostAuthRedirect(r.URL.Query().Get("redirect")); redirect != "" {
		h.setAppleWebCookie(w, "nabu_apple_redirect", redirect, 600)
	}

	http.Redirect(w, r, url, http.StatusFound)
}

// AppleWebCallback handles POST /api/auth/apple/web/callback — Apple's
// form_post response. It is CSRF-exempt (the POST comes from
// appleid.apple.com and cannot carry our header token); the state cookie is
// the double-submit protection here. The posted identity token is verified
// exactly like the native flow, nonce included.
func (h *AuthHandler) AppleWebCallback(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid form body")
		return
	}

	// Apple posts error=user_cancelled_authorize when the user backs out;
	// no session is created, so just return to the app.
	if r.PostFormValue("error") != "" {
		http.Redirect(w, r, h.appBaseURL, http.StatusSeeOther)
		return
	}

	state := r.PostFormValue("state")
	expectedState := h.getOIDCCookie(r, "nabu_apple_state")
	if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(expectedState)) != 1 {
		writeError(w, http.StatusBadRequest, "invalid state parameter")
		return
	}

	identityToken := r.PostFormValue("id_token")
	if identityToken == "" {
		writeError(w, http.StatusBadRequest, "missing identity token")
		return
	}

	expectedNonce := h.getOIDCCookie(r, "nabu_apple_nonce")

	user, session, err := h.authService.LoginWithApple(r.Context(), identityToken, expectedNonce)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "apple authentication failed")
		return
	}
	_ = user

	h.SetSessionCookie(w, session.ID)

	redirectURL := h.appBaseURL
	if stored := h.getOIDCCookie(r, "nabu_apple_redirect"); stored != "" {
		if target := allowedPostAuthRedirect(stored); target != "" {
			redirectURL = target
		}
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (h *AuthHandler) authResponse(user auth.User, session auth.Session) map[string]any {
	return map[string]any{
		"user": user,
	}
}

func (h *AuthHandler) SetSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.secure,
		MaxAge:   30 * 24 * 60 * 60,
	})
}

func (h *AuthHandler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.secure,
		MaxAge:   -1,
	})
}

func (h *AuthHandler) setOIDCCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/api/auth/google",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.secure,
		MaxAge:   maxAge,
	})
}

// setAppleWebCookie sets the state/nonce/redirect cookies for the Apple web
// flow. Unlike the Google flow (whose callback is a same-site top-level GET,
// so SameSite=Lax works), Apple delivers the callback as a cross-site POST
// from appleid.apple.com; Lax cookies are not sent with cross-site POSTs, so
// these must be SameSite=None — and None requires Secure regardless of the
// deployment flag. In practice the web flow only runs over HTTPS anyway:
// Apple requires a registered HTTPS return URL.
func (h *AuthHandler) setAppleWebCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/api/auth/apple/web",
		HttpOnly: true,
		SameSite: http.SameSiteNoneMode,
		Secure:   true,
		MaxAge:   maxAge,
	})
}

func (h *AuthHandler) getOIDCCookie(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}
