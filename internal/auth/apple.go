package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// AppleTokenVerifier verifies Sign in with Apple identity tokens delivered by
// a native client (ASAuthorizationController hands the app an identity token
// directly; there is no authorization-code exchange in the native flow).
type AppleTokenVerifier interface {
	Enabled() bool
	VerifyIdentityToken(ctx context.Context, identityToken, expectedNonce string) (OIDCIdentity, error)
}

const (
	appleIssuer       = "https://appleid.apple.com"
	appleJWKsURL      = "https://appleid.apple.com/auth/keys"
	appleAuthorizeURL = "https://appleid.apple.com/auth/authorize"
)

// AppleWebAuth builds authorization URLs for the web Sign in with Apple
// flow. The flow requests `response_type=code id_token` with
// `response_mode=form_post`, so Apple POSTs the identity token straight to
// the callback and it can be verified exactly like the native flow — no
// authorization-code exchange (and therefore no client secret) is needed.
type AppleWebAuth struct {
	ClientID    string // the Services ID registered for the web flow
	RedirectURL string
	AuthURL     string // defaults to appleAuthorizeURL
}

func (a *AppleWebAuth) AuthCodeURL(state, nonce string) string {
	authURL := a.AuthURL
	if authURL == "" {
		authURL = appleAuthorizeURL
	}
	q := url.Values{}
	q.Set("client_id", a.ClientID)
	q.Set("redirect_uri", a.RedirectURL)
	q.Set("response_type", "code id_token")
	q.Set("response_mode", "form_post")
	q.Set("scope", "email")
	q.Set("state", state)
	q.Set("nonce", nonce)
	return authURL + "?" + q.Encode()
}

// AppleVerifier validates RS256 identity tokens against Apple's JWKS.
// ClientIDs are the accepted audiences: the iOS app's bundle ID and,
// if the web flow is added later, the Services ID.
type AppleVerifier struct {
	ClientIDs  []string
	Issuer     string // defaults to appleIssuer
	JWKsURL    string // defaults to appleJWKsURL
	httpClient *http.Client

	jwksMu      sync.RWMutex
	jwksKeys    map[string]*rsa.PublicKey // kid → key
	jwksFetched time.Time
}

func NewAppleVerifier(clientIDs []string) *AppleVerifier {
	return &AppleVerifier{ClientIDs: clientIDs}
}

func (v *AppleVerifier) Enabled() bool {
	return len(v.ClientIDs) > 0
}

// VerifyIdentityToken validates signature (via Apple JWKS), iss, aud, exp,
// and nonce. The nonce is mandatory: the native client generates one per
// request and Apple echoes it into the token, which is what stops a token
// harvested elsewhere from being replayed against this endpoint.
func (v *AppleVerifier) VerifyIdentityToken(ctx context.Context, identityToken, expectedNonce string) (OIDCIdentity, error) {
	if expectedNonce == "" {
		return OIDCIdentity{}, fmt.Errorf("nonce required")
	}

	parts := strings.Split(identityToken, ".")
	if len(parts) != 3 {
		return OIDCIdentity{}, fmt.Errorf("invalid identity token format")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return OIDCIdentity{}, fmt.Errorf("decode token header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return OIDCIdentity{}, fmt.Errorf("parse token header: %w", err)
	}
	if header.Alg != "RS256" {
		return OIDCIdentity{}, fmt.Errorf("unsupported JWT algorithm: %q", header.Alg)
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return OIDCIdentity{}, fmt.Errorf("decode token payload: %w", err)
	}
	var claims struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"` // Apple sends bool or the string "true"
		Nonce         string `json:"nonce"`
		Iss           string `json:"iss"`
		Aud           any    `json:"aud"`
		Exp           int64  `json:"exp"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return OIDCIdentity{}, fmt.Errorf("parse token claims: %w", err)
	}

	issuer := v.Issuer
	if issuer == "" {
		issuer = appleIssuer
	}
	if claims.Iss != issuer {
		return OIDCIdentity{}, fmt.Errorf("invalid iss: %q", claims.Iss)
	}

	audOK := false
	for _, clientID := range v.ClientIDs {
		if containsAudience(claims.Aud, clientID) {
			audOK = true
			break
		}
	}
	if !audOK {
		return OIDCIdentity{}, fmt.Errorf("aud does not match a configured client id")
	}

	if time.Now().Unix() > claims.Exp {
		return OIDCIdentity{}, fmt.Errorf("identity token expired")
	}

	if claims.Nonce == "" || claims.Nonce != expectedNonce {
		return OIDCIdentity{}, fmt.Errorf("nonce missing or mismatch")
	}

	key, err := v.getJWK(ctx, header.Kid)
	if err != nil {
		return OIDCIdentity{}, fmt.Errorf("get JWK: %w", err)
	}
	signingInput := parts[0] + "." + parts[1]
	digest := sha256.Sum256([]byte(signingInput))
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return OIDCIdentity{}, fmt.Errorf("decode signature: %w", err)
	}
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sigBytes); err != nil {
		return OIDCIdentity{}, fmt.Errorf("invalid token signature: %w", err)
	}

	return OIDCIdentity{
		Subject:       claims.Sub,
		Email:         claims.Email,
		EmailVerified: appleEmailVerified(claims.EmailVerified),
	}, nil
}

// appleEmailVerified normalizes Apple's email_verified claim, which has been
// observed as both a JSON bool and the string "true".
func appleEmailVerified(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true"
	}
	return false
}

func (v *AppleVerifier) getJWK(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.jwksMu.RLock()
	key, ok := v.jwksKeys[kid]
	stale := time.Since(v.jwksFetched) > 1*time.Hour
	v.jwksMu.RUnlock()

	if ok && !stale {
		return key, nil
	}
	if err := v.refreshJWKS(ctx); err != nil {
		return nil, err
	}

	v.jwksMu.RLock()
	key, ok = v.jwksKeys[kid]
	v.jwksMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no JWK found for kid %q", kid)
	}
	return key, nil
}

func (v *AppleVerifier) refreshJWKS(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, oauthTimeout)
	defer cancel()
	jwksURL := v.JWKsURL
	if jwksURL == "" {
		jwksURL = appleJWKsURL
	}
	client := v.httpClient
	if client == nil {
		client = oauthHTTPClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return fmt.Errorf("create JWKS request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read JWKS response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}

	var jwks jwksResponse
	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("parse JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}

	v.jwksMu.Lock()
	v.jwksKeys = keys
	v.jwksFetched = time.Now()
	v.jwksMu.Unlock()
	return nil
}
