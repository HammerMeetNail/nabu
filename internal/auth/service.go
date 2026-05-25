package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/dave/choresy/internal/audit"
	choresymail "github.com/dave/choresy/internal/mail"
)

var (
	ErrDuplicateEmail      = errors.New("email is already registered")
	ErrInvalidEmail        = errors.New("email must be valid")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrWeakPassword        = errors.New("password must be at least 8 characters")
	ErrSessionNotFound     = errors.New("session not found")
	ErrUserNotFound        = errors.New("user not found")
	ErrInvalidToken        = errors.New("invalid or expired token")
	ErrOIDCUnavailable     = errors.New("google oidc is not configured")
	ErrOIDCEmailUnverified = errors.New("google account email must be verified")
)

type Service struct {
	store           Store
	sessionDuration time.Duration
	mailer          choresymail.Sender
	auditLogger     audit.Logger
	baseURL         string
	oidcProvider    OIDCProvider
	now             func() time.Time
}

func NewService(store Store) *Service {
	return &Service{
		store:           store,
		sessionDuration: 30 * 24 * time.Hour,
		mailer:          choresymail.NopSender{},
		auditLogger:     audit.NopLogger{},
		baseURL:         "http://localhost:8080",
		now:             func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) SetMailer(sender choresymail.Sender, baseURL string) {
	if sender != nil {
		s.mailer = sender
	}
	if baseURL != "" {
		s.baseURL = strings.TrimRight(baseURL, "/")
	}
}

func (s *Service) SetOIDCProvider(provider OIDCProvider) {
	s.oidcProvider = provider
}

func (s *Service) SetAuditLogger(logger audit.Logger) {
	if logger != nil {
		s.auditLogger = logger
	}
}

func (s *Service) SetUserHousehold(ctx context.Context, userID, householdID int64) error {
	return s.store.SetUserHousehold(ctx, userID, householdID)
}

func (s *Service) Register(ctx context.Context, email, password string) (User, Session, error) {
	normalizedEmail, err := normalizeAndValidateEmail(email)
	if err != nil {
		return User{}, Session{}, err
	}

	if len(password) < 8 {
		return User{}, Session{}, ErrWeakPassword
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return User{}, Session{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.store.CreateUser(ctx, normalizedEmail, passwordHash)
	if err != nil {
		return User{}, Session{}, err
	}

	session, err := s.newSession(ctx, user.ID)
	if err != nil {
		return User{}, Session{}, err
	}

	if err := s.sendVerificationEmail(ctx, user); err != nil {
		return User{}, Session{}, err
	}

	return user, session, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (User, Session, error) {
	normalizedEmail := normalizeEmail(email)
	user, passwordHash, err := s.store.GetUserByEmail(ctx, normalizedEmail)
	if err != nil {
		s.logAudit(ctx, "auth.login_failed", map[string]string{"method": "password"})
		return User{}, Session{}, ErrInvalidCredentials
	}

	if err := verifyPassword(passwordHash, password); err != nil {
		s.logAudit(ctx, "auth.login_failed", map[string]string{"method": "password", "user_id": fmt.Sprintf("%d", user.ID)})
		return User{}, Session{}, ErrInvalidCredentials
	}

	session, err := s.rotatedSession(ctx, user.ID)
	if err != nil {
		return User{}, Session{}, err
	}

	s.logAudit(ctx, "auth.login_succeeded", map[string]string{"method": "password", "user_id": fmt.Sprintf("%d", user.ID)})
	return user, session, nil
}

func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	if sessionToken == "" {
		return nil
	}
	tokenHash := hashToken(sessionToken)
	session, err := s.store.GetSession(ctx, tokenHash)
	if err != nil {
		return s.store.DeleteSession(ctx, tokenHash)
	}
	if err := s.store.DeleteSession(ctx, tokenHash); err != nil {
		return err
	}
	s.logAudit(ctx, "auth.logout", map[string]string{"user_id": fmt.Sprintf("%d", session.UserID)})
	return nil
}

func (s *Service) Authenticate(ctx context.Context, sessionToken string) (User, error) {
	if sessionToken == "" {
		return User{}, ErrSessionNotFound
	}
	tokenHash := hashToken(sessionToken)
	session, err := s.store.GetSession(ctx, tokenHash)
	if err != nil {
		return User{}, ErrSessionNotFound
	}
	if session.ExpiresAt.Before(s.now()) {
		_ = s.store.DeleteSession(ctx, tokenHash)
		return User{}, ErrSessionNotFound
	}
	return s.store.GetUserByID(ctx, session.UserID)
}

func (s *Service) VerifyEmail(ctx context.Context, token string) (User, error) {
	tokenHash := hashToken(token)
	authToken, err := s.store.ConsumeAuthToken(ctx, tokenHash, "verify")
	if err != nil {
		return User{}, ErrInvalidToken
	}
	if authToken.UserID == nil {
		return User{}, ErrInvalidToken
	}
	user, err := s.store.VerifyEmail(ctx, *authToken.UserID)
	if err != nil {
		return User{}, err
	}
	s.logAudit(ctx, "auth.email_verified", map[string]string{"user_id": fmt.Sprintf("%d", user.ID)})
	return user, nil
}

func (s *Service) ResendVerification(ctx context.Context, userID int64) error {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil || user.EmailVerified {
		return nil
	}
	return s.sendVerificationEmail(ctx, user)
}

func (s *Service) sendVerificationEmail(ctx context.Context, user User) error {
	token, err := s.createToken(ctx, &user.ID, user.Email, "verify", 24*time.Hour)
	if err != nil {
		return err
	}
	if err := s.mailer.Send(ctx, choresymail.Message{
		To:      user.Email,
		Subject: "Verify your Choresy email",
		Body:    emailVerificationTemplate(s.baseURL, token),
	}); err != nil {
		return err
	}
	s.logAudit(ctx, "auth.email_verification_sent", map[string]string{"user_id": fmt.Sprintf("%d", user.ID)})
	return nil
}

func (s *Service) RequestMagicLink(ctx context.Context, email string) error {
	normalizedEmail := normalizeEmail(email)
	if normalizedEmail == "" || !strings.Contains(normalizedEmail, "@") {
		return nil
	}

	user, err := s.store.FindUserByEmail(ctx, normalizedEmail)
	var userID *int64
	if err == nil {
		userID = &user.ID
	}

	token, err := s.createToken(ctx, userID, normalizedEmail, "magic", 30*time.Minute)
	if err != nil {
		return err
	}

	if err := s.mailer.Send(ctx, choresymail.Message{
		To:      normalizedEmail,
		Subject: "Your Choresy magic link",
		Body:    magicLinkTemplate(s.baseURL, token),
	}); err != nil {
		return err
	}

	fields := map[string]string{"email": normalizedEmail}
	if userID != nil {
		fields["user_id"] = fmt.Sprintf("%d", *userID)
	}
	s.logAudit(ctx, "auth.magic_link_requested", fields)
	return nil
}

func (s *Service) ConsumeMagicLink(ctx context.Context, token string) (User, Session, error) {
	tokenHash := hashToken(token)
	authToken, err := s.store.ConsumeAuthToken(ctx, tokenHash, "magic")
	if err != nil {
		return User{}, Session{}, ErrInvalidToken
	}

	if authToken.UserID == nil {
		user, err := s.store.CreateUser(ctx, authToken.Email, "")
		if err != nil {
			return User{}, Session{}, err
		}
		user, err = s.store.VerifyEmail(ctx, user.ID)
		if err != nil {
			return User{}, Session{}, err
		}
		session, err := s.rotatedSession(ctx, user.ID)
		if err != nil {
			return User{}, Session{}, err
		}
		s.logAudit(ctx, "auth.login_succeeded", map[string]string{"method": "magic_link_signup", "user_id": fmt.Sprintf("%d", user.ID)})
		return user, session, nil
	}

	user, err := s.store.GetUserByID(ctx, *authToken.UserID)
	if err != nil {
		return User{}, Session{}, err
	}
	session, err := s.rotatedSession(ctx, user.ID)
	if err != nil {
		return User{}, Session{}, err
	}
	s.logAudit(ctx, "auth.login_succeeded", map[string]string{"method": "magic_link", "user_id": fmt.Sprintf("%d", user.ID)})
	return user, session, nil
}

func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	user, err := s.store.FindUserByEmail(ctx, normalizeEmail(email))
	if err != nil {
		return nil
	}

	token, err := s.createToken(ctx, &user.ID, user.Email, "reset", 2*time.Hour)
	if err != nil {
		return err
	}

	if err := s.mailer.Send(ctx, choresymail.Message{
		To:      user.Email,
		Subject: "Reset your Choresy password",
		Body:    passwordResetTemplate(s.baseURL, token),
	}); err != nil {
		return err
	}

	s.logAudit(ctx, "auth.password_reset_requested", map[string]string{"user_id": fmt.Sprintf("%d", user.ID)})
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) (User, Session, error) {
	if len(newPassword) < 8 {
		return User{}, Session{}, ErrWeakPassword
	}

	tokenHash := hashToken(token)
	authToken, err := s.store.ConsumeAuthToken(ctx, tokenHash, "reset")
	if err != nil {
		return User{}, Session{}, ErrInvalidToken
	}
	if authToken.UserID == nil {
		return User{}, Session{}, ErrInvalidToken
	}

	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		return User{}, Session{}, fmt.Errorf("hash password: %w", err)
	}

	if err := s.store.UpdatePassword(ctx, *authToken.UserID, passwordHash); err != nil {
		return User{}, Session{}, err
	}
	if err := s.store.DeleteUserSessions(ctx, *authToken.UserID); err != nil {
		return User{}, Session{}, err
	}

	user, err := s.store.GetUserByID(ctx, *authToken.UserID)
	if err != nil {
		return User{}, Session{}, err
	}

	session, err := s.rotatedSession(ctx, user.ID)
	if err != nil {
		return User{}, Session{}, err
	}

	s.logAudit(ctx, "auth.password_reset_completed", map[string]string{"user_id": fmt.Sprintf("%d", user.ID)})
	s.logAudit(ctx, "auth.login_succeeded", map[string]string{"method": "password_reset", "user_id": fmt.Sprintf("%d", user.ID)})
	return user, session, nil
}

func (s *Service) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) (User, Session, error) {
	if len(newPassword) < 8 {
		return User{}, Session{}, ErrWeakPassword
	}

	user, passwordHash, err := s.store.GetUserByIDWithHash(ctx, userID)
	if err != nil {
		return User{}, Session{}, ErrInvalidCredentials
	}
	if passwordHash == "" {
		return User{}, Session{}, errors.New("no password set")
	}

	if err := verifyPassword(passwordHash, currentPassword); err != nil {
		return User{}, Session{}, ErrInvalidCredentials
	}

	newHash, err := hashPassword(newPassword)
	if err != nil {
		return User{}, Session{}, fmt.Errorf("hash password: %w", err)
	}

	if err := s.store.UpdatePassword(ctx, userID, newHash); err != nil {
		return User{}, Session{}, err
	}
	if err := s.store.DeleteUserSessions(ctx, userID); err != nil {
		return User{}, Session{}, err
	}

	session, err := s.rotatedSession(ctx, user.ID)
	if err != nil {
		return User{}, Session{}, err
	}

	s.logAudit(ctx, "auth.password_changed", map[string]string{"user_id": fmt.Sprintf("%d", user.ID)})
	return user, session, nil
}

