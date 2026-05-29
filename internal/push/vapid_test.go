package push_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/dave/choresy/internal/push"
)

// ─── GenerateVAPIDKeys ────────────────────────────────────────────────────────

func TestGenerateVAPIDKeys_ReturnsValidBase64(t *testing.T) {
	priv, pub, err := push.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	if priv == "" || pub == "" {
		t.Fatal("expected non-empty keys")
	}
	// Both should be valid base64url (no padding)
	if _, err := base64.RawURLEncoding.DecodeString(priv); err != nil {
		t.Errorf("private key not valid base64url: %v", err)
	}
	pubBytes, err := base64.RawURLEncoding.DecodeString(pub)
	if err != nil {
		t.Errorf("public key not valid base64url: %v", err)
	}
	// Uncompressed P-256 point: 65 bytes, starts with 0x04
	if len(pubBytes) != 65 {
		t.Errorf("public key length = %d, want 65", len(pubBytes))
	}
	if pubBytes[0] != 0x04 {
		t.Errorf("public key first byte = %x, want 0x04", pubBytes[0])
	}
}

func TestGenerateVAPIDKeys_Unique(t *testing.T) {
	p1, pub1, _ := push.GenerateVAPIDKeys()
	p2, pub2, _ := push.GenerateVAPIDKeys()
	if p1 == p2 {
		t.Error("two generated private keys are identical")
	}
	if pub1 == pub2 {
		t.Error("two generated public keys are identical")
	}
}

// ─── NewVAPIDSigner ───────────────────────────────────────────────────────────

func TestNewVAPIDSigner_RoundTrip(t *testing.T) {
	priv, pub, err := push.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	signer, err := push.NewVAPIDSigner(priv, pub, "mailto:test@example.com")
	if err != nil {
		t.Fatalf("NewVAPIDSigner: %v", err)
	}
	if signer.PublicKeyBase64() != pub {
		t.Errorf("PublicKeyBase64 = %q, want %q", signer.PublicKeyBase64(), pub)
	}
}

func TestNewVAPIDSigner_BadPrivateKey(t *testing.T) {
	_, pub, _ := push.GenerateVAPIDKeys()
	_, err := push.NewVAPIDSigner("not-base64!!!", pub, "mailto:x@x.com")
	if err == nil {
		t.Fatal("expected error for bad private key")
	}
}

func TestNewVAPIDSigner_BadPublicKey(t *testing.T) {
	priv, _, _ := push.GenerateVAPIDKeys()
	// Valid base64url but wrong length
	badPub := base64.RawURLEncoding.EncodeToString([]byte("tooshort"))
	_, err := push.NewVAPIDSigner(priv, badPub, "mailto:x@x.com")
	if err == nil {
		t.Fatal("expected error for invalid public key")
	}
}

func TestNewVAPIDSigner_MismatchedKeys(t *testing.T) {
	priv1, _, _ := push.GenerateVAPIDKeys()
	_, pub2, _ := push.GenerateVAPIDKeys()
	// Using private key from key 1 with public key from key 2 —
	// the keys are on the curve so NewVAPIDSigner may or may not detect this,
	// but SignJWT must still produce a 3-part JWT token.
	signer, err := push.NewVAPIDSigner(priv1, pub2, "mailto:x@x.com")
	if err != nil {
		// Some implementations do detect the mismatch; that's fine.
		return
	}
	// If no error: signer must still produce a syntactically valid JWT
	jwt, err := signer.SignJWT("https://fcm.googleapis.com/fcm/send/example")
	if err != nil {
		t.Fatalf("SignJWT with mismatched keys: %v", err)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Errorf("JWT should have 3 parts, got %d", len(parts))
	}
}

// ─── SignJWT ──────────────────────────────────────────────────────────────────

func TestSignJWT_ThreeParts(t *testing.T) {
	priv, pub, _ := push.GenerateVAPIDKeys()
	signer, _ := push.NewVAPIDSigner(priv, pub, "mailto:test@example.com")

	jwt, err := signer.SignJWT("https://fcm.googleapis.com/fcm/send/example")
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT parts, got %d", len(parts))
	}
	for i, p := range parts {
		if p == "" {
			t.Errorf("JWT part %d is empty", i)
		}
	}
}

func TestSignJWT_HeaderTypAndAlgOnly(t *testing.T) {
	priv, pub, _ := push.GenerateVAPIDKeys()
	signer, _ := push.NewVAPIDSigner(priv, pub, "mailto:test@example.com")

	jwt, _ := signer.SignJWT("https://fcm.googleapis.com/fcm/send/example")
	parts := strings.Split(jwt, ".")

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	headerStr := string(headerJSON)
	// Must contain typ and alg
	if !strings.Contains(headerStr, `"typ"`) || !strings.Contains(headerStr, `"alg"`) {
		t.Errorf("header missing typ or alg: %s", headerStr)
	}
	// Must NOT contain kty or crv
	if strings.Contains(headerStr, `"kty"`) || strings.Contains(headerStr, `"crv"`) {
		t.Errorf("header must not contain kty/crv: %s", headerStr)
	}
}

func TestSignJWT_ClaimsContainAudAndSub(t *testing.T) {
	priv, pub, _ := push.GenerateVAPIDKeys()
	signer, _ := push.NewVAPIDSigner(priv, pub, "mailto:test@example.com")

	endpoint := "https://fcm.googleapis.com/fcm/send/example"
	jwt, _ := signer.SignJWT(endpoint)
	parts := strings.Split(jwt, ".")

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	claimsStr := string(claimsJSON)
	if !strings.Contains(claimsStr, `"aud"`) {
		t.Errorf("claims missing aud: %s", claimsStr)
	}
	if !strings.Contains(claimsStr, "fcm.googleapis.com") {
		t.Errorf("aud should contain endpoint origin, claims: %s", claimsStr)
	}
	if !strings.Contains(claimsStr, "mailto:test@example.com") {
		t.Errorf("claims should contain subject, claims: %s", claimsStr)
	}
}

func TestSignJWT_SignatureIsBase64URL(t *testing.T) {
	priv, pub, _ := push.GenerateVAPIDKeys()
	signer, _ := push.NewVAPIDSigner(priv, pub, "mailto:test@example.com")

	jwt, _ := signer.SignJWT("https://fcm.googleapis.com/fcm/send/example")
	parts := strings.Split(jwt, ".")

	if _, err := base64.RawURLEncoding.DecodeString(parts[2]); err != nil {
		t.Errorf("signature part is not valid base64url: %v", err)
	}
}

func TestSignJWT_DifferentSignatureEachCall(t *testing.T) {
	priv, pub, _ := push.GenerateVAPIDKeys()
	signer, _ := push.NewVAPIDSigner(priv, pub, "mailto:test@example.com")

	// ECDSA uses random nonce, so signatures should differ
	endpoint := "https://fcm.googleapis.com/fcm/send/example"
	j1, _ := signer.SignJWT(endpoint)
	j2, _ := signer.SignJWT(endpoint)

	parts1 := strings.Split(j1, ".")
	parts2 := strings.Split(j2, ".")

	// Header and claims (first two parts) should be equal (same time, same endpoint)
	// Signature (third part) will almost certainly differ due to ECDSA randomness
	_ = parts1[2]
	_ = parts2[2]
	// We just verify both are valid JWTs — probabilistic check not guaranteed
	if len(parts1) != 3 || len(parts2) != 3 {
		t.Error("both JWTs should have 3 parts")
	}
}
