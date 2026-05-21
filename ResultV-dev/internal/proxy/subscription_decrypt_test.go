package proxy

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func encryptForTest(t *testing.T, keyHex, plaintext string) string {
	t.Helper()
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		t.Fatalf("hex decode key: %v", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("aes cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("gcm: %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("nonce: %v", err)
	}
	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return subscriptionMagic + base64.StdEncoding.EncodeToString(append(nonce, ct...))
}

func TestTryDecryptSubscription_RoundTrip(t *testing.T) {
	keyHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	prev := subscriptionEncryptKey
	subscriptionEncryptKey = keyHex
	defer func() { subscriptionEncryptKey = prev }()

	plain := "vless://abc@host:443?encryption=none\nss://xxx@host:8388\n"
	encoded := encryptForTest(t, keyHex, plain)

	got, err := tryDecryptSubscription(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != plain {
		t.Fatalf("mismatch: got %q want %q", got, plain)
	}
}

func TestTryDecryptSubscription_PlainPassthrough(t *testing.T) {
	prev := subscriptionEncryptKey
	subscriptionEncryptKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	defer func() { subscriptionEncryptKey = prev }()

	plain := "vless://abc@host:443"
	got, err := tryDecryptSubscription(plain)
	if err != nil {
		t.Fatalf("unexpected error on plain body: %v", err)
	}
	if got != plain {
		t.Fatalf("plain body mutated: %q", got)
	}
}

func TestTryDecryptSubscription_MissingKey(t *testing.T) {
	prev := subscriptionEncryptKey
	subscriptionEncryptKey = ""
	defer func() { subscriptionEncryptKey = prev }()

	_, err := tryDecryptSubscription(subscriptionMagic + "AAAAAAAAAAAAAAAA")
	if !errors.Is(err, ErrSubscriptionKeyMissing) {
		t.Fatalf("expected ErrSubscriptionKeyMissing, got %v", err)
	}
}

func TestTryDecryptSubscription_BadKey(t *testing.T) {
	prev := subscriptionEncryptKey
	// wrong key, same length
	subscriptionEncryptKey = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	defer func() { subscriptionEncryptKey = prev }()

	encoded := encryptForTest(t, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "hello")
	_, err := tryDecryptSubscription(encoded)
	if err == nil || !strings.Contains(err.Error(), "gcm open failed") {
		t.Fatalf("expected gcm open failure, got %v", err)
	}
}

func TestTryDecryptSubscription_URLSafeBase64(t *testing.T) {
	keyHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	prev := subscriptionEncryptKey
	subscriptionEncryptKey = keyHex
	defer func() { subscriptionEncryptKey = prev }()

	plain := "ss://test@host:443"
	key, _ := hex.DecodeString(keyHex)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	_, _ = rand.Read(nonce)
	ct := gcm.Seal(nil, nonce, []byte(plain), nil)
	urlSafe := subscriptionMagic + base64.URLEncoding.EncodeToString(append(nonce, ct...))

	got, err := tryDecryptSubscription(urlSafe)
	if err != nil {
		t.Fatalf("URL-safe decode failed: %v", err)
	}
	if got != plain {
		t.Fatalf("mismatch: got %q want %q", got, plain)
	}
}

func TestTryDecryptSubscription_WhitespaceTolerant(t *testing.T) {
	keyHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	prev := subscriptionEncryptKey
	subscriptionEncryptKey = keyHex
	defer func() { subscriptionEncryptKey = prev }()

	plain := "ss://test@host:443"
	encoded := encryptForTest(t, keyHex, plain)
	// Inject whitespace mid-base64 (common from clipboards/line wraps).
	idx := len(encoded) / 2
	withWhitespace := encoded[:idx] + "\r\n  \t" + encoded[idx:]

	got, err := tryDecryptSubscription(withWhitespace)
	if err != nil {
		t.Fatalf("whitespace-injected decode failed: %v", err)
	}
	if got != plain {
		t.Fatalf("mismatch: got %q want %q", got, plain)
	}
}
