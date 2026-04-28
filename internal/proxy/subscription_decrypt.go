package proxy

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var subscriptionEncryptKey string

const subscriptionMagic = "RVSUB1:"

var utf8BOM = string(rune(0xFEFF))

// ErrSubscriptionKeyMissing means the body was encrypted with RVSUB1 but this
// build has no decryption key compiled in.
var ErrSubscriptionKeyMissing = errors.New("encrypted subscription requires a decryption key that is not present in this build")

// IsEncryptedSubscription reports whether s carries the RVSUB1 magic prefix.
func IsEncryptedSubscription(s string) bool {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, utf8BOM)
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, subscriptionMagic)
}

// DecryptSubscription unwraps an RVSUB1-prefixed payload. For non-encrypted
// input it returns the trimmed value unchanged.
func DecryptSubscription(body string) (string, error) {
	return tryDecryptSubscription(body)
}

// tryDecryptSubscription attempts to decrypt an RVSUB1-prefixed payload. Returns
// the plaintext when successful, or a specific error so callers can surface a
// meaningful message to the user instead of a generic "unsupported format".
func tryDecryptSubscription(body string) (string, error) {
	body = strings.TrimSpace(body)
	body = strings.TrimPrefix(body, utf8BOM)
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(body, subscriptionMagic) {
		return body, nil
	}
	if subscriptionEncryptKey == "" {
		return "", ErrSubscriptionKeyMissing
	}
	keyHex := strings.TrimSpace(subscriptionEncryptKey)
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", fmt.Errorf("decryption key is not valid hex: %w", err)
	}
	if len(keyBytes) != 32 {
		return "", fmt.Errorf("decryption key must be 32 bytes (got %d)", len(keyBytes))
	}

	encoded := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, body[len(subscriptionMagic):])

	data, err := decodeBase64Flexible(encoded)
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("aes cipher init failed: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm init failed: %w", err)
	}

	if len(data) < gcm.NonceSize()+gcm.Overhead() {
		return "", fmt.Errorf("ciphertext too short (%d bytes, need at least %d)", len(data), gcm.NonceSize()+gcm.Overhead())
	}

	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("gcm open failed (wrong key or corrupted payload): %w", err)
	}
	return string(plaintext), nil
}

// decodeBase64Flexible tries standard, URL-safe, and raw (no-padding) variants.
func decodeBase64Flexible(s string) ([]byte, error) {
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	}
	var lastErr error
	for _, enc := range encodings {
		if data, err := enc.DecodeString(s); err == nil {
			return data, nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no base64 variant matched")
	}
	return nil, lastErr
}