func (s *Service) GoogleAuthCodeURL(state, nonce string) (string, error) {
	if s.oidcProvider == nil || !s.oidcProvider.Enabled() {
		return "", ErrOIDCUnavailable
	}
	return s.oidcProvider.AuthCodeURL(state, nonce), nil
}

func (s *Service) CompleteGoogleOIDC(ctx context.Context, code, expectedNonce string) (User, Session, error) {
	if s.oidcProvider == nil || !s.oidcProvider.Enabled() {
		return User{}, Session{}, ErrOIDCUnavailable
	}

	identity, err := s.oidcProvider.ExchangeCode(ctx, code, expectedNonce)
	if err != nil {
		s.logAudit(ctx, "auth.login_failed", map[string]string{"method": "google_oidc"})
		return User{}, Session{}, err
	}
	if !identity.EmailVerified {
		s.logAudit(ctx, "auth.login_failed", map[string]string{"method": "google_oidc", "reason": "email_unverified"})
		return User{}, Session{}, ErrOIDCEmailUnverified
	}

	existingUser, existingErr := s.store.FindUserByEmail(ctx, identity.Email)
	switch existingErr {
	case nil:
		session, err := s.rotatedSession(ctx, existingUser.ID)
		if err != nil {
			return User{}, Session{}, err
		}
		if !existingUser.EmailVerified {
			existingUser, err = s.store.VerifyEmail(ctx, existingUser.ID)
			if err != nil {
				return User{}, Session{}, err
			}
		}
		s.logAudit(ctx, "auth.login_succeeded", map[string]string{"method": "google_oidc", "user_id": fmt.Sprintf("%d", existingUser.ID)})
		return existingUser, session, nil
	default:
		user, err := s.store.CreateUser(ctx, identity.Email, "")
		if err != nil {
			return User{}, Session{}, err
		}
		user, err = s.store.VerifyEmail(ctx, user.ID)
		if err != nil {
			return User{}, Session{}, err
		}
		session, err := s.rotatedSession(ctx, user.ID)
		if err != nil {
			return User{}, Session{}, err
		}
		s.logAudit(ctx, "auth.login_succeeded", map[string]string{"method": "google_oidc", "user_id": fmt.Sprintf("%d", user.ID)})
		return user, session, nil
	}
}

