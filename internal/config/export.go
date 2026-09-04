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
	"fmt"
	"strings"
)

// exportPrefix is the legacy plaintext-base64 prefix. New exports use
// exportPrefixV2 (RESULTPROXY2:) — see export_v2.go. We still accept this
// prefix on import for backward compatibility, but ImportConfig returns
// ErrLegacyPlaintext so the UI can warn the user that the source export
// was unencrypted.
const exportPrefix = "RESULTPROXY:"

// ExportableConfig is the subset of AppConfig that travels across machines.
// We intentionally omit Settings (window theme, last-used proxy, etc.) —
// those are per-device.
type ExportableConfig struct {
	Version      int          `json:"version"`
	Proxies      []ProxyEntry `json:"proxies"`
	RoutingRules RoutingRules `json:"routingRules"`
}

// ExportConfig writes a password-encrypted RESULTPROXY2: payload. The
// previous version produced a plaintext base64 string that anyone could
// decode without a password — the README claimed otherwise, so this fixes
// the discrepancy. Returns ErrPasswordTooShort if the password is shorter
// than MinPasswordLength.
func ExportConfig(cfg AppConfig, password string) (string, error) {
	export := ExportableConfig{
		Version:      1,
		Proxies:      cfg.Proxies,
		RoutingRules: cfg.RoutingRules,
	}
	return EncryptExportV2(export, password)
}

// ImportConfig dispatches by prefix:
//   - "RESULTPROXY2:" → requires password, validated via AES-GCM tag.
//   - "RESULTPROXY:"  → legacy plaintext, returned with ErrLegacyPlaintext so
//     the UI can show a warning before applying.
//   - raw base64 without prefix → treated as legacy plaintext (same warning).
//
// Pass an empty password for legacy imports; pass the user-supplied password
// for v2 imports. errors.Is(err, ErrPasswordRequired/ErrWrongPassword/
// ErrLegacyPlaintext) tells the UI what to prompt for.
func ImportConfig(input, password string) (*ExportableConfig, error) {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, exportPrefixV2) {
		return DecryptExportV2(input, password)
	}
	// Legacy / unprefixed: plaintext base64. We still parse it (some users
	// may have old backups) but surface a sentinel so the UI can warn that
	// these exports are unencrypted.
	exp, err := decodeLegacyExport(input)
	if err != nil {
		return nil, err
	}
	return exp, ErrLegacyPlaintext
}

// decodeLegacyExport parses the pre-v2 plaintext format. Kept private — new
// code must go through EncryptExportV2 / DecryptExportV2.
func decodeLegacyExport(input string) (*ExportableConfig, error) {
	input = strings.TrimPrefix(input, exportPrefix)
	data, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		data, err = base64.URLEncoding.DecodeString(input)
		if err != nil {
			return nil, fmt.Errorf("decoding base64: %w", err)
		}
	}
	var export ExportableConfig
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, fmt.Errorf("parsing import data: %w", err)
	}
	if err := validateImport(&export); err != nil {
		return nil, fmt.Errorf("validating import: %w", err)
	}
	return &export, nil
}

// MergeImport applies an imported export on top of an existing config.
// Routing rules are overwritten. Proxies are appended, skipping IDs that
// already exist locally.
func MergeImport(existing AppConfig, imported *ExportableConfig) AppConfig {
	existing.RoutingRules = imported.RoutingRules

	existingIDs := make(map[string]struct{}, len(existing.Proxies))
	for _, p := range existing.Proxies {
		existingIDs[p.ID] = struct{}{}
	}
	for _, p := range imported.Proxies {
		if _, exists := existingIDs[p.ID]; !exists {
			existing.Proxies = append(existing.Proxies, p)
		}
	}
	return existing
}

func validateImport(export *ExportableConfig) error {
	if export.Version < 1 {
		return fmt.Errorf("unsupported export version: %d", export.Version)
	}
	for i, p := range export.Proxies {
		if p.IP == "" && p.URI == "" {
			return fmt.Errorf("proxy %d: must have IP or URI", i)
		}
		if p.Port < 0 || p.Port > 65535 {
			return fmt.Errorf("proxy %d: invalid port %d", i, p.Port)
		}
	}
	validModes := map[string]bool{"global": true, "whitelist": true, "smart": true}
	if export.RoutingRules.Mode != "" && !validModes[export.RoutingRules.Mode] {
		return fmt.Errorf("invalid routing mode: %q", export.RoutingRules.Mode)
	}
	return nil
}

// Compile-time check: keep errors import alive even if validation code below
// shrinks. Avoids "imported and not used" if we ever trim the file.
var _ = errors.New
