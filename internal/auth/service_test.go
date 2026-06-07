package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/mail"
)

func TestRegister(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")

	user, session, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if user.Email != "alice@example.com" {
		t.Fatalf("email = %q, want alice@example.com", user.Email)
	}
	if user.EmailVerified {
		t.Fatal("expected email not verified")
	}
	if session.ID == "" {
		t.Fatal("expected session ID")
	}
	if len(mailer.Messages) != 1 {
		t.Fatalf("mail messages = %d, want 1", len(mailer.Messages))
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	_, _, err := svc.Register(context.Background(), "bob@example.com", "password123")
	if err != nil {
		t.Fatalf("first register: %v", err)
	}

	_, _, err = svc.Register(context.Background(), "bob@example.com", "password456")
	if err != ErrDuplicateEmail {
		t.Fatalf("error = %v, want ErrDuplicateEmail", err)
	}
}

func TestRegisterWeakPassword(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	_, _, err := svc.Register(context.Background(), "test@example.com", "short")
	if err != ErrWeakPassword {
		t.Fatalf("error = %v, want ErrWeakPassword", err)
	}
}

func TestRegisterInvalidEmail(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	_, _, err := svc.Register(context.Background(), "not-an-email", "password123")
	if err != ErrInvalidEmail {
		t.Fatalf("error = %v, want ErrInvalidEmail", err)
	}

	_, _, err = svc.Register(context.Background(), "", "password123")
	if err != ErrInvalidEmail {
		t.Fatalf("error = %v, want ErrInvalidEmail", err)
	}
}

func TestLogin(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	user, _, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	loginUser, session, err := svc.Login(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if loginUser.ID != user.ID {
		t.Fatal("expected same user")
	}
	if session.ID == "" {
		t.Fatal("expected session ID")
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	_, _, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, _, err = svc.Login(context.Background(), "alice@example.com", "wrongpass")
	if err != ErrInvalidCredentials {
		t.Fatalf("error = %v, want ErrInvalidCredentials", err)
	}

	_, _, err = svc.Login(context.Background(), "nonexistent@example.com", "password123")
	if err != ErrInvalidCredentials {
		t.Fatalf("error = %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthenticate(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	user, session, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	authUser, err := svc.Authenticate(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if authUser.ID != user.ID {
		t.Fatal("expected same user")
	}
}

func TestAuthenticateInvalidSession(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	_, err := svc.Authenticate(context.Background(), "nonexistent")
	if err != ErrSessionNotFound {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
	}
}

func TestLogout(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	_, session, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := svc.Logout(context.Background(), session.ID); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	_, err = svc.Authenticate(context.Background(), session.ID)
	if err != ErrSessionNotFound {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
	}
}

func TestVerifyEmail(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")

	_, _, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	body := mailer.Messages[0].Body
	token := extractToken(body, "token=")
	if token == "" {
		t.Fatal("could not extract token from verification email")
	}

	user, err := svc.VerifyEmail(context.Background(), token)
	if err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}
	if !user.EmailVerified {
		t.Fatal("expected email verified")
	}
}

func TestMagicLink(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")

	_, _, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := svc.RequestMagicLink(context.Background(), "alice@example.com"); err != nil {
		t.Fatalf("RequestMagicLink: %v", err)
	}

	body := mailer.Messages[1].Body
	token := extractToken(body, "token=")

	_, session, err := svc.ConsumeMagicLink(context.Background(), token)
	if err != nil {
		t.Fatalf("ConsumeMagicLink: %v", err)
	}
	if session.ID == "" {
		t.Fatal("expected session ID")
	}
}

func TestMagicLinkNewUser(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")

	if err := svc.RequestMagicLink(context.Background(), "newuser@example.com"); err != nil {
		t.Fatalf("RequestMagicLink: %v", err)
	}

	body := mailer.Messages[0].Body
	token := extractToken(body, "token=")

	user, session, err := svc.ConsumeMagicLink(context.Background(), token)
	if err != nil {
		t.Fatalf("ConsumeMagicLink: %v", err)
	}
	if user.Email != "newuser@example.com" {
		t.Fatalf("email = %q", user.Email)
	}
	if session.ID == "" {
		t.Fatal("expected session ID")
	}
}

func TestPasswordReset(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")

	_, _, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := svc.RequestPasswordReset(context.Background(), "alice@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}

	body := mailer.Messages[1].Body
	token := extractToken(body, "token=")

	_, _, err = svc.ResetPassword(context.Background(), token, "newpassword123")
	if err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	_, _, err = svc.Login(context.Background(), "alice@example.com", "newpassword123")
	if err != nil {
		t.Fatalf("Login with new password: %v", err)
	}
}

func TestResendVerification(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")

	user, _, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	mailer.Messages = nil

	if err := svc.ResendVerification(context.Background(), user.ID); err != nil {
		t.Fatalf("ResendVerification: %v", err)
	}
	if len(mailer.Messages) != 1 {
		t.Fatalf("mail messages = %d, want 1", len(mailer.Messages))
	}
}

func extractToken(body, prefix string) string {
	idx := 0
	for i := 0; i <= len(body)-len(prefix); i++ {
		if body[i:i+len(prefix)] == prefix {
			idx = i + len(prefix)
			break
		}
	}
	if idx == 0 {
		return ""
	}
	end := idx
	for end < len(body) && body[end] != '&' && body[end] != '"' && body[end] != '<' && body[end] != ' ' && body[end] != '\n' && body[end] != '\r' {
		end++
	}
	return body[idx:end]
}

func TestEmailNormalization(t *testing.T) {
	email, err := normalizeAndValidateEmail(" Alice@Example.Com ")
	if err != nil {
		t.Fatalf("normalizeAndValidateEmail: %v", err)
	}
	if email != "alice@example.com" {
		t.Fatalf("email = %q, want alice@example.com", email)
	}
}

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := hashPassword("password123")
	if err != nil {
		t.Fatalf("hashPassword: %v", err)
	}
	if err := verifyPassword(hash, "password123"); err != nil {
		t.Fatalf("verifyPassword: %v", err)
	}
	if err := verifyPassword(hash, "wrong"); err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestChangePassword(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")

	user, _, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	updatedUser, session, err := svc.ChangePassword(context.Background(), user.ID, "password123", "newpassword456")
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if updatedUser.ID != user.ID {
		t.Fatal("expected same user")
	}
	if session.ID == "" {
		t.Fatal("expected session ID")
	}

	_, _, err = svc.Login(context.Background(), "alice@example.com", "password123")
	if err != ErrInvalidCredentials {
		t.Fatal("old password should not work")
	}
	_, _, err = svc.Login(context.Background(), "alice@example.com", "newpassword456")
	if err != nil {
		t.Fatalf("new password should work: %v", err)
	}
}

func TestChangePasswordWrongCurrent(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	user, _, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, _, err = svc.ChangePassword(context.Background(), user.ID, "wrongcurrent", "newpassword456")
	if err != ErrInvalidCredentials {
		t.Fatalf("error = %v, want ErrInvalidCredentials", err)
	}
}

func TestChangePasswordWeak(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	user, _, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, _, err = svc.ChangePassword(context.Background(), user.ID, "password123", "short")
	if err != ErrWeakPassword {
		t.Fatalf("error = %v, want ErrWeakPassword", err)
	}
}

func TestLoginPreservesOtherDeviceSession(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	_, deviceA, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, _, err = svc.Login(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Login (device B): %v", err)
	}

	_, err = svc.Authenticate(context.Background(), deviceA.ID)
	if err != nil {
		t.Fatalf("device A session should still be valid after device B login: %v", err)
	}
}

func TestPasswordChangeInvalidatesAllSessions(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")

	user, deviceA, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, oldDeviceB, err := svc.Login(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Login (device B): %v", err)
	}

	_, newSession, err := svc.ChangePassword(context.Background(), user.ID, "password123", "newpassword456")
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	_, err = svc.Authenticate(context.Background(), deviceA.ID)
	if err != ErrSessionNotFound {
		t.Fatalf("device A session should be invalid after password change: error = %v", err)
	}

	_, err = svc.Authenticate(context.Background(), oldDeviceB.ID)
	if err != ErrSessionNotFound {
		t.Fatalf("old device B session should be invalid after password change: error = %v", err)
	}

	_, err = svc.Authenticate(context.Background(), newSession.ID)
	if err != nil {
		t.Fatalf("new session (from ChangePassword) should be valid: %v", err)
	}
}

func TestSetAuditLogger(t *testing.T) {
	svc := NewService(NewMemoryStore())
	// nil logger should be a no-op
	svc.SetAuditLogger(nil)

	// NopLogger should not panic
	svc.SetAuditLogger(audit.NopLogger{})
}

func TestSetUserHousehold(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")

	user, _, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := svc.SetUserHousehold(context.Background(), user.ID, 42, "owner"); err != nil {
		t.Fatalf("SetUserHousehold: %v", err)
	}

	// Verify via Authenticate
	// Re-login to get fresh session
	_, session, err := svc.Login(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	updated, err := svc.Authenticate(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if updated.HouseholdID == nil || *updated.HouseholdID != 42 {
		t.Errorf("HouseholdID = %v, want 42", updated.HouseholdID)
	}
	if updated.Role != "owner" {
		t.Errorf("Role = %q, want owner", updated.Role)
	}
}

func TestSetUserHousehold_NotFound(t *testing.T) {
	svc := NewService(NewMemoryStore())
	err := svc.SetUserHousehold(context.Background(), 9999, 1, "owner")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestLogoutUnknownSession(t *testing.T) {
	svc := NewService(NewMemoryStore())
	// Logout with an unknown token should be a no-op (no error).
	if err := svc.Logout(context.Background(), "unknown-token"); err != nil {
		t.Fatalf("Logout with unknown token: %v", err)
	}
}

func TestAuthenticateExpiredSession(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	_, session, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Wind the clock forward past session expiry.
	svc.now = func() time.Time { return time.Now().UTC().Add(365 * 24 * time.Hour) }

	_, err = svc.Authenticate(context.Background(), session.ID)
	if err != ErrSessionNotFound {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
	}
}

func TestRequestPasswordResetUnknownEmail(t *testing.T) {
	svc := NewService(NewMemoryStore())
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")

	// Unknown email should be silent (no error, no email sent).
	if err := svc.RequestPasswordReset(context.Background(), "nobody@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset unknown email: %v", err)
	}
	if len(mailer.Messages) != 0 {
		t.Fatalf("expected no emails, got %d", len(mailer.Messages))
	}
}

func TestResendVerificationAlreadyVerified(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")

	user, _, err := svc.Register(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Verify email first.
	body := mailer.Messages[0].Body
	token := extractToken(body, "token=")
	if _, err := svc.VerifyEmail(context.Background(), token); err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}
	mailer.Messages = nil

	// ResendVerification for already-verified user should be a no-op.
	if err := svc.ResendVerification(context.Background(), user.ID); err != nil {
		t.Fatalf("ResendVerification already verified: %v", err)
	}
	if len(mailer.Messages) != 0 {
		t.Fatalf("expected no emails sent, got %d", len(mailer.Messages))
	}
}

// ─── F7: Password max length ──────────────────────────────────────────────────

func TestRegisterPasswordTooLong(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	longPassword := strings.Repeat("a", 73)
	_, _, err := svc.Register(context.Background(), "a@example.com", longPassword)
	if err != ErrPasswordTooLong {
		t.Fatalf("Register with 73-char password: error = %v, want ErrPasswordTooLong", err)
	}
}

func TestRegisterPasswordAtLimit(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	password := strings.Repeat("a", 72)
	_, _, err := svc.Register(context.Background(), "a@example.com", password)
	if err != nil {
		t.Fatalf("Register with exactly 72-char password should succeed: %v", err)
	}
}

func TestResetPasswordTooLong(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	longPassword := strings.Repeat("a", 73)
	_, _, err := svc.ResetPassword(context.Background(), "sometoken", longPassword)
	if err != ErrPasswordTooLong {
		t.Fatalf("ResetPassword with 73-char password: error = %v, want ErrPasswordTooLong", err)
	}
}

func TestChangePasswordTooLong(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	longPassword := strings.Repeat("a", 73)
	_, _, err := svc.ChangePassword(context.Background(), 1, "current", longPassword)
	if err != ErrPasswordTooLong {
		t.Fatalf("ChangePassword with 73-char password: error = %v, want ErrPasswordTooLong", err)
	}
}

// ─── F10: Session idle timeout ────────────────────────────────────────────────

func TestAuthenticateIdleTimeout(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return base }

	_, session, err := svc.Register(context.Background(), "idle@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Advance past the idle timeout — session should be rejected.
	svc.now = func() time.Time { return base.Add(25 * time.Hour) }
	_, err = svc.Authenticate(context.Background(), session.ID)
	if err != ErrSessionNotFound {
		t.Fatalf("Authenticate after idle timeout: error = %v, want ErrSessionNotFound", err)
	}
}

func TestAuthenticateActiveSessionNotExpired(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return base }

	_, session, err := svc.Register(context.Background(), "active@example.com", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Advance to just under the idle timeout — session should still be valid.
	svc.now = func() time.Time { return base.Add(23 * time.Hour) }
	user, err := svc.Authenticate(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Authenticate within idle window: %v", err)
	}
	if user.Email != "active@example.com" {
		t.Fatalf("email = %q", user.Email)
	}
}
