package auth

import (
	"testing"
)

func TestGenerateState(t *testing.T) {
	state, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState: %v", err)
	}
	if state == "" {
		t.Fatal("expected non-empty state")
	}
}

func TestGenerateNonce(t *testing.T) {
	nonce, err := GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce: %v", err)
	}
	if nonce == "" {
		t.Fatal("expected non-empty nonce")
	}
}

func TestOIDCProviderNotEnabled(t *testing.T) {
	provider := &GoogleOIDCProvider{}
	if provider.Enabled() {
		t.Fatal("expected provider not enabled")
	}
	svc := NewService(NewMemoryStore())
	svc.SetOIDCProvider(provider)
	_, err := svc.GoogleAuthCodeURL("state", "nonce")
	if err != ErrOIDCUnavailable {
		t.Fatalf("error = %v, want ErrOIDCUnavailable", err)
	}
}

func TestOIDCProviderAuthURL(t *testing.T) {
	provider := &GoogleOIDCProvider{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURL:  "http://localhost/callback",
		AuthURL:      "https://accounts.test/auth",
		TokenURL:     "https://accounts.test/token",
		Issuer:       "https://accounts.test",
	}
	if !provider.Enabled() {
		t.Fatal("expected provider enabled")
	}

	url := provider.AuthCodeURL("mystate", "mynonce")
	if url == "" {
		t.Fatal("expected auth URL")
	}
	expectedPrefix := "https://accounts.test/auth?"
	if len(url) < len(expectedPrefix) || url[:len(expectedPrefix)] != expectedPrefix {
		t.Fatalf("url = %q", url)
	}
}
