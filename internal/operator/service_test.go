package operator

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/database"
	"github.com/HammerMeetNail/nabu/internal/testdb"
)

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestPlatformOwnerReportingAndKeyLifecycle(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var ownerID, memberID int64
	if err := db.QueryRow(`INSERT INTO users(email,password_hash,display_name,email_verified) VALUES('owner@example.com','hash','Owner',true) RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO users(email,password_hash,display_name,email_verified) VALUES('member@example.com','hash','Member',true) RETURNING id`).Scan(&memberID); err != nil {
		t.Fatal(err)
	}
	var householdID, choreID int64
	if err := db.QueryRow(`INSERT INTO households(name,invite_code) VALUES('Shared','operator-test') RETURNING id`).Scan(&householdID); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO user_households(user_id,household_id,role) VALUES($1,$3,'member'),($2,$3,'owner')`, ownerID, memberID, householdID)
	if err := db.QueryRow(`INSERT INTO chores(household_id,name) VALUES($1,'Shared chore') RETURNING id`, householdID).Scan(&choreID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	mustExec(t, db, `INSERT INTO sessions(id,user_id,token_hash,expires_at,created_at,authenticated_at,auth_version)
	 VALUES('owner-session',$1,'owner-hash',$2,$3,$3,0)`, ownerID, now.Add(time.Hour), now)
	// The chore is attributed to the member, but the operator performed it.
	mustExec(t, db, `INSERT INTO chore_logs(household_id,user_id,chore_id,completed_at,created_at,idempotency_actor_id)
	 VALUES($1,$2,$3,$4,$4,$5)`, householdID, memberID, choreID, now, ownerID)
	service := NewService(db, ownerID)
	service.now = func() time.Time { return now }
	owner := auth.User{ID: ownerID, EmailVerified: true, AuthVersion: 0, SessionHash: "owner-hash"}
	member := auth.User{ID: memberID, EmailVerified: true, Role: "owner"}
	if _, err := service.AuthorizeSession(member); !errors.Is(err, ErrDenied) {
		t.Fatalf("household owner obtained platform access: %v", err)
	}
	if _, err := service.Summary(ctx, Access{}); !errors.Is(err, ErrDenied) {
		t.Fatalf("reporting accepted missing service authorization: %v", err)
	}
	access, err := service.AuthorizeSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.Summary(ctx, access)
	if err != nil || summary.RegisteredUsers != 2 || summary.Authors7Days != 1 || summary.ActiveHouseholds30Days != 1 {
		t.Fatalf("unexpected summary: %+v, %v", summary, err)
	}
	users, err := service.Users(ctx, access, 0, 50)
	if err != nil || len(users.Users) != 2 {
		t.Fatalf("unexpected users page: %+v, %v", users, err)
	}
	if users.Users[0].Authored7Days != 1 || users.Users[1].Authored7Days != 0 ||
		users.Users[0].HouseholdLogs30Days != 1 || users.Users[1].HouseholdLogs30Days != 1 {
		t.Fatalf("actor and attributed user were conflated: %+v", users.Users)
	}
	mustExec(t, db, `INSERT INTO households(name,invite_code) VALUES('Second','operator-second')`)
	households, err := service.Households(ctx, access, 0, 1)
	if err != nil || len(households.Households) != 1 || households.NextCursor == "" ||
		households.Households[0].Members != 2 || households.Households[0].Logs30Days != 1 {
		t.Fatalf("unexpected first household page: %+v, %v", households, err)
	}
	firstID, err := strconv.ParseInt(households.NextCursor, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	households, err = service.Households(ctx, access, firstID, 1)
	if err != nil || len(households.Households) != 1 || households.Households[0].Name != "Second" || households.NextCursor != "" {
		t.Fatalf("unexpected second household page: %+v, %v", households, err)
	}
	activity, err := service.Activity(ctx, access, 30)
	if err != nil || len(activity.Days) != 30 || activity.Days[29].Authors != 1 {
		t.Fatalf("unexpected daily activity: %+v, %v", activity, err)
	}
	key, token, err := service.CreateKey(ctx, owner, "Local report", ScopeSummary, 7)
	if err != nil || token == "" {
		t.Fatalf("create key: %+v, %v", key, err)
	}
	var hashLength int
	if err := db.QueryRow(`SELECT length(token_hash) FROM operator_api_keys WHERE id=$1`, key.ID).Scan(&hashLength); err != nil || hashLength != 32 {
		t.Fatalf("key digest not stored as SHA-256: %d, %v", hashLength, err)
	}
	readAccess, err := service.AuthenticateKey(ctx, token, ScopeSummary)
	if err != nil {
		t.Fatal(err)
	}
	if readAccess.KeyID() != key.ID {
		t.Fatalf("authenticated key ID = %q, want persisted ID %q", readAccess.KeyID(), key.ID)
	}
	if _, err := service.Users(ctx, readAccess, 0, 50); !errors.Is(err, ErrDenied) {
		t.Fatalf("summary key read email data: %v", err)
	}
	if _, err := service.AuthenticateKey(ctx, token, ScopeFull); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("summary key authenticated for full scope: %v", err)
	}
	if _, err := service.AuthenticateKey(ctx, token+"x", ScopeSummary); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("tampered key authenticated: %v", err)
	}
	if err := service.RevokeKey(ctx, owner, key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateKey(ctx, token, ScopeSummary); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("revoked key authenticated: %v", err)
	}
	_, fullToken, err := service.CreateKey(ctx, owner, "Full report", ScopeFull, 30)
	if err != nil {
		t.Fatal(err)
	}
	fullAccess, err := service.AuthenticateKey(ctx, fullToken, ScopeFull)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Users(ctx, fullAccess, 0, 50); err != nil {
		t.Fatalf("full key failed to read users: %v", err)
	}
	mustExec(t, db, `UPDATE users SET auth_version = auth_version + 1 WHERE id=$1`, ownerID)
	if _, err := service.AuthenticateKey(ctx, fullToken, ScopeFull); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("key survived credential reset: %v", err)
	}
	// A recovered account can create replacement credentials. Old keys are
	// visibly invalidated and do not consume the five-live-key allowance.
	owner.AuthVersion++
	list, err := service.ListKeys(ctx, owner)
	if err != nil || len(list) != 2 || !list[0].Invalidated {
		t.Fatalf("recovery did not mark old keys invalid: %+v, %v", list, err)
	}
	mustExec(t, db, `UPDATE sessions SET auth_version=$1 WHERE token_hash='owner-hash'`, owner.AuthVersion)
	_, replacementToken, err := service.CreateKey(ctx, owner, "Replacement", ScopeFull, 90)
	if err != nil {
		t.Fatalf("recovery blocked replacement key: %v", err)
	}
	// Revoked history can exceed the list cap, but every live key must stay
	// visible and revocable to the owner.
	for i := 0; i < 105; i++ {
		mustExec(t, db, `INSERT INTO operator_api_keys(id,owner_user_id,name,scope,token_hash,auth_version,created_at,expires_at,revoked_at)
		 VALUES($1,$2,'retired','full',$3,$4,$5,$6,$5)`, fmt.Sprintf("old-key-%09d", i), ownerID,
			[]byte(fmt.Sprintf("historic-key-%03d", i)), owner.AuthVersion, now.Add(time.Duration(i+1)*time.Second), now.AddDate(1, 0, 0))
	}
	list, err = service.ListKeys(ctx, owner)
	if err != nil || len(list) != 100 || list[0].Name != "Replacement" {
		t.Fatalf("live key hidden by retired history: keys=%+v, err=%v", list, err)
	}
	service.now = func() time.Time { return now.AddDate(0, 0, 91) }
	if _, err := service.AuthenticateKey(ctx, replacementToken, ScopeFull); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("expired key authenticated: %v", err)
	}
}

func TestOperatorKeyRequiresRecentSessionAndVerifiedOwner(t *testing.T) {
	db := testdb.New(t)
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var ownerID int64
	if err := db.QueryRow(`INSERT INTO users(email,password_hash,display_name,email_verified) VALUES('owner@example.com','hash','Owner',true) RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	mustExec(t, db, `INSERT INTO sessions(id,user_id,token_hash,expires_at,created_at,authenticated_at,auth_version)
	 VALUES('old',$1,'old-hash',$2,$3,$3,0)`, ownerID, now.Add(time.Hour), now.Add(-11*time.Minute))
	s := NewService(db, ownerID)
	s.now = func() time.Time { return now }
	owner := auth.User{ID: ownerID, EmailVerified: true, SessionHash: "old-hash"}
	if _, _, err := s.CreateKey(context.Background(), owner, "test", ScopeFull, 7); !errors.Is(err, ErrRecentLogin) {
		t.Fatalf("old session created key: %v", err)
	}
	mustExec(t, db, `UPDATE sessions SET created_at=$1, authenticated_at=$2 WHERE token_hash='old-hash'`, now.Add(-11*time.Minute), now.Add(-10*time.Minute))
	if err := s.RequireRecentSession(context.Background(), owner); err != nil {
		t.Fatalf("proof exactly ten minutes old was denied: %v", err)
	}
	mustExec(t, db, `UPDATE sessions SET created_at=$1, authenticated_at=$2 WHERE token_hash='old-hash'`, now, now.Add(-10*time.Minute-time.Microsecond))
	if err := s.RequireRecentSession(context.Background(), owner); !errors.Is(err, ErrRecentLogin) {
		t.Fatalf("fresh session with old proof was accepted: %v", err)
	}
	mustExec(t, db, `UPDATE sessions SET authenticated_at=NULL WHERE token_hash='old-hash'`)
	if err := s.RequireRecentSession(context.Background(), owner); !errors.Is(err, ErrRecentLogin) {
		t.Fatalf("session without a proven auth time was accepted: %v", err)
	}
	owner.EmailVerified = false
	if _, err := s.AuthorizeSession(owner); !errors.Is(err, ErrDenied) {
		t.Fatalf("unverified owner authorized: %v", err)
	}
}

