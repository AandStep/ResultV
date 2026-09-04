// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

// exportPrefixV2 marks the new authenticated, password-encrypted export
// format. The legacy "RESULTPROXY:" prefix (plaintext base64) is still parsed
// for import but no longer produced by ExportConfig.
const exportPrefixV2 = "RESULTPROXY2:"

// Tunable parameters of the export KDF. OWASP 2023 recommends >= 600k
// iterations for PBKDF2-HMAC-SHA256. Salt is 16 bytes (per export), nonce is
// 12 bytes (standard GCM). Changing these values does not break old payloads
// — iter/salt/nonce/kdf are stored alongside the ciphertext.
const (
	pbkdf2Iterations = 600_000
	pbkdf2SaltLen    = 16
	gcmNonceLen      = 12
	derivedKeyLen    = 32 // AES-256
)

// MinPasswordLength is the minimum length enforced for export passwords. UI
// can enforce stricter checks (zxcvbn, etc.) but this is the floor.
const MinPasswordLength = 8

// exportEnvelopeV2 is the on-disk JSON shape inside a RESULTPROXY2: payload.
// Field names are short to keep the resulting string compact when transferred
// over chat / QR.
type exportEnvelopeV2 struct {
	Version int    `json:"v"`
	KDF     string `json:"kdf"`
	Iter    int    `json:"iter"`
	Salt    string `json:"salt"`
	Nonce   string `json:"nonce"`
	CT      string `json:"ct"`
}

// Sentinel errors. Callers should rely on errors.Is() for UI dispatch:
//   - ErrPasswordRequired:   UI must prompt for password
//   - ErrWrongPassword:      UI must show "wrong password, try again"
//   - ErrLegacyPlaintext:    UI must warn that the import is from an
//                            unencrypted export and offer to proceed
//   - ErrPasswordTooShort:   UI must explain min length
var (
	ErrPasswordRequired = errors.New("export password is required")
	ErrWrongPassword    = errors.New("wrong export password or corrupted payload")
	ErrLegacyPlaintext  = errors.New("import payload is from an older unencrypted export")
	ErrPasswordTooShort = fmt.Errorf("export password must be at least %d characters", MinPasswordLength)
	ErrUnsupportedKDF   = errors.New("unsupported export KDF")
	ErrInvalidEnvelope  = errors.New("invalid export envelope")
)

// EncryptExportV2 produces a RESULTPROXY2: payload encrypted with a key
// derived from the user-supplied password. Each call uses fresh random salt
// and nonce — the output is non-deterministic, which is a property the tests
// rely on.
func EncryptExportV2(cfg ExportableConfig, password string) (string, error) {
	if len(password) < MinPasswordLength {
		return "", ErrPasswordTooShort
	}

	plaintext, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshaling export: %w", err)
	}

	salt := make([]byte, pbkdf2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}
	nonce := make([]byte, gcmNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	key := pbkdf2.Key([]byte(password), salt, pbkdf2Iterations, derivedKeyLen, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm init: %w", err)
	}

	// AAD = the literal prefix. Authenticating it prevents an attacker from
	// re-wrapping ciphertext under a different format prefix (e.g. forging a
	// "RESULTPROXY:" header to bypass password validation).
	aad := []byte(strings.TrimSuffix(exportPrefixV2, ":"))
	sealed := gcm.Seal(nil, nonce, plaintext, aad)

	env := exportEnvelopeV2{
		Version: 2,
		KDF:     "pbkdf2-sha256",
		Iter:    pbkdf2Iterations,
		Salt:    base64.StdEncoding.EncodeToString(salt),
		Nonce:   base64.StdEncoding.EncodeToString(nonce),
		CT:      base64.StdEncoding.EncodeToString(sealed),
	}
	envJSON, err := json.Marshal(env)
	if err != nil {
		return "", fmt.Errorf("marshaling envelope: %w", err)
	}
	return exportPrefixV2 + base64.StdEncoding.EncodeToString(envJSON), nil
}

// DecryptExportV2 reverses EncryptExportV2. Passwords are validated by GCM —
// a wrong password produces a tag mismatch that we map to ErrWrongPassword.
// Caller is expected to have already checked the prefix.
func DecryptExportV2(payload, password string) (*ExportableConfig, error) {
	if password == "" {
		return nil, ErrPasswordRequired
	}
	body := strings.TrimSpace(strings.TrimPrefix(payload, exportPrefixV2))
	envJSON, err := decodeBase64Flexible(body)
	if err != nil {
		return nil, fmt.Errorf("%w: base64 decode failed: %v", ErrInvalidEnvelope, err)
	}
	var env exportEnvelopeV2
	if err := json.Unmarshal(envJSON, &env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidEnvelope, err)
	}
	if env.Version != 2 {
		return nil, fmt.Errorf("%w: unsupported envelope version %d", ErrInvalidEnvelope, env.Version)
	}
	if env.KDF != "pbkdf2-sha256" {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedKDF, env.KDF)
	}
	if env.Iter < 100_000 {
		// Refuse weak parameters even if the envelope claims them. Stops
		// downgrade attacks where a tampered envelope sets iter=1.
		return nil, fmt.Errorf("%w: iteration count too low: %d", ErrInvalidEnvelope, env.Iter)
	}

	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return nil, fmt.Errorf("%w: salt b64: %v", ErrInvalidEnvelope, err)
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, fmt.Errorf("%w: nonce b64: %v", ErrInvalidEnvelope, err)
	}
	ct, err := base64.StdEncoding.DecodeString(env.CT)
	if err != nil {
		return nil, fmt.Errorf("%w: ct b64: %v", ErrInvalidEnvelope, err)
	}
	if len(nonce) != gcmNonceLen {
		return nil, fmt.Errorf("%w: nonce length %d", ErrInvalidEnvelope, len(nonce))
	}

	key := pbkdf2.Key([]byte(password), salt, env.Iter, derivedKeyLen, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm init: %w", err)
	}
	aad := []byte(strings.TrimSuffix(exportPrefixV2, ":"))
	plaintext, err := gcm.Open(nil, nonce, ct, aad)
	if err != nil {
		// GCM doesn't distinguish "wrong password" from "tampered payload" —
		// both produce the same tag mismatch. Surface as wrong-password since
		// that's the overwhelmingly common cause and the user-actionable one.
		return nil, ErrWrongPassword
	}

	var exp ExportableConfig
	if err := json.Unmarshal(plaintext, &exp); err != nil {
		return nil, fmt.Errorf("%w: plaintext is not valid JSON: %v", ErrInvalidEnvelope, err)
	}
	if err := validateImport(&exp); err != nil {
		return nil, fmt.Errorf("validating import: %w", err)
	}
	return &exp, nil
}

// decodeBase64Flexible mirrors the helper in subscription_decrypt.go but lives
// here to keep the config package self-contained. Tries Std/URL with and
// without padding.
func decodeBase64Flexible(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
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
