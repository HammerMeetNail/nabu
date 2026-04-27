package auth

import (
	"context"
	"testing"

	"github.com/dave/choresy/internal/mail"
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
