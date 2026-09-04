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
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const testPassword = "correct horse battery staple"

func sampleConfig() AppConfig {
	return AppConfig{
		Proxies: []ProxyEntry{
			{ID: "1", IP: "1.2.3.4", Port: 8080, Type: "http", Name: "Server 1", Password: "secret1"},
			{ID: "2", IP: "5.6.7.8", Port: 1080, Type: "socks5", Name: "Server 2"},
		},
		RoutingRules: RoutingRules{
			Mode:         "smart",
			Whitelist:    []string{"example.com"},
			AppWhitelist: []string{"chrome.exe"},
		},
	}
}

func TestExportImportRoundTripV2(t *testing.T) {
	exported, err := ExportConfig(sampleConfig(), testPassword)
	if err != nil {
		t.Fatalf("ExportConfig failed: %v", err)
	}
	if !strings.HasPrefix(exported, exportPrefixV2) {
		t.Fatalf("exported string should start with %q, got %q", exportPrefixV2, exported[:min(20, len(exported))])
	}

	imported, err := ImportConfig(exported, testPassword)
	if err != nil {
		t.Fatalf("ImportConfig failed: %v", err)
	}
	if imported.Version != 1 {
		t.Errorf("Version: got %d, want 1", imported.Version)
	}
	if len(imported.Proxies) != 2 {
		t.Fatalf("expected 2 proxies, got %d", len(imported.Proxies))
	}
	if imported.Proxies[0].IP != "1.2.3.4" {
		t.Errorf("Proxy[0].IP: got %q, want '1.2.3.4'", imported.Proxies[0].IP)
	}
	if imported.Proxies[0].Password != "secret1" {
		t.Errorf("Proxy[0].Password: got %q, want 'secret1'", imported.Proxies[0].Password)
	}
	if imported.RoutingRules.Mode != "smart" {
		t.Errorf("RoutingRules.Mode: got %q, want 'smart'", imported.RoutingRules.Mode)
	}
}

func TestExportWrongPassword(t *testing.T) {
	exported, err := ExportConfig(sampleConfig(), testPassword)
	if err != nil {
		t.Fatalf("ExportConfig failed: %v", err)
	}
	_, err = ImportConfig(exported, "obviously-wrong-pw")
	if !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("expected ErrWrongPassword, got %v", err)
	}
}

func TestExportRequiresPasswordOnImport(t *testing.T) {
	exported, err := ExportConfig(sampleConfig(), testPassword)
	if err != nil {
		t.Fatalf("ExportConfig failed: %v", err)
	}
	_, err = ImportConfig(exported, "")
	if !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("expected ErrPasswordRequired, got %v", err)
	}
}

func TestExportPasswordTooShort(t *testing.T) {
	_, err := ExportConfig(sampleConfig(), "short")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
}

// Each export must use a fresh salt+nonce — repeating the same call must not
// produce the same ciphertext, otherwise an attacker who sees two exports can
// confirm the plaintext is identical.
func TestExportNonDeterministic(t *testing.T) {
	a, err := ExportConfig(sampleConfig(), testPassword)
	if err != nil {
		t.Fatalf("first export: %v", err)
	}
	b, err := ExportConfig(sampleConfig(), testPassword)
	if err != nil {
		t.Fatalf("second export: %v", err)
	}
	if a == b {
		t.Fatal("two consecutive exports produced identical output (salt/nonce reuse)")
	}
}

// Tamper-detection: flipping bits in the ciphertext must produce a
// password-mismatch error, not a silent decryption with garbage plaintext.
func TestExportTamperDetection(t *testing.T) {
	exported, err := ExportConfig(sampleConfig(), testPassword)
	if err != nil {
		t.Fatalf("ExportConfig failed: %v", err)
	}
	body := strings.TrimPrefix(exported, exportPrefixV2)
	envJSON, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("decoding outer base64: %v", err)
	}
	var env exportEnvelopeV2
	if err := json.Unmarshal(envJSON, &env); err != nil {
		t.Fatalf("parsing envelope: %v", err)
	}
	ct, err := base64.StdEncoding.DecodeString(env.CT)
	if err != nil {
		t.Fatalf("decoding ct: %v", err)
	}
	// Flip the first ciphertext byte.
	ct[0] ^= 0xFF
	env.CT = base64.StdEncoding.EncodeToString(ct)
	patched, _ := json.Marshal(env)
	tampered := exportPrefixV2 + base64.StdEncoding.EncodeToString(patched)

	_, err = ImportConfig(tampered, testPassword)
	if !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("tampered payload should fail with ErrWrongPassword, got %v", err)
	}
}

