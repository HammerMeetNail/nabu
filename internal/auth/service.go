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

	"github.com/HammerMeetNail/nabu/internal/audit"
	nabumail "github.com/HammerMeetNail/nabu/internal/mail"
)

var (
	ErrDuplicateEmail      = errors.New("email is already registered")
	ErrInvalidEmail        = errors.New("email must be valid")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrWeakPassword        = errors.New("password must be at least 8 characters")
	ErrPasswordTooLong     = errors.New("password must be 72 characters or fewer")
	ErrSessionNotFound     = errors.New("session not found")
	ErrUserNotFound        = errors.New("user not found")
	ErrInvalidToken        = errors.New("invalid or expired token")
	ErrOIDCUnavailable     = errors.New("google oidc is not configured")
	ErrOIDCEmailUnverified = errors.New("google account email must be verified")
	ErrAppleUnavailable    = errors.New("sign in with apple is not configured")
	ErrAppleNoEmail        = errors.New("apple did not return a verified email")
)

type MembershipResolver func(context.Context, int64) (*int64, string, error)

type Service struct {
	store              Store
	sessionDuration    time.Duration
	mailer             nabumail.Sender
	auditLogger        audit.Logger
	baseURL            string
	oidcProvider       OIDCProvider
	appleVerifier      AppleTokenVerifier
	appleWebAuth       *AppleWebAuth
	now                func() time.Time
	mailWake           chan struct{}
	membershipResolver MembershipResolver
}

func NewService(store Store) *Service {
	return &Service{
		store:           store,
		sessionDuration: 30 * 24 * time.Hour,
		mailer:          nabumail.NopSender{},
		auditLogger:     audit.NopLogger{},
		baseURL:         "http://localhost:8080",
		now:             func() time.Time { return time.Now().UTC() },
		mailWake:        make(chan struct{}, 1),
	}
}

func (s *Service) SetMailer(sender nabumail.Sender, baseURL string) {
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

func (s *Service) SetAppleVerifier(verifier AppleTokenVerifier) {
	s.appleVerifier = verifier
}

func (s *Service) SetAppleWebAuth(webAuth *AppleWebAuth) {
	s.appleWebAuth = webAuth
}

func (s *Service) SetAuditLogger(logger audit.Logger) {
	if logger != nil {
		s.auditLogger = logger
	}
}

// PostgreSQL user reads already join canonical membership. Memory mode has
// separate stores, so resolve current membership instead of trusting copies.
func (s *Service) SetMembershipResolver(resolve MembershipResolver) {
	if _, postgres := s.store.(*PostgresStore); !postgres {
		s.membershipResolver = resolve
	}
}

func (s *Service) profile(ctx context.Context, user User) (User, error) {
	if s.membershipResolver == nil {
		return user, nil
	}
	householdID, role, err := s.membershipResolver(ctx, user.ID)
	if err != nil {
		return User{}, err
	}
	user.HouseholdID, user.Role = householdID, role
	return user, nil
}

func (s *Service) SetUserHousehold(ctx context.Context, userID, householdID int64, role string) error {
	return s.store.SetUserHousehold(ctx, userID, householdID, role)
}

// GetUserByID exposes non-secret profile data to authorized internal services.
func (s *Service) GetUserByID(ctx context.Context, userID int64) (User, error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return User{}, err
	}
	return s.profile(ctx, user)
}

func (s *Service) Register(ctx context.Context, email, password string) (User, Session, error) {
	normalizedEmail, err := normalizeAndValidateEmail(email)
	if err != nil {
		return User{}, Session{}, err
	}

	if len(password) < 8 {
		return User{}, Session{}, ErrWeakPassword
	}
	if len(password) > 72 {
		return User{}, Session{}, ErrPasswordTooLong
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return User{}, Session{}, fmt.Errorf("hash password: %w", err)
	}

	return s.RegisterWithHash(ctx, normalizedEmail, passwordHash)
}