func (s *Service) newSession(ctx context.Context, userID int64) (Session, error) {
	token := randomToken(32)
	tokenHash := hashToken(token)
	session, err := s.store.CreateSession(ctx, userID, tokenHash, s.now().Add(s.sessionDuration))
	if err != nil {
		return Session{}, err
	}
	session.ID = token
	return session, nil
}

func (s *Service) rotatedSession(ctx context.Context, userID int64) (Session, error) {
	if err := s.store.DeleteUserSessions(ctx, userID); err != nil {
		return Session{}, err
	}
	return s.newSession(ctx, userID)
}

func (s *Service) createToken(ctx context.Context, userID *int64, email, kind string, ttl time.Duration) (string, error) {
	token := randomToken(24)
	tokenHash := hashToken(token)
	_, err := s.store.CreateAuthToken(ctx, userID, email, tokenHash, kind, s.now().Add(ttl))
	return token, err
}

func (s *Service) logAudit(ctx context.Context, event string, attrs map[string]string) {
	if s.auditLogger == nil {
		return
	}
	s.auditLogger.Log(ctx, event, attrs)
}

func normalizeAndValidateEmail(email string) (string, error) {
	e := normalizeEmail(email)
	if e == "" {
		return "", ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e {
		return "", ErrInvalidEmail
	}
	return e, nil
}

func normalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

func randomToken(numBytes int) string {
	buf := make([]byte, numBytes)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func emailVerificationTemplate(baseURL, token string) string {
	link := fmt.Sprintf("%s/verify-email?token=%s", baseURL, token)
	return fmt.Sprintf(`
	<h2>Welcome to Choresy!</h2>
	<p>Click the link below to verify your email address:</p>
	<p><a href="%s">Verify Email</a></p>
	<p>Or copy this link: %s</p>
	`, link, link)
}

func magicLinkTemplate(baseURL, token string) string {
	link := fmt.Sprintf("%s/magic-login?token=%s", baseURL, token)
	return fmt.Sprintf(`
	<h2>Your Choresy magic link</h2>
	<p>Click the link below to sign in:</p>
	<p><a href="%s">Sign in to Choresy</a></p>
	<p>Or copy this link: %s</p>
	`, link, link)
}

func passwordResetTemplate(baseURL, token string) string {
	link := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)
	return fmt.Sprintf(`
	<h2>Reset your Choresy password</h2>
	<p>Click the link below to reset your password:</p>
	<p><a href="%s">Reset Password</a></p>
	<p>Or copy this link: %s</p>
	`, link, link)
}
