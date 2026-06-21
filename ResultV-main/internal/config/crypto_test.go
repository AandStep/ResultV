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
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const testMachineID = "test-machine-id-12345678"

func newTestCrypto() *CryptoService {
	return NewCryptoServiceWithID(testMachineID)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	cs := newTestCrypto()

	original := map[string]any{
		"routingRules": map[string]any{
			"mode":         "global",
			"whitelist":    []any{"localhost", "127.0.0.1"},
			"appWhitelist": []any{},
		},
	}

	encrypted, err := cs.Encrypt(original)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	
	var env secureEnvelope
	if err := json.Unmarshal([]byte(encrypted), &env); err != nil {
		t.Fatalf("encrypted output is not valid JSON: %v", err)
	}
	if !env.IsSecure {
		t.Error("_isSecure should be true")
	}
	if env.IV == "" || env.Data == "" || env.AuthTag == "" {
		t.Error("envelope fields should not be empty")
	}

	
	raw, err := cs.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("Unmarshal decrypted data failed: %v", err)
	}

	rules, ok := result["routingRules"].(map[string]any)
	if !ok {
		t.Fatal("routingRules not found or wrong type")
	}
	if rules["mode"] != "global" {
		t.Errorf("expected mode 'global', got %v", rules["mode"])
	}
}

func TestDecryptInto(t *testing.T) {
	cs := newTestCrypto()

	type Config struct {
		Name string `json:"name"`
		Port int    `json:"port"`
	}
	original := Config{Name: "test-proxy", Port: 14081}

	encrypted, err := cs.Encrypt(original)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	var result Config
	if err := cs.DecryptInto(encrypted, &result); err != nil {
		t.Fatalf("DecryptInto failed: %v", err)
	}

	if result.Name != original.Name {
		t.Errorf("Name: got %q, want %q", result.Name, original.Name)
	}
	if result.Port != original.Port {
		t.Errorf("Port: got %d, want %d", result.Port, original.Port)
	}
}

