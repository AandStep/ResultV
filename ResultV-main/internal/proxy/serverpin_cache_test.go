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

package proxy

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeCodec stands in for config.CryptoService: it round-trips through base64
// so the on-disk bytes are NOT readable plaintext (mirroring "encrypted at
// rest") while staying deterministic for tests.
type fakeCodec struct{}

func (fakeCodec) Encrypt(data any) (string, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func (fakeCodec) DecryptInto(s string, dst any) error {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func TestServerPin_SaveLoadClearRoundTrip(t *testing.T) {
	dir := t.TempDir()
	codec := fakeCodec{}

	if got := loadServerPin(dir, codec, "k.sunsetglow.today"); got != "" {
		t.Fatalf("expected empty before save, got %q", got)
	}

	saveServerPin(dir, codec, "k.sunsetglow.today", "203.0.113.7")
	if got := loadServerPin(dir, codec, "k.sunsetglow.today"); got != "203.0.113.7" {
		t.Fatalf("expected 203.0.113.7, got %q", got)
	}

	// Key is case-insensitive on the host.
	if got := loadServerPin(dir, codec, "K.SunsetGlow.Today"); got != "203.0.113.7" {
		t.Fatalf("expected case-insensitive hit, got %q", got)
	}

	clearServerPin(dir, codec, "k.sunsetglow.today")
	if got := loadServerPin(dir, codec, "k.sunsetglow.today"); got != "" {
		t.Fatalf("expected empty after clear, got %q", got)
	}
}

// TestServerPin_FileIsNotPlaintext is the security regression guard: the
// hostname and IP must never appear verbatim in the on-disk file.
func TestServerPin_FileIsNotPlaintext(t *testing.T) {
	dir := t.TempDir()
	saveServerPin(dir, fakeCodec{}, "k.sunsetglow.today", "152.53.33.10")

	raw, err := os.ReadFile(filepath.Join(dir, serverPinCacheFile))
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	if strings.Contains(string(raw), "sunsetglow") || strings.Contains(string(raw), "152.53.33.10") {
		t.Fatalf("cache file leaks plaintext server data: %s", raw)
	}
}

// TestServerPin_NilCodecNeverPersists guarantees no plaintext fallback: with no
// codec we must not create the file at all.
func TestServerPin_NilCodecNeverPersists(t *testing.T) {
	dir := t.TempDir()
	saveServerPin(dir, nil, "host.example", "203.0.113.7")
	if _, err := os.Stat(filepath.Join(dir, serverPinCacheFile)); !os.IsNotExist(err) {
		t.Fatalf("expected no file written with nil codec, stat err=%v", err)
	}
	if got := loadServerPin(dir, nil, "host.example"); got != "" {
		t.Fatalf("expected empty with nil codec, got %q", got)
	}
}

// TestServerPin_DropsUndecryptablePlaintextLeftover covers migration from the
// old plaintext file: it can't be decrypted, so it must be removed and ignored.
func TestServerPin_DropsUndecryptablePlaintextLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, serverPinCacheFile)
	if err := os.WriteFile(path, []byte(`{"k.sunsetglow.today":"152.53.33.10"}`), 0o600); err != nil {
		t.Fatalf("seed plaintext: %v", err)
	}
	if got := loadServerPin(dir, fakeCodec{}, "k.sunsetglow.today"); got != "" {
		t.Fatalf("expected plaintext leftover ignored, got %q", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected plaintext leftover removed, stat err=%v", err)
	}
}

func TestServerPin_RejectsNonIPValues(t *testing.T) {
	dir := t.TempDir()
	saveServerPin(dir, fakeCodec{}, "host.example", "not-an-ip")
	if got := loadServerPin(dir, fakeCodec{}, "host.example"); got != "" {
		t.Fatalf("expected non-IP save to be rejected, got %q", got)
	}
}

func TestServerPin_OverwriteReplacesValue(t *testing.T) {
	dir := t.TempDir()
	codec := fakeCodec{}
	saveServerPin(dir, codec, "host.example", "203.0.113.7")
	saveServerPin(dir, codec, "host.example", "198.51.100.9")
	if got := loadServerPin(dir, codec, "host.example"); got != "198.51.100.9" {
		t.Fatalf("expected overwrite to 198.51.100.9, got %q", got)
	}
}