// Downgrade attempt: setting iter=1 should be rejected even if the rest of
// the envelope decodes — we refuse weak KDF parameters supplied by an
// attacker who controls the envelope JSON.
func TestExportRejectsWeakKDFParams(t *testing.T) {
	env := exportEnvelopeV2{
		Version: 2,
		KDF:     "pbkdf2-sha256",
		Iter:    1, // far below 100k threshold
		Salt:    base64.StdEncoding.EncodeToString(make([]byte, 16)),
		Nonce:   base64.StdEncoding.EncodeToString(make([]byte, 12)),
		CT:      base64.StdEncoding.EncodeToString(make([]byte, 32)),
	}
	envJSON, _ := json.Marshal(env)
	weak := exportPrefixV2 + base64.StdEncoding.EncodeToString(envJSON)

	_, err := ImportConfig(weak, testPassword)
	if !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("expected ErrInvalidEnvelope for weak iter, got %v", err)
	}
}

// Legacy plaintext import: still accepted (for old backups) but surfaced with
// ErrLegacyPlaintext so the UI can warn the user.
func TestLegacyPlaintextImportSurfacesWarning(t *testing.T) {
	cfg := sampleConfig()
	export := ExportableConfig{
		Version:      1,
		Proxies:      cfg.Proxies,
		RoutingRules: cfg.RoutingRules,
	}
	data, _ := json.Marshal(export)
	legacy := exportPrefix + base64.StdEncoding.EncodeToString(data)

	imported, err := ImportConfig(legacy, "")
	if !errors.Is(err, ErrLegacyPlaintext) {
		t.Fatalf("expected ErrLegacyPlaintext, got %v", err)
	}
	if imported == nil {
		t.Fatal("imported config should be non-nil alongside the warning")
	}
	if len(imported.Proxies) != 2 {
		t.Errorf("expected 2 proxies from legacy import, got %d", len(imported.Proxies))
	}
}

func TestImportInvalidBase64(t *testing.T) {
	_, err := ImportConfig("not-valid-base64!!!", "")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestImportInvalidJSON(t *testing.T) {
	_, err := ImportConfig("RESULTPROXY:bm90IGpzb24=", "")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestImportValidation(t *testing.T) {
	cases := []struct {
		name    string
		export  ExportableConfig
		wantErr bool
	}{
		{
			name:    "valid",
			export:  ExportableConfig{Version: 1, Proxies: []ProxyEntry{{IP: "1.2.3.4", Port: 80}}},
			wantErr: false,
		},
		{
			name:    "invalid version",
			export:  ExportableConfig{Version: 0},
			wantErr: true,
		},
		{
			name:    "invalid port",
			export:  ExportableConfig{Version: 1, Proxies: []ProxyEntry{{IP: "1.2.3.4", Port: 99999}}},
			wantErr: true,
		},
		{
			name:    "empty proxy",
			export:  ExportableConfig{Version: 1, Proxies: []ProxyEntry{{Port: 80}}},
			wantErr: true,
		},
		{
			name:    "invalid mode",
			export:  ExportableConfig{Version: 1, RoutingRules: RoutingRules{Mode: "invalid"}},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateImport(&tc.export)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateImport() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestMergeImport(t *testing.T) {
	existing := AppConfig{
		Proxies: []ProxyEntry{
			{ID: "1", IP: "1.1.1.1", Port: 80, Name: "Existing"},
		},
		RoutingRules: RoutingRules{Mode: "global"},
		Settings:     AppSettings{Mode: "proxy", Theme: "dark"},
	}

	imported := &ExportableConfig{
		Version: 1,
		Proxies: []ProxyEntry{
			{ID: "1", IP: "1.1.1.1", Port: 80, Name: "Duplicate"},
			{ID: "2", IP: "2.2.2.2", Port: 1080, Name: "New"},
		},
		RoutingRules: RoutingRules{Mode: "smart", Whitelist: []string{"new.com"}},
	}

	result := MergeImport(existing, imported)

	if result.RoutingRules.Mode != "smart" {
		t.Errorf("RoutingRules.Mode: got %q, want 'smart'", result.RoutingRules.Mode)
	}
	if len(result.Proxies) != 2 {
		t.Fatalf("expected 2 proxies after merge, got %d", len(result.Proxies))
	}
	if result.Settings.Theme != "dark" {
		t.Errorf("Settings.Theme should be 'dark', got %q", result.Settings.Theme)
	}
}
