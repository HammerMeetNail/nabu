package handlers

import (
	"crypto/subtle"
	"net/http"

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
		_ = h.authService.Logout(r.Context(), cookie.Value)
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

	writeJSON(w, http.StatusOK, map[string]string{"status": "verification email sent"})
}

func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	_, err := h.authService.VerifyEmail(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}

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

	_ = user
	http.Redirect(w, r, h.appBaseURL, http.StatusFound)
}

func (h *AuthHandler) authResponse(user auth.User, session auth.Session) map[string]any {
	return map[string]any{
		"user":    user,
		"session": session.ID,
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

func (h *AuthHandler) getOIDCCookie(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}
