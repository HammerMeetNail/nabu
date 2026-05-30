package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type OIDCProvider interface {
	Enabled() bool
	AuthCodeURL(state, nonce string) string
	ExchangeCode(ctx context.Context, code, expectedNonce string) (OIDCIdentity, error)
}

type OIDCIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
}

type GoogleOIDCProvider struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AuthURL      string
	TokenURL     string
	Issuer       string
	JWKsURL      string
	httpClient   *http.Client

	jwksMu      sync.RWMutex
	jwksKeys    map[string]*rsa.PublicKey // kid → key
	jwksFetched time.Time
}

func (p *GoogleOIDCProvider) Enabled() bool {
	return p.ClientID != "" && p.ClientSecret != ""
}

func (p *GoogleOIDCProvider) AuthCodeURL(state, nonce string) string {
	params := url.Values{
		"client_id":     {p.ClientID},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"redirect_uri":  {p.RedirectURL},
		"state":         {state},
		"nonce":         {nonce},
		"access_type":   {"online"},
	}
	return p.AuthURL + "?" + params.Encode()
}

func (p *GoogleOIDCProvider) ExchangeCode(ctx context.Context, code, expectedNonce string) (OIDCIdentity, error) {
	client := p.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	tokenData, err := p.exchangeForm(ctx, client, code)
	if err != nil {
		return OIDCIdentity{}, err
	}

	idToken, ok := tokenData["id_token"].(string)
	if !ok || idToken == "" {
		return OIDCIdentity{}, fmt.Errorf("missing id_token in response")
	}

	return p.verifyToken(ctx, idToken, expectedNonce)
}

func (p *GoogleOIDCProvider) exchangeForm(ctx context.Context, client *http.Client, code string) (map[string]any, error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"redirect_uri":  {p.RedirectURL},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	return result, nil
}

// verifyToken validates an RS256-signed Google ID token.
// It verifies: signature (via JWKS), iss, aud, exp, and nonce.
func (p *GoogleOIDCProvider) verifyToken(ctx context.Context, idToken, expectedNonce string) (OIDCIdentity, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return OIDCIdentity{}, fmt.Errorf("invalid id_token format")
	}

	// Decode and parse header.
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return OIDCIdentity{}, fmt.Errorf("decode id_token header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return OIDCIdentity{}, fmt.Errorf("parse id_token header: %w", err)
	}
	if header.Alg != "RS256" {
		return OIDCIdentity{}, fmt.Errorf("unsupported JWT algorithm: %q", header.Alg)
	}

	// Decode and parse payload claims.
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return OIDCIdentity{}, fmt.Errorf("decode id_token payload: %w", err)
	}
	var claims struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Nonce         string `json:"nonce"`
		Iss           string `json:"iss"`
		Aud           any    `json:"aud"` // may be string or []string
		Exp           int64  `json:"exp"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return OIDCIdentity{}, fmt.Errorf("parse id_token claims: %w", err)
	}

	// Verify issuer.
	if claims.Iss != p.Issuer {
		return OIDCIdentity{}, fmt.Errorf("invalid iss: %q", claims.Iss)
	}

	// Verify audience.
	if !containsAudience(claims.Aud, p.ClientID) {
		return OIDCIdentity{}, fmt.Errorf("aud does not contain client_id")
	}

	// Verify expiry.
	if time.Now().Unix() > claims.Exp {
		return OIDCIdentity{}, fmt.Errorf("id_token expired")
	}

	// Verify nonce (mandatory — reject if absent).
	if claims.Nonce == "" || claims.Nonce != expectedNonce {
		return OIDCIdentity{}, fmt.Errorf("nonce missing or mismatch")
	}

	// Verify RS256 signature.
	key, err := p.getJWK(ctx, header.Kid)
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
		EmailVerified: claims.EmailVerified,
	}, nil
}

// containsAudience checks whether a JWT aud claim (string or []string) contains the target.
func containsAudience(aud any, target string) bool {
	switch v := aud.(type) {
	case string:
		return v == target
	case []any:
		for _, a := range v {
			if s, ok := a.(string); ok && s == target {
				return true
			}
		}
	}
	return false
}

// jwksResponse is the minimal JWKS JSON structure we care about.
type jwksResponse struct {
	Keys []struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

// getJWK returns the RSA public key for the given kid, fetching the JWKS if needed.
func (p *GoogleOIDCProvider) getJWK(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	p.jwksMu.RLock()
	key, ok := p.jwksKeys[kid]
	stale := time.Since(p.jwksFetched) > 1*time.Hour
	p.jwksMu.RUnlock()

	if ok && !stale {
		return key, nil
	}

	// Fetch fresh JWKS.
	if err := p.refreshJWKS(ctx); err != nil {
		return nil, err
	}

	p.jwksMu.RLock()
	key, ok = p.jwksKeys[kid]
	p.jwksMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no JWK found for kid %q", kid)
	}
	return key, nil
}

func (p *GoogleOIDCProvider) refreshJWKS(ctx context.Context) error {
	jwksURL := p.JWKsURL
	if jwksURL == "" {
		jwksURL = "https://www.googleapis.com/oauth2/v3/certs"
	}
	client := p.httpClient
	if client == nil {
		client = http.DefaultClient
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

	p.jwksMu.Lock()
	p.jwksKeys = keys
	p.jwksFetched = time.Now()
	p.jwksMu.Unlock()
	return nil
}

func rsaPublicKeyFromJWK(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	// Pad eBytes to 4 bytes big-endian.
	if len(eBytes) < 4 {
		padded := make([]byte, 4)
		copy(padded[4-len(eBytes):], eBytes)
		eBytes = padded
	}
	e := int(binary.BigEndian.Uint32(eBytes))
	n := new(big.Int).SetBytes(nBytes)
	return &rsa.PublicKey{N: n, E: e}, nil
}

func GenerateState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func GenerateNonce() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	hash := sha256.Sum256(buf)
	return base64.RawURLEncoding.EncodeToString(hash[:]), nil
}
