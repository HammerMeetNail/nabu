package apikey

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HammerMeetNail/nabu/internal/auth"
)

type fakeUsers map[int64]auth.User

func (f fakeUsers) AuthenticateUserVersion(_ context.Context, userID, version int64) (auth.User, error) {
	u, ok := f[userID]
	if !ok || u.AuthVersion != version {
		return auth.User{}, auth.ErrSessionNotFound
	}
	return u, nil
}

func newTestService(users fakeUsers) *Service {
	return NewService(NewMemoryStore(), users)
}

func TestCreateAndAuthenticate(t *testing.T) {
	alice := auth.User{ID: 1, AuthVersion: 1}
	s := newTestService(fakeUsers{1: alice})
	key, token, err := s.Create(context.Background(), alice, "  script  ")
	if err != nil {
		t.Fatal(err)
	}
	if key.Name != "script" || !strings.HasPrefix(token, TokenPrefix+key.ID+".") {
		t.Fatalf("unexpected key %+v token %q", key, token)
	}
	user, id, err := s.Authenticate(context.Background(), token)
	if err != nil || user.ID != 1 || id != key.ID {
		t.Fatalf("authenticate: %+v %q %v", user, id, err)
	}
	keys, _ := s.List(context.Background(), alice)
	if len(keys) != 1 || keys[0].LastUsedAt == nil {
		t.Fatalf("expected last-used to be recorded: %+v", keys)
	}
}

func TestCreateValidatesName(t *testing.T) {
	alice := auth.User{ID: 1, AuthVersion: 1}
	s := newTestService(fakeUsers{1: alice})
	for _, name := range []string{"", "   ", strings.Repeat("x", 61), "bad\nname"} {
		if _, _, err := s.Create(context.Background(), alice, name); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("name %q: want ErrInvalidInput, got %v", name, err)
		}
	}
}

func TestActiveKeyCap(t *testing.T) {
	alice := auth.User{ID: 1, AuthVersion: 1}
	s := newTestService(fakeUsers{1: alice})
	var first Key
	for i := range MaxActiveKeys {
		k, _, err := s.Create(context.Background(), alice, "k")
		if err != nil {
			t.Fatalf("key %d: %v", i, err)
		}
		if i == 0 {
			first = k
		}
	}
	if _, _, err := s.Create(context.Background(), alice, "k"); !errors.Is(err, ErrTooManyKeys) {
		t.Fatalf("want ErrTooManyKeys, got %v", err)
	}
	if err := s.Revoke(context.Background(), alice, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Create(context.Background(), alice, "k"); err != nil {
		t.Fatalf("revoking should free a slot: %v", err)
	}
}

func TestRevokeIsOwnerScoped(t *testing.T) {
	alice, bob := auth.User{ID: 1, AuthVersion: 1}, auth.User{ID: 2, AuthVersion: 1}
	s := newTestService(fakeUsers{1: alice, 2: bob})
	key, token, _ := s.Create(context.Background(), alice, "k")
	if err := s.Revoke(context.Background(), bob, key.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob revoking alice's key: want ErrNotFound, got %v", err)
	}
	if _, _, err := s.Authenticate(context.Background(), token); err != nil {
		t.Fatalf("key should still work: %v", err)
	}
	if err := s.Revoke(context.Background(), alice, key.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Authenticate(context.Background(), token); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("revoked key: want ErrInvalidKey, got %v", err)
	}
	if err := s.Revoke(context.Background(), alice, "short"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("malformed id: want ErrNotFound, got %v", err)
	}
}

func TestAuthVersionChangeInvalidates(t *testing.T) {
	alice := auth.User{ID: 1, AuthVersion: 1}
	users := fakeUsers{1: alice}
	s := newTestService(users)
	_, token, _ := s.Create(context.Background(), alice, "k")
	users[1] = auth.User{ID: 1, AuthVersion: 2}
	if _, _, err := s.Authenticate(context.Background(), token); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("want ErrInvalidKey, got %v", err)
	}
	keys, _ := s.List(context.Background(), users[1])
	if len(keys) != 1 || !keys[0].Invalidated {
		t.Fatalf("key should list as invalidated: %+v", keys)
	}
}

func TestAuthenticateRejectsMalformedTokens(t *testing.T) {
	alice := auth.User{ID: 1, AuthVersion: 1}
	s := newTestService(fakeUsers{1: alice})
	_, token, _ := s.Create(context.Background(), alice, "k")
	for _, bad := range []string{
		"", "nabu_pat_", token[:len(token)-1], token + "A", strings.Replace(token, TokenPrefix, "nabu_op_", 1),
		TokenPrefix + strings.Repeat("a", 16) + "." + strings.Repeat("a", 43),
		token[:len(token)-1] + "B",
	} {
		if _, _, err := s.Authenticate(context.Background(), bad); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("token %q: want ErrInvalidKey, got %v", bad, err)
		}
	}
}

func TestMemoryTouchThrottled(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	_ = store.Create(context.Background(), Record{ID: "a", UserID: 1, CreatedAt: now}, 1)
	_ = store.Touch(context.Background(), "a", now)
	_ = store.Touch(context.Background(), "a", now.Add(30*time.Minute))
	rec, _ := store.Get(context.Background(), "a")
	if rec.LastUsedAt == nil || !rec.LastUsedAt.Equal(now) {
		t.Fatalf("want last used %v, got %v", now, rec.LastUsedAt)
	}
}
