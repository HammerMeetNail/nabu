package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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
	httpClient   *http.Client
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

func (p *GoogleOIDCProvider) verifyToken(_ context.Context, idToken, expectedNonce string) (OIDCIdentity, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return OIDCIdentity{}, fmt.Errorf("invalid id_token format")
	}

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
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return OIDCIdentity{}, fmt.Errorf("parse id_token claims: %w", err)
	}

	if claims.Iss != p.Issuer {
		return OIDCIdentity{}, fmt.Errorf("invalid iss: %s", claims.Iss)
	}

	if claims.Nonce != "" && claims.Nonce != expectedNonce {
		return OIDCIdentity{}, fmt.Errorf("nonce mismatch")
	}

	return OIDCIdentity{
		Subject:       claims.Sub,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
	}, nil
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
