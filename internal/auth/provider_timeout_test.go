package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type providerTransportFunc func(*http.Request) (*http.Response, error)

func (f providerTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOAuthCallsBoundLifetimeAndHonorCancellation(t *testing.T) {
	for _, kind := range []string{"google exchange", "google keys", "apple keys"} {
		t.Run(kind, func(t *testing.T) {
			entered := make(chan bool, 1)
			client := &http.Client{Transport: providerTransportFunc(func(r *http.Request) (*http.Response, error) {
				deadline, ok := r.Context().Deadline()
				entered <- ok && time.Until(deadline) <= oauthTimeout
				<-r.Context().Done()
				return nil, r.Context().Err()
			})}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			google := &GoogleOIDCProvider{httpClient: client, TokenURL: "https://provider.invalid/token", JWKsURL: "https://provider.invalid/keys"}
			apple := &AppleVerifier{httpClient: client, JWKsURL: "https://provider.invalid/keys"}
			done := make(chan error, 1)
			go func() {
				var err error
				switch kind {
				case "google exchange":
					_, err = google.ExchangeCode(ctx, "code", "nonce")
				case "google keys":
					err = google.refreshJWKS(ctx)
				case "apple keys":
					err = apple.refreshJWKS(ctx)
				}
				done <- err
			}()
			select {
			case bounded := <-entered:
				if !bounded {
					t.Error("provider request has no deadline")
				}
			case <-time.After(time.Second):
				t.Fatal("provider request did not start")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("provider cancellation=%v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("provider outlived request cancellation")
			}
		})
	}
}

func TestGoogleExchangeAndJWKSShareOneDeadline(t *testing.T) {
	kit := newAppleTestKit(t)
	token := kit.mint(t, map[string]any{"iss": "test-issuer", "aud": "test-client", "email_verified": true})
	tokenJSON, err := json.Marshal(map[string]string{"id_token": token})
	if err != nil {
		t.Fatal(err)
	}
	type stage struct {
		path     string
		deadline time.Time
		bounded  bool
	}
	stages := make(chan stage, 2)
	client := &http.Client{Transport: providerTransportFunc(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		stages <- stage{r.URL.Path, deadline, ok}
		if r.URL.Path == "/token" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(tokenJSON))), Header: make(http.Header)}, nil
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	p := &GoogleOIDCProvider{ClientID: "test-client", Issuer: "test-issuer", TokenURL: "https://provider.invalid/token", JWKsURL: "https://provider.invalid/keys", httpClient: client}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := p.ExchangeCode(ctx, "code", "expected-nonce"); done <- err }()
	var observed []stage
	for range 2 {
		select {
		case value := <-stages:
			observed = append(observed, value)
		case <-time.After(time.Second):
			t.Fatal("exchange did not reach both provider stages")
		}
	}
	if observed[0].path != "/token" || observed[1].path != "/keys" || !observed[0].bounded || !observed[1].bounded || !observed[0].deadline.Equal(observed[1].deadline) {
		t.Error("JWKS started a fresh time budget after token exchange")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("shared exchange cancellation=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("JWKS outlived exchange cancellation")
	}
}