// RegisterWithHash creates a user and session using a pre-computed password
// hash, skipping the expensive bcrypt step. Useful for test setup.
func (s *Service) RegisterWithHash(ctx context.Context, normalizedEmail, passwordHash string) (User, Session, error) {
	user, session, err := s.transactionalLogin(ctx, func(tx *Service) (User, error) {
		user, err := tx.store.CreateUser(ctx, normalizedEmail, passwordHash)
		if err != nil {
			return User{}, err
		}
		if err := tx.queueVerificationEmail(ctx, user); err != nil {
			return User{}, err
		}
		return user, nil
	})
	if err != nil {
		return User{}, Session{}, err
	}
	// Account/session/token/mail commit together. SMTP failure cannot undo that
	// commit or turn a usable registration into an apparent failed signup.
	s.wakeMailOutbox()
	return user, session, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (User, Session, error) {
	normalizedEmail := normalizeEmail(email)
	user, passwordHash, err := s.store.GetUserByEmail(ctx, normalizedEmail)
	if err != nil {
		// Timing parity: an unknown email must cost the same as a real bcrypt
		// compare, otherwise login latency reveals whether an email is
		// registered. Nothing can authenticate against the dummy hash.
		_ = verifyPassword(dummyPasswordHash, password)
		s.logAudit(ctx, "auth.login_failed", map[string]string{"method": "password"})
		return User{}, Session{}, ErrInvalidCredentials
	}

	if err := verifyPassword(passwordHash, password); err != nil {
		s.logAudit(ctx, "auth.login_failed", map[string]string{"method": "password", "user_id": fmt.Sprintf("%d", user.ID)})
		return User{}, Session{}, ErrInvalidCredentials
	}

	user, err = s.profile(ctx, user)
	if err != nil {
		return User{}, Session{}, err
	}
	session, err := s.newSession(ctx, user)
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

const sessionIdleTimeout = 24 * time.Hour

func (s *Service) Authenticate(ctx context.Context, sessionToken string) (User, error) {
	if sessionToken == "" {
		return User{}, ErrSessionNotFound
	}
	tokenHash := hashToken(sessionToken)
	session, err := s.store.GetSession(ctx, tokenHash)
	if err != nil {
		return User{}, ErrSessionNotFound
	}
	now := s.now()
	if session.ExpiresAt.Before(now) {
		_ = s.store.DeleteSession(ctx, tokenHash)
		return User{}, ErrSessionNotFound
	}
	// Enforce idle timeout: reject sessions not seen in the last 24 hours.
	if now.Sub(session.LastSeenAt) > sessionIdleTimeout {
		_ = s.store.DeleteSession(ctx, tokenHash)
		return User{}, ErrSessionNotFound
	}
	// Touch the session no more than once per minute to avoid excessive writes.
	if now.Sub(session.LastSeenAt) > time.Minute {
		_ = s.store.TouchSession(ctx, tokenHash, now)
	}
	user, err := s.store.GetUserByID(ctx, session.UserID)
	if err != nil {
		return User{}, err
	}
	if user.AuthVersion != session.AuthVersion {
		return User{}, ErrSessionNotFound
	}
	user.SessionHash = tokenHash
	return s.profile(ctx, user)
}

func (s *Service) VerifyEmail(ctx context.Context, token string) (User, error) {
	user, _, err := s.VerifyEmailAndLogin(ctx, token)
	return user, err
}

func (s *Service) VerifyEmailAndLogin(ctx context.Context, token string) (User, Session, error) {
	user, session, err := s.transactionalLogin(ctx, func(tx *Service) (User, error) {
		user, err := tx.consumeEmailProof(ctx, token, "verify", false)
		if err != nil {
			return User{}, err
		}
		return tx.claimUnverifiedUser(ctx, user)
	})
	if err == nil {
		s.logAudit(ctx, "auth.email_verified", map[string]string{"user_id": fmt.Sprintf("%d", user.ID)})
	}
	return user, session, err
}

func (s *Service) ResendVerification(ctx context.Context, userID int64) error {
	err := s.store.InTransaction(ctx, func(store Store) error {
		user, err := store.GetUserByID(ctx, userID)
		if err != nil {
			return err
		}
		if user.EmailVerified {
			return nil
		}
		tx := *s
		tx.store = store
		return tx.queueVerificationEmail(ctx, user)
	})
	if err != nil {
		return err
	}
	s.wakeMailOutbox()
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

	if err := s.mailer.Send(ctx, nabumail.Message{
		To:      normalizedEmail,
		Subject: "Your Nabu magic link",
		Body:    magicLinkTemplate(s.baseURL, token),
	}); err != nil {
		return err
	}

	fields := map[string]string{"email_hash": hashEmailForAudit(normalizedEmail)}
	if userID != nil {
		fields["user_id"] = fmt.Sprintf("%d", *userID)
	}
	s.logAudit(ctx, "auth.magic_link_requested", fields)
	return nil
}

func (s *Service) ConsumeMagicLink(ctx context.Context, token string) (User, Session, error) {
	user, session, err := s.transactionalLogin(ctx, func(tx *Service) (User, error) {
		user, err := tx.consumeEmailProof(ctx, token, "magic", true)
		if err != nil {
			return User{}, err
		}
		return tx.claimUnverifiedUser(ctx, user)
	})
	if err == nil {
		s.logAudit(ctx, "auth.login_succeeded", map[string]string{"method": "magic_link", "user_id": fmt.Sprintf("%d", user.ID)})
	}
	return user, session, err
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

	if err := s.mailer.Send(ctx, nabumail.Message{
		To:      user.Email,
		Subject: "Reset your Nabu password",
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
	if len(newPassword) > 72 {
		return User{}, Session{}, ErrPasswordTooLong
	}

	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		return User{}, Session{}, fmt.Errorf("hash password: %w", err)
	}
	user, session, err := s.transactionalLogin(ctx, func(tx *Service) (User, error) {
		user, err := tx.consumeEmailProof(ctx, token, "reset", false)
		if err != nil {
			return User{}, err
		}
		return tx.replaceCredentials(ctx, user, passwordHash, true)
	})
	if err == nil {
		s.logAudit(ctx, "auth.password_reset_completed", map[string]string{"user_id": fmt.Sprintf("%d", user.ID)})
	}
	return user, session, err
}

func (s *Service) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) (User, Session, error) {
	if len(newPassword) < 8 {
		return User{}, Session{}, ErrWeakPassword
	}
	if len(newPassword) > 72 {
		return User{}, Session{}, ErrPasswordTooLong
	}

	user, passwordHash, err := s.store.GetUserByIDWithHash(ctx, userID)
	if err != nil {
		return User{}, Session{}, ErrInvalidCredentials
	}
	if passwordHash == "" {
		actor, ok := audit.ActorFromContext(ctx)
		if !ok || actor.UserID != userID || actor.AuthVersion != user.AuthVersion || !user.EmailVerified || currentPassword != "" {
			return User{}, Session{}, ErrInvalidCredentials
		}
	} else if err := verifyPassword(passwordHash, currentPassword); err != nil {
		return User{}, Session{}, ErrInvalidCredentials
	}
	newHash, err := hashPassword(newPassword)
	if err != nil {
		return User{}, Session{}, fmt.Errorf("hash password: %w", err)
	}
	updated, session, err := s.transactionalLogin(ctx, func(tx *Service) (User, error) {
		current, err := tx.store.GetUserByID(ctx, userID)
		if err != nil {
			return User{}, err
		}
		// The password comparison happens outside the transaction. Reject it if
		// email ownership or another password change won the race in the meantime.
		if current.AuthVersion != user.AuthVersion {
			return User{}, ErrInvalidCredentials
		}
		return tx.replaceCredentials(ctx, current, newHash, false)
	})
	if err == nil {
		s.logAudit(ctx, "auth.password_changed", map[string]string{"user_id": fmt.Sprintf("%d", updated.ID)})
		s.wakeMailOutbox()
	}
	return updated, session, err
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

	return s.loginWithVerifiedEmail(ctx, identity.Email, "google_oidc")
}

// AppleWebAuthCodeURL returns the appleid.apple.com authorization URL for
// the web Sign in with Apple flow, or ErrAppleUnavailable when the web flow
// is not configured.
func (s *Service) AppleWebAuthCodeURL(state, nonce string) (string, error) {
	if s.appleWebAuth == nil || s.appleVerifier == nil || !s.appleVerifier.Enabled() {
		return "", ErrAppleUnavailable
	}
	return s.appleWebAuth.AuthCodeURL(state, nonce), nil
}

// LoginWithApple verifies a native Sign in with Apple identity token and
// logs the user in, creating the account on first sign-in. Like the Google
// flow, identities are keyed by email: an Apple sign-in with the same email
// as an existing account (password or Google) logs into that account and
// marks the email verified. Apple private-relay addresses are ordinary
// working emails and get no special handling.
func (s *Service) LoginWithApple(ctx context.Context, identityToken, nonce string) (User, Session, error) {
	if s.appleVerifier == nil || !s.appleVerifier.Enabled() {
		return User{}, Session{}, ErrAppleUnavailable
	}

	identity, err := s.appleVerifier.VerifyIdentityToken(ctx, identityToken, nonce)
	if err != nil {
		s.logAudit(ctx, "auth.login_failed", map[string]string{"method": "apple"})
		return User{}, Session{}, err
	}
	if identity.Email == "" || !identity.EmailVerified {
		s.logAudit(ctx, "auth.login_failed", map[string]string{"method": "apple", "reason": "email_missing_or_unverified"})
		return User{}, Session{}, ErrAppleNoEmail
	}

	return s.loginWithVerifiedEmail(ctx, identity.Email, "apple")
}

func (s *Service) newSession(ctx context.Context, user User) (Session, error) {
	token := randomToken(32)
	tokenHash := hashToken(token)
	now := s.now()
	session, err := s.store.CreateSession(ctx, user.ID, user.AuthVersion, tokenHash, now.Add(s.sessionDuration))
	if err != nil {
		return Session{}, err
	}
	session.ID = token
	// Sync last_seen_at to the service clock so tests can override svc.now().
	_ = s.store.TouchSession(ctx, tokenHash, now)
	session.LastSeenAt = now
	return session, nil
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

// hashEmailForAudit returns a short, stable fingerprint of an email address for
// audit logs. It lets operators correlate repeated events for the same address
// (e.g. magic-link request floods) without writing raw PII to the log. Note
// that email addresses are low-entropy, so this is disclosure reduction — not a
// cryptographic guarantee against a determined attacker who can brute-force
// candidate addresses.
func hashEmailForAudit(email string) string {
	h := sha256.Sum256([]byte(normalizeEmail(email)))
	return base64.RawURLEncoding.EncodeToString(h[:])[:16]
}

func emailVerificationTemplate(baseURL, token string) string {
	link := fmt.Sprintf("%s/verify-email?token=%s", baseURL, token)
	return fmt.Sprintf(`
	<h2>Welcome to Nabu!</h2>
	<p>Click the link below to verify your email address:</p>
	<p><a href="%s">Verify Email</a></p>
	<p>Or copy this link: %s</p>
	`, link, link)
}

func magicLinkTemplate(baseURL, token string) string {
	link := fmt.Sprintf("%s/magic-login?token=%s", baseURL, token)
	return fmt.Sprintf(`
	<h2>Your Nabu magic link</h2>
	<p>Click the link below to sign in:</p>
	<p><a href="%s">Sign in to Nabu</a></p>
	<p>Or copy this link: %s</p>
	`, link, link)
}

func passwordResetTemplate(baseURL, token string) string {
	link := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)
	return fmt.Sprintf(`
	<h2>Reset your Nabu password</h2>
	<p>Click the link below to reset your password:</p>
	<p><a href="%s">Reset Password</a></p>
	<p>Or copy this link: %s</p>
	`, link, link)
}

// UserExists checks raw identity without resolving household membership. The
// memory lifecycle coordinator calls this while holding its household lock.
func (s *Service) UserExists(ctx context.Context, userID int64) error {
	_, err := s.store.GetUserByID(ctx, userID)
	return err
}
