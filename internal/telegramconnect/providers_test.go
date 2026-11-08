package telegramconnect

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

func TestTelegramVerifierAcceptsRSAKeyFromMixedJWKS(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signingKey := jose.JSONWebKey{Key: key, KeyID: "rsa-fixture", Algorithm: "RS256"}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: signingKey}, nil)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := json.Marshal(signingKey.Public())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string][]json.RawMessage{"keys": {
			publicKey,
			json.RawMessage(`{"kty":"EC","alg":"ES256K","crv":"secp256k1","kid":"ec-fixture","x":"AQ","y":"AQ"}`),
		}})
	}))
	defer server.Close()

	nonce := newCode()
	claims := map[string]any{
		"iss": telegramIssuer, "aud": "123", "sub": "fixture",
		"nonce": nonce, "id": int64(987654321), "exp": time.Now().Add(time.Minute).Unix(),
	}
	sign := func() string {
		t.Helper()
		payload, err := json.Marshal(claims)
		if err != nil {
			t.Fatal(err)
		}
		signed, err := signer.Sign(payload)
		if err != nil {
			t.Fatal(err)
		}
		token, err := signed.CompactSerialize()
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	verify := telegramVerifier(123, server.URL)
	token := sign()
	if id, err := verify(t.Context(), token, nonce); err != nil || id != 987654321 {
		t.Fatalf("valid signed token rejected: id %d, error %v", id, err)
	}
	if _, err := verify(t.Context(), token, newCode()); !errors.Is(err, ErrAuth) {
		t.Fatal("token from a different login was accepted")
	}
	forged := strings.Split(token, ".")
	forged[2] = strings.Repeat("A", len(forged[2]))
	if _, err := verify(t.Context(), strings.Join(forged, "."), nonce); !errors.Is(err, ErrAuth) {
		t.Fatal("forged signature was accepted")
	}
	claims["exp"] = time.Now().Add(-time.Minute).Unix()
	if _, err := verify(t.Context(), sign(), nonce); !errors.Is(err, ErrAuth) {
		t.Fatal("expired token was accepted")
	}
}