func TestPasswordlessSetupDoesNotRefreshOperatorProof(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var ownerID int64
	const email = "passwordless-operator@example.com"
	if err := db.QueryRow(`INSERT INTO users(email,password_hash,display_name,email_verified)
		VALUES($1,'','Owner',true) RETURNING id`, email).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	oldToken := "passwordless-old-session"
	digest := sha256.Sum256([]byte(oldToken))
	oldHash := base64.RawURLEncoding.EncodeToString(digest[:])
	now := time.Now().UTC().Truncate(time.Microsecond)
	mustExec(t, db, `INSERT INTO sessions(id,user_id,token_hash,expires_at,last_seen_at,created_at,authenticated_at,auth_version)
		VALUES('old-session',$1,$2,$3,$4,$5,$5,0)`, ownerID, oldHash, now.Add(time.Hour), now, now.Add(-20*time.Minute))
	authService := auth.NewService(auth.NewPostgresStore(db))
	operatorService := NewService(db, ownerID)
	operatorService.now = func() time.Time { return now }
	oldUser, err := authService.Authenticate(ctx, oldToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := operatorService.RequireRecentSession(ctx, oldUser); !errors.Is(err, ErrRecentLogin) {
		t.Fatalf("old passwordless session was accepted: %v", err)
	}
	actorCtx := audit.WithActor(ctx, audit.Actor{UserID: ownerID, AuthVersion: oldUser.AuthVersion})
	_, setupSession, err := authService.ChangePassword(actorCtx, ownerID, "", "new-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	setupUser, err := authService.Authenticate(ctx, setupSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := operatorService.CreateKey(ctx, setupUser, "must reauthenticate", ScopeFull, 7); !errors.Is(err, ErrRecentLogin) {
		t.Fatalf("password setup refreshed proof without authentication: %v", err)
	}
	_, loginSession, err := authService.Login(ctx, email, "new-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	loggedIn, err := authService.Authenticate(ctx, loginSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := operatorService.CreateKey(ctx, loggedIn, "recent login", ScopeFull, 7); err != nil {
		t.Fatalf("genuine password login was denied: %v", err)
	}
}

func TestConcurrentOperatorKeyCreationHonorsCap(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var ownerID int64
	if err := db.QueryRow(`INSERT INTO users(email,password_hash,display_name,email_verified) VALUES('owner@example.com','hash','Owner',true) RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	mustExec(t, db, `INSERT INTO sessions(id,user_id,token_hash,expires_at,created_at,authenticated_at,auth_version)
	 VALUES('owner-session',$1,'owner-hash',$2,$3,$3,0)`, ownerID, now.Add(time.Hour), now)
	service := NewService(db, ownerID)
	service.now = func() time.Time { return now }
	owner := auth.User{ID: ownerID, EmailVerified: true, AuthVersion: 0, SessionHash: "owner-hash"}

	const requests = 8
	results := make(chan error, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := service.CreateKey(ctx, owner, fmt.Sprintf("Concurrent %d", i), ScopeFull, 7)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	created, capped := 0, 0
	for err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrTooManyKeys):
			capped++
		default:
			t.Fatalf("unexpected concurrent creation error: %v", err)
		}
	}
	if created != 5 || capped != requests-5 {
		t.Fatalf("key cap was not serialized: created=%d capped=%d", created, capped)
	}
}