func TestDecryptLegacyUnencrypted(t *testing.T) {
	cs := newTestCrypto()

	
	legacy := `{"routingRules":{"mode":"global"}}`

	raw, err := cs.Decrypt(legacy)
	if err != nil {
		t.Fatalf("Decrypt legacy failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	rules := result["routingRules"].(map[string]any)
	if rules["mode"] != "global" {
		t.Errorf("expected mode 'global', got %v", rules["mode"])
	}
}

func TestDecryptWrongKey(t *testing.T) {
	cs1 := NewCryptoServiceWithID("machine-A")
	cs2 := NewCryptoServiceWithID("machine-B")

	encrypted, err := cs1.Encrypt(map[string]string{"secret": "data"})
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = cs2.Decrypt(encrypted)
	if err == nil {
		t.Error("expected decryption to fail with wrong key")
	}
}

func TestNodeJSCompatibility(t *testing.T) {
	
	
	
	
	
	
	
	
	
	
	
	
	
	t.Skip("TODO: populate with actual Node.js encrypted output for cross-compat validation")
}

// NewCryptoService must generate an install-salt file on first use and
// derive a key that differs from the legacy sha256(machineID+keySalt).
// Without this, the migration to per-install salt would be silently a
// no-op — the new key would equal the legacy key on a fresh install.
func TestNewCryptoServiceGeneratesInstallSalt(t *testing.T) {
	dir := t.TempDir()
	cs, err := NewCryptoService(dir)
	if err != nil {
		t.Fatalf("NewCryptoService: %v", err)
	}
	saltPath := filepath.Join(dir, installSaltFile)
	if _, err := os.Stat(saltPath); err != nil {
		t.Fatalf("expected install-salt file at %s, got %v", saltPath, err)
	}

	// Derive what the legacy scheme would produce for the same machineID.
	// We don't know the actual machineID here — NewCryptoService resolved
	// it from OS APIs / fallback file — so reuse the legacyKey the service
	// stored. The point: legacyKey != key, otherwise no migration value.
	if cs.key == cs.legacyKey {
		t.Fatal("expected key to differ from legacyKey after salt derivation")
	}
}

// Subsequent calls must read the existing salt rather than overwriting it,
// otherwise every restart would invalidate all previously-encrypted configs.
func TestNewCryptoServiceReusesExistingSalt(t *testing.T) {
	dir := t.TempDir()
	cs1, err := NewCryptoService(dir)
	if err != nil {
		t.Fatalf("first NewCryptoService: %v", err)
	}
	cs2, err := NewCryptoService(dir)
	if err != nil {
		t.Fatalf("second NewCryptoService: %v", err)
	}
	if cs1.key != cs2.key {
		t.Fatal("salt rotated across calls — config would be unreadable on next launch")
	}
}

// Wiping the salt file should produce a NEW key (otherwise the salt did
// nothing). Existing legacy-encrypted configs are still openable through
// the legacy fallback, so this is safe.
func TestDeletingSaltFileRotatesKey(t *testing.T) {
	dir := t.TempDir()
	cs1, err := NewCryptoService(dir)
	if err != nil {
		t.Fatalf("first NewCryptoService: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, installSaltFile)); err != nil {
		t.Fatalf("remove salt: %v", err)
	}
	cs2, err := NewCryptoService(dir)
	if err != nil {
		t.Fatalf("second NewCryptoService: %v", err)
	}
	if cs1.key == cs2.key {
		t.Fatal("removing the salt file should have produced a different key")
	}
}

// Configs encrypted by older builds (sha256(machineID + keySalt), no salt)
// must still open on a build that introduced PBKDF2 + install-salt. The
// migration is signalled via needsReencrypt.
func TestDecryptFallsBackToLegacyKey(t *testing.T) {
	machineID := "migration-test-machine"
	legacyKey := sha256.Sum256([]byte(machineID + keySalt))

	// Simulate an older build: encrypt under legacyKey only.
	legacyCS := &CryptoService{key: legacyKey, legacyKey: legacyKey}
	payload, err := legacyCS.Encrypt(map[string]string{"hello": "world"})
	if err != nil {
		t.Fatalf("legacy encrypt: %v", err)
	}

	// New build: different primary key, same legacyKey. Decrypt should
	// succeed via the fallback and set the migration flag.
	newCS := &CryptoService{
		legacyKey: legacyKey,
	}
	// Construct a definitely-different primary key.
	newCS.key = sha256.Sum256([]byte("a totally unrelated salt"))

	raw, err := newCS.Decrypt(payload)
	if err != nil {
		t.Fatalf("expected legacy fallback to succeed, got %v", err)
	}
	if !newCS.NeedsReencrypt() {
		t.Fatal("legacy fallback did not set needsReencrypt")
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal plaintext: %v", err)
	}
	if got["hello"] != "world" {
		t.Fatalf("plaintext mismatch: %v", got)
	}

	// After ClearReencryptFlag the flag must stay cleared even though the
	// CryptoService can still decrypt with legacy (which would otherwise
	// re-set the flag on the next Decrypt call).
	newCS.ClearReencryptFlag()
	if newCS.NeedsReencrypt() {
		t.Fatal("ClearReencryptFlag did not clear the flag")
	}
}

// Wrong-key path: when neither current nor legacy can open the ciphertext
// we must return an error, not silently produce garbage plaintext.
func TestDecryptFailsWhenBothKeysWrong(t *testing.T) {
	srcKey := sha256.Sum256([]byte("source-key"))
	srcCS := &CryptoService{key: srcKey, legacyKey: srcKey}
	payload, err := srcCS.Encrypt(map[string]string{"x": "y"})
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	dst := &CryptoService{
		key:       sha256.Sum256([]byte("wrong-key-1")),
		legacyKey: sha256.Sum256([]byte("wrong-key-2")),
	}
	if _, err := dst.Decrypt(payload); err == nil {
		t.Fatal("expected decryption to fail with both wrong keys")
	}
	if dst.NeedsReencrypt() {
		t.Fatal("failed decrypt must not set needsReencrypt")
	}
}

func TestEncryptProducesUniqueIVs(t *testing.T) {
	cs := newTestCrypto()
	data := map[string]string{"key": "value"}

	enc1, err := cs.Encrypt(data)
	if err != nil {
		t.Fatalf("Encrypt 1 failed: %v", err)
	}
	enc2, err := cs.Encrypt(data)
	if err != nil {
		t.Fatalf("Encrypt 2 failed: %v", err)
	}

	var env1, env2 secureEnvelope
	json.Unmarshal([]byte(enc1), &env1)
	json.Unmarshal([]byte(enc2), &env2)

	if env1.IV == env2.IV {
		t.Error("two encryptions produced the same IV — randomness failure")
	}
	if env1.Data == env2.Data {
		t.Error("two encryptions produced the same ciphertext — randomness failure")
	}
}
