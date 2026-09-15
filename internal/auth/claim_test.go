package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/mail"
	"github.com/HammerMeetNail/nabu/internal/testdb"
	"golang.org/x/crypto/bcrypt"
)

type verifiedTestIdentity struct{ email string }

func (p verifiedTestIdentity) Enabled() bool                     { return true }
func (p verifiedTestIdentity) AuthCodeURL(string, string) string { return "" }
func (p verifiedTestIdentity) ExchangeCode(context.Context, string, string) (OIDCIdentity, error) {
	return OIDCIdentity{Subject: "synthetic", Email: p.email, EmailVerified: true}, nil
}
func (p verifiedTestIdentity) VerifyIdentityToken(context.Context, string, string) (OIDCIdentity, error) {
	return OIDCIdentity{Subject: "synthetic", Email: p.email, EmailVerified: true}, nil
}

func claimFixture(t *testing.T, store Store) (*Service, User, Session, *mail.MemorySender) {
	t.Helper()
	svc := NewService(store)
	mailer := mail.NewMemorySender()
	svc.SetMailer(mailer, "http://localhost:8080")
	hash, err := bcrypt.GenerateFromPassword([]byte("attacker-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	u, session, err := svc.RegisterWithHash(context.Background(), "claimed@example.invalid", string(hash))
	if err != nil {
		t.Fatal(err)
	}
	deliverInitialAuthMail(t, svc, u.ID)
	if len(mailer.Messages()) != 1 {
		t.Fatal("fixture registration did not deliver exactly one verification email")
	}
	svc.SetOIDCProvider(verifiedTestIdentity{email: u.Email})
	svc.SetAppleVerifier(verifiedTestIdentity{email: u.Email})
	return svc, u, session, mailer
}

func TestVerifiedProofRevokesUnverifiedCredentials(t *testing.T) {
	checkProofRevocation(t, func(t *testing.T) Store { return NewMemoryStore() })
}

func postgresClaimStore(t *testing.T) Store {
	db := testdb.New(t)
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return NewPostgresStore(db)
}

func TestPostgresProofRevokesUnverifiedCredentials(t *testing.T) {
	checkProofRevocation(t, postgresClaimStore)
}

func checkProofRevocation(t *testing.T, newStore func(*testing.T) Store) {
	t.Helper()
	for _, method := range []string{"google", "apple", "magic", "reset", "verify"} {
		t.Run(method, func(t *testing.T) {
			svc, u, old, mailer := claimFixture(t, newStore(t))
			ctx := context.Background()
			var err error
			switch method {
			case "google":
				_, _, err = svc.CompleteGoogleOIDC(ctx, "code", "nonce")
			case "apple":
				_, _, err = svc.LoginWithApple(ctx, "identity", "nonce")
			case "magic":
				err = svc.RequestMagicLink(ctx, u.Email)
				if err == nil {
					_, _, err = svc.ConsumeMagicLink(ctx, extractToken(mailer.Messages()[len(mailer.Messages())-1].Body, "token="))
				}
			case "reset":
				err = svc.RequestPasswordReset(ctx, u.Email)
				if err == nil {
					_, _, err = svc.ResetPassword(ctx, extractToken(mailer.Messages()[len(mailer.Messages())-1].Body, "token="), "owner-password")
				}
			case "verify":
				_, err = svc.VerifyEmail(ctx, extractToken(mailer.Messages()[0].Body, "token="))
			}
			if err != nil {
				t.Fatal(err)
			}
			claimed, err := svc.GetUserByID(ctx, u.ID)
			if err != nil || !claimed.EmailVerified {
				t.Errorf("claim not verified: %+v %v", claimed, err)
			}
			if _, err := svc.Authenticate(ctx, old.ID); err == nil {
				t.Error("attacker's preclaim session still authenticates")
			}
			if _, _, err := svc.Login(ctx, u.Email, "attacker-password"); err == nil {
				t.Error("attacker's preclaim password still authenticates")
			}
		})
	}
}

type pausedCredentialRead struct {
	Store
	entered, resume chan struct{}
}

func (s *pausedCredentialRead) GetUserByEmail(ctx context.Context, email string) (User, string, error) {
	u, hash, err := s.Store.GetUserByEmail(ctx, email)
	close(s.entered)
	select {
	case <-s.resume:
	case <-ctx.Done():
		return User{}, "", ctx.Err()
	}
	return u, hash, err
}

func (s *pausedCredentialRead) GetUserByIDWithHash(ctx context.Context, id int64) (User, string, error) {
	u, hash, err := s.Store.GetUserByIDWithHash(ctx, id)
	close(s.entered)
	select {
	case <-s.resume:
	case <-ctx.Done():
		return User{}, "", ctx.Err()
	}
	return u, hash, err
}

func TestClaimRejectsCredentialOperationAlreadyInFlight(t *testing.T) {
	checkStaleCredentialOperations(t, func(t *testing.T) Store { return NewMemoryStore() })
}

func TestPostgresClaimRejectsCredentialOperationAlreadyInFlight(t *testing.T) {
	checkStaleCredentialOperations(t, postgresClaimStore)
}

func checkStaleCredentialOperations(t *testing.T, newStore func(*testing.T) Store) {
	t.Helper()
	for _, change := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		t.Cleanup(cancel)
		store := newStore(t)
		svc, u, _, _ := claimFixture(t, store)
		barrier := &pausedCredentialRead{Store: store, entered: make(chan struct{}), resume: make(chan struct{})}
		stale := NewService(barrier)
		done := make(chan error, 1)
		go func() {
			var err error
			if change {
				_, _, err = stale.ChangePassword(ctx, u.ID, "attacker-password", "attacker-changed")
			} else {
				_, _, err = stale.Login(ctx, u.Email, "attacker-password")
			}
			done <- err
		}()
		select {
		case <-barrier.entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		if _, _, err := svc.CompleteGoogleOIDC(ctx, "code", "nonce"); err != nil {
			t.Fatal(err)
		}
		close(barrier.resume)
		select {
		case err := <-done:
			if err == nil {
				t.Errorf("stale credential operation succeeded (password change=%v)", change)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		cancel()
	}
}

func TestVerifiedAccountKeepsEstablishedCredentials(t *testing.T) {
	for _, method := range []string{"google", "apple", "magic", "verify"} {
		t.Run(method, func(t *testing.T) {
			store := NewMemoryStore()
			svc, user, old, mailer := claimFixture(t, store)
			ctx := context.Background()
			if _, err := store.VerifyEmail(ctx, user.ID); err != nil {
				t.Fatal(err)
			}
			var err error
			switch method {
			case "google":
				_, _, err = svc.CompleteGoogleOIDC(ctx, "code", "nonce")
			case "apple":
				_, _, err = svc.LoginWithApple(ctx, "identity", "nonce")
			case "magic":
				if err = svc.RequestMagicLink(ctx, user.Email); err == nil {
					messages := mailer.Messages()
					_, _, err = svc.ConsumeMagicLink(ctx, extractToken(messages[len(messages)-1].Body, "token="))
				}
			case "verify":
				_, _, err = svc.VerifyEmailAndLogin(ctx, extractToken(mailer.Messages()[0].Body, "token="))
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.Authenticate(ctx, old.ID); err != nil {
				t.Fatalf("trusted session revoked: %v", err)
			}
			if _, _, err := svc.Login(ctx, user.Email, "attacker-password"); err != nil {
				t.Fatalf("trusted password revoked: %v", err)
			}
		})
	}
}

func TestClaimedAccountCanSetPasswordOnlyFromFreshSession(t *testing.T) {
	svc, before, _, _ := claimFixture(t, NewMemoryStore())
	user, _, err := svc.CompleteGoogleOIDC(context.Background(), "code", "nonce")
	if err != nil {
		t.Fatal(err)
	}
	if user.HasPassword {
		t.Fatal("claim retained password flag")
	}
	stale := audit.WithActor(context.Background(), audit.Actor{UserID: before.ID, AuthVersion: before.AuthVersion})
	if _, _, err := svc.ChangePassword(stale, user.ID, "", "attacker-new-password"); err == nil {
		t.Fatal("preclaim actor set a new password")
	}
	ctx := audit.WithActor(context.Background(), audit.Actor{UserID: user.ID, AuthVersion: user.AuthVersion})
	updated, session, err := svc.ChangePassword(ctx, user.ID, "", "owner-new-password")
	if err != nil {
		t.Fatal(err)
	}
	if !updated.HasPassword || !updated.EmailVerified {
		t.Fatal("password setup did not update profile")
	}
	if _, err := svc.Authenticate(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Login(ctx, user.Email, "owner-new-password"); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordChangeDoesNotProveEmailOwnership(t *testing.T) {
	svc, user, _, _ := claimFixture(t, NewMemoryStore())
	updated, _, err := svc.ChangePassword(context.Background(), user.ID, "attacker-password", "another-password")
	if err != nil {
		t.Fatal(err)
	}
	if updated.EmailVerified {
		t.Fatal("password possession marked email verified")
	}
}

func TestClaimInvalidatesEveryPreclaimEmailToken(t *testing.T) {
	store := NewMemoryStore()
	svc, user, _, _ := claimFixture(t, store)
	ctx := context.Background()
	for _, kind := range []string{"verify", "magic", "reset"} {
		if _, err := store.CreateAuthToken(ctx, &user.ID, user.Email, "old-"+kind, kind, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.CreateAuthToken(ctx, nil, user.Email, "old-unregistered", "magic", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.CompleteGoogleOIDC(ctx, "code", "nonce"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"verify", "magic", "reset"} {
		if _, err := store.ConsumeAuthToken(ctx, "old-"+kind, kind); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("old %s proof survived: %v", kind, err)
		}
	}
	if _, err := store.ConsumeAuthToken(ctx, "old-unregistered", "magic"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("pre-registration proof survived: %v", err)
	}
}

type blockedMailer struct{ entered chan struct{} }

func (s blockedMailer) Send(ctx context.Context, _ mail.Message) error {
	select {
	case s.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestRegistrationDoesNotWaitForSMTP(t *testing.T) {
	svc := NewService(NewMemoryStore())
	mailer := blockedMailer{entered: make(chan struct{}, 1)}
	svc.SetMailer(mailer, "http://localhost:8080")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workerDone := make(chan struct{})
	type result struct {
		user    User
		session Session
		err     error
	}
	registered := make(chan result, 1)
	go func() {
		u, s, err := svc.RegisterWithHash(ctx, "slow-smtp@example.invalid", "synthetic")
		registered <- result{u, s, err}
	}()
	select {
	case got := <-registered:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if user, err := svc.Authenticate(ctx, got.session.ID); err != nil || user.ID != got.user.ID {
			t.Fatalf("no usable registration: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("registration waited for SMTP")
	}
	// No worker could have won the claim above: an inline send in Register
	// would necessarily block on the sender and fail the preceding assertion.
	go func() { svc.RunMailOutbox(ctx); close(workerDone) }()
	select {
	case <-mailer.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("delivery worker did not pick up the committed email")
	}
	cancel()
	select {
	case <-workerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("mail worker ignored shutdown")
	}
}

type unavailableTestMailer struct{}

func (unavailableTestMailer) Send(context.Context, mail.Message) error {
	return errors.New("synthetic SMTP failure")
}

func TestRegistrationEmailFailureStillReturnsCommittedSession(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)
	svc.SetMailer(unavailableTestMailer{}, "http://localhost:8080")
	u, session, err := svc.RegisterWithHash(context.Background(), "queued@example.invalid", "test-hash")
	svc.DeliverPendingMail(context.Background())
	if err != nil {
		t.Fatalf("committed account reported as failure: %v", err)
	}
	if got, err := svc.Authenticate(context.Background(), session.ID); err != nil || got.ID != u.ID {
		t.Fatalf("registration not usable: %v", err)
	}
}
