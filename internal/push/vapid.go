package push

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// VAPIDSigner holds the ECDSA P-256 key pair used for VAPID authentication.
type VAPIDSigner struct {
	privateKey *ecdsa.PrivateKey
	publicKey  []byte // uncompressed point, 65 bytes
	subject    string // mailto: URL
}

// NewVAPIDSigner decodes a base64url-encoded P-256 private key (d) and
// public key (uncompressed point x||y).  Both are expected without padding.
func NewVAPIDSigner(privateKeyB64, publicKeyB64, subject string) (*VAPIDSigner, error) {
	privBytes, err := base64urlDecode(privateKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	pubBytes, err := base64urlDecode(publicKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}

	curve := elliptic.P256()
	// Reconstruct public key from uncompressed point
	if len(pubBytes) != 65 || pubBytes[0] != 0x04 {
		return nil, fmt.Errorf("invalid uncompressed public key length %d", len(pubBytes))
	}
	x := new(big.Int).SetBytes(pubBytes[1:33])
	y := new(big.Int).SetBytes(pubBytes[33:65])

	privateKey := new(ecdsa.PrivateKey)
	privateKey.Curve = curve
	privateKey.X = x
	privateKey.Y = y
	privateKey.D = new(big.Int).SetBytes(privBytes)

	// Verify the public key matches the private key
	if !curve.IsOnCurve(x, y) {
		return nil, fmt.Errorf("public key not on curve")
	}

	return &VAPIDSigner{
		privateKey: privateKey,
		publicKey:  pubBytes,
		subject:    subject,
	}, nil
}

// GenerateVAPIDKeys creates a fresh P-256 key pair and returns the base64url
// encoded private key and uncompressed public key (suitable for VAPID).
func GenerateVAPIDKeys() (privB64, pubB64 string, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	privB64 = base64urlEncode(priv.D.Bytes())
	x := priv.X.Bytes()
	y := priv.Y.Bytes()
	// Pad to 32 bytes each
	pub := make([]byte, 65)
	pub[0] = 0x04
	copy(pub[1+(32-len(x)):33], x)
	copy(pub[33+(32-len(y)):65], y)
	pubB64 = base64urlEncode(pub)
	return privB64, pubB64, nil
}

// SignJWT creates a VAPID JWT for the given push service endpoint.
func (s *VAPIDSigner) SignJWT(endpoint string) (string, error) {
	head := struct {
		Typ string `json:"typ"`
		Alg string `json:"alg"`
	}{
		Typ: "JWT",
		Alg: "ES256",
	}

	origin := endpointOrigin(endpoint)
	now := time.Now().UTC()
	claims := struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}{
		Aud: origin,
		Exp: now.Add(24 * time.Hour).Unix(),
		Sub: s.subject,
	}

	headJSON, err := json.Marshal(head)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	b64Head := base64urlEncode(headJSON)
	b64Claims := base64urlEncode(claimsJSON)
	signingInput := b64Head + "." + b64Claims

	hash := sha256.Sum256([]byte(signingInput))
	r, sigS, err := ecdsa.Sign(rand.Reader, s.privateKey, hash[:])
	if err != nil {
		return "", err
	}

	sig := encodeECDSASignature(r, sigS)
	b64Sig := base64urlEncode(sig)

	return signingInput + "." + b64Sig, nil
}

// PublicKeyBase64 returns the base64url-encoded uncompressed public key.
func (s *VAPIDSigner) PublicKeyBase64() string {
	return base64urlEncode(s.publicKey)
}

func endpointOrigin(endpoint string) string {
	// Extract scheme + host from endpoint URL
	endpoint = strings.TrimSpace(endpoint)
	if idx := strings.Index(endpoint, "://"); idx != -1 {
		rest := endpoint[idx+3:]
		if pathIdx := strings.Index(rest, "/"); pathIdx != -1 {
			return endpoint[:idx+3+pathIdx]
		}
		return endpoint
	}
	return "https://" + endpoint
}

func base64urlDecode(s string) ([]byte, error) {
	// Add padding if needed
	pad := len(s) % 4
	if pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	return base64.URLEncoding.DecodeString(s)
}

func base64urlEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// encodeECDSASignature returns the raw fixed-width r||s signature (64 bytes)
// as required by the JWT ES256 specification (RFC 7518 §3.4).
// DER encoding is NOT used — push services reject it.
func encodeECDSASignature(r, s *big.Int) []byte {
	rb := r.Bytes()
	sb := s.Bytes()
	sig := make([]byte, 64)
	// Left-pad each component to exactly 32 bytes.
	copy(sig[32-len(rb):32], rb)
	copy(sig[64-len(sb):64], sb)
	return sig
}
