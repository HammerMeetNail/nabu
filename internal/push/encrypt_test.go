package push

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	clientP256DH := "BFiD30jh-xT1-ztVT4-JzRZUkVaC3jSJpXSpsu8uy1q86f28QIg8W2iznxdqLqdlg7nYlVru_A1FjmmTmrV31Eo"
	clientAuth := "mfbNpuEjYa06PwSg5azFcw"
	payload := []byte(`{"title":"Test","body":"Hello"}`)

	encrypted, err := EncryptPayload(payload, clientP256DH, clientAuth)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Verify format: salt(16) + rs(4) + idlen(1) + keyid(65) + ciphertext
	if len(encrypted) < 16+4+1+65+1 {
		t.Fatalf("encrypted too short: %d", len(encrypted))
	}

	// Salt should be random
	// Record size should be 4096
	rs := uint32(encrypted[16])<<24 | uint32(encrypted[17])<<16 | uint32(encrypted[18])<<8 | uint32(encrypted[19])
	if rs != 4096 {
		t.Errorf("record size = %d, want 4096", rs)
	}

	// Key ID length should be 65
	if encrypted[20] != 65 {
		t.Errorf("keyid len = %d, want 65", encrypted[20])
	}

	// Key ID should start with 0x04 (uncompressed point)
	if encrypted[21] != 0x04 {
		t.Errorf("keyid[0] = 0x%02x, want 0x04", encrypted[21])
	}

	t.Logf("encrypted %d bytes: salt=%x rs=%d keyidLen=%d", len(encrypted), encrypted[:16], rs, encrypted[20])
}

func TestEncryptDecryptWithKnownValues(t *testing.T) {
	// Verify that encrypting and decrypting with our own code is consistent
	// (not a full round-trip since we can't decrypt, but at least verify no panic)
	clientP256DH := "BFiD30jh-xT1-ztVT4-JzRZUkVaC3jSJpXSpsu8uy1q86f28QIg8W2iznxdqLqdlg7nYlVru_A1FjmmTmrV31Eo"
	clientAuth := "mfbNpuEjYa06PwSg5azFcw"

	// Encrypt twice — should produce different ciphertexts (random salt)
	e1, err := EncryptPayload([]byte("test"), clientP256DH, clientAuth)
	if err != nil {
		t.Fatal(err)
	}
	e2, err := EncryptPayload([]byte("test"), clientP256DH, clientAuth)
	if err != nil {
		t.Fatal(err)
	}

	// Salts should differ
	if bytes.Equal(e1[:16], e2[:16]) {
		t.Error("salt should be random")
	}
	// But keyid lengths should be the same
	if e1[20] != e2[20] {
		t.Error("keyid len should be 65")
	}
}

func TestBase64URLDecode(t *testing.T) {
	tests := []struct {
		input    string
		wantLen  int
	}{
		{"BFiD30jh-xT1-ztVT4-JzRZUkVaC3jSJpXSpsu8uy1q86f28QIg8W2iznxdqLqdlg7nYlVru_A1FjmmTmrV31Eo", 65},
		{"mfbNpuEjYa06PwSg5azFcw", 16},
	}

	for _, tt := range tests {
		b, err := base64urlDecode(tt.input)
		if err != nil {
			t.Errorf("decode %q: %v", tt.input[:20], err)
			continue
		}
		if len(b) != tt.wantLen {
			t.Errorf("decode %q: len=%d want %d", tt.input[:20], len(b), tt.wantLen)
		}
	}
}
