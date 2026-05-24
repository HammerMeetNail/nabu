package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// EncryptPayload encrypts a Web Push payload using RFC 8291 aes128gcm.
// It accepts the client's base64url-encoded p256dh and auth strings.
func EncryptPayload(payload []byte, clientP256DH, clientAuth string) ([]byte, error) {
	clientPubKey, err := base64urlDecode(clientP256DH)
	if err != nil {
		return nil, fmt.Errorf("decode p256dh: %w", err)
	}
	authSecret, err := base64urlDecode(clientAuth)
	if err != nil {
		return nil, fmt.Errorf("decode auth: %w", err)
	}

	// Generate ephemeral P-256 key pair
	curve := ecdh.P256()
	ephemeralPriv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ephemeral key: %w", err)
	}
	ephemeralPub := ephemeralPriv.PublicKey()

	clientPub, err := curve.NewPublicKey(clientPubKey)
	if err != nil {
		return nil, fmt.Errorf("parse client public key: %w", err)
	}

	sharedSecret, err := ephemeralPriv.ECDH(clientPub)
	if err != nil {
		return nil, fmt.Errorf("ecdh: %w", err)
	}

	// Generate salt
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	// Derive key and nonce via HKDF (RFC 8291)
	prk := hkdf.Extract(sha256.New, sharedSecret, authSecret)
	info := buildInfo(ephemeralPub.Bytes(), clientPubKey)
	kdf := hkdf.Expand(sha256.New, prk, info)

	cek := make([]byte, 16) // content encryption key
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(kdf, cek); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(kdf, nonce); err != nil {
		return nil, err
	}

	// AES-128-GCM
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Add padding — the minimum overhead is 1 byte of padding length + delimiter
	// For simplicity, pad to nearest block or add minimal padding.
	// RFC 8291 requires the padding length to be encoded at the end.
	// We use zero padding for simplicity (padding length = 0, so delimiter = 0x02).
	// Actually, the payload is: plaintext || padding || delimiter
	// delimiter is 0x02 (last record) or 0x01 (intermediate record)
	// padding length = 0 → [0x02]
	paddedPayload := append([]byte(nil), payload...)
	paddedPayload = append(paddedPayload, 0x02)

	ciphertext := aead.Seal(nil, nonce, paddedPayload, nil)

	// Build aes128gcm payload:
	// salt (16) + rs (4, uint32be) + idlen (1) + keyid (idlen bytes) + ciphertext
	recordSize := uint32(4096)
	idLen := uint8(len(ephemeralPub.Bytes()))

	result := make([]byte, 0, 16+4+1+len(ephemeralPub.Bytes())+len(ciphertext))
	result = append(result, salt...)
	rsb := make([]byte, 4)
	binary.BigEndian.PutUint32(rsb, recordSize)
	result = append(result, rsb...)
	result = append(result, idLen)
	result = append(result, ephemeralPub.Bytes()...)
	result = append(result, ciphertext...)

	return result, nil
}

// buildInfo creates the HKDF info string per RFC 8291 section 3.3.
func buildInfo(ephemeralPub, clientPubKey []byte) []byte {
	// "WebPush: info\0" || receiver public key || sender public key
	info := []byte("WebPush: info\x00")
	info = append(info, clientPubKey...)
	info = append(info, ephemeralPub...)
	return info
}
