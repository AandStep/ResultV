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

//go:build windows

package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const dnsSnapshotFile = "dns-snapshot.json"

type adapterDNS struct {
	InterfaceIndex  int      `json:"InterfaceIndex"`
	InterfaceAlias  string   `json:"InterfaceAlias"`
	ServerAddresses []string `json:"ServerAddresses"`
}

type dnsSnapshot struct {
	Adapters []adapterDNS `json:"adapters"`
}

type windowsSystemDNS struct {
	snapshotPath string
}

func newSystemDNS() SystemDNS {
	return &windowsSystemDNS{
		snapshotPath: filepath.Join(resultProxyDataDir(), dnsSnapshotFile),
	}
}

func (w *windowsSystemDNS) Override(servers []string) error {
	if len(servers) == 0 {
		return errors.New("system dns override: empty server list")
	}

	adapters, err := listAdapterDNS()
	if err != nil {
		return fmt.Errorf("enumerate adapter dns: %w", err)
	}

	// Persist BEFORE mutating anything. If we crash after persisting but
	// before applying, Restore() at next start is a no-op on adapters that
	// were never touched. If we crash mid-apply, Restore reads the snapshot
	// and reverts what we did touch — also safe.
	if err := writeSnapshot(w.snapshotPath, adapters); err != nil {
		return fmt.Errorf("save dns snapshot: %w", err)
	}

	var firstErr error
	for _, a := range adapters {
		if skipAdapterDNS(a.InterfaceAlias) {
			continue
		}
		if err := setAdapterDNS(a.InterfaceIndex, servers); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("set dns on %q: %w", a.InterfaceAlias, err)
		}
	}
	return firstErr
}

func (w *windowsSystemDNS) Restore() error {
	data, err := os.ReadFile(w.snapshotPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read dns snapshot: %w", err)
	}

	var snap dnsSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		// Bad snapshot is worse than a stale one — drop it so we don't
		// re-enter restore forever on every start.
		_ = os.Remove(w.snapshotPath)
		return fmt.Errorf("parse dns snapshot: %w", err)
	}

	var firstErr error
	for _, a := range snap.Adapters {
		if skipAdapterDNS(a.InterfaceAlias) {
			continue
		}
		var err error
		if len(a.ServerAddresses) == 0 {
			err = resetAdapterDNS(a.InterfaceIndex)
		} else {
			err = setAdapterDNS(a.InterfaceIndex, a.ServerAddresses)
		}
		if err != nil {
			// TUN is torn down before Restore runs; a stale snapshot may still
			// reference its old InterfaceIndex — treat as success.
			if isAdapterGoneError(err) {
				continue
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("restore dns on %q: %w", a.InterfaceAlias, err)
			}
		}
	}

	// Remove the snapshot only on success; otherwise the next start gets
	// another shot at restoring.
	if firstErr == nil {
		_ = os.Remove(w.snapshotPath)
	}
	return firstErr
}

func writeSnapshot(path string, adapters []adapterDNS) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(dnsSnapshot{Adapters: adapters})
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func listAdapterDNS() ([]adapterDNS, error) {
	// Statically-configured adapters report their servers in ServerAddresses;
	// DHCP-configured ones also do (the resolved value). PowerShell surfaces
	// them the same way, so we snapshot whatever's effective right now and
	// restore that exact set on disconnect.
	script := `Get-DnsClientServerAddress -AddressFamily IPv4 | ` +
		`Where-Object { $_.ServerAddresses.Count -gt 0 } | ` +
		`Select-Object InterfaceIndex,InterfaceAlias,ServerAddresses | ` +
		`ConvertTo-Json -Compress`
	out, err := powerShellRun(script)
	if err != nil {
		return nil, fmt.Errorf("powershell: %w (out=%s)", err, strings.TrimSpace(string(out)))
	}
	adapters, err := parseAdapterDNS(out)
	if err != nil {
		return nil, err
	}
	return filterPhysicalAdapterDNS(adapters), nil
}

// skipAdapterDNS returns true for virtual tunnel adapters (tun0, Wintun, …).
// Their DNS is managed by sing-box and the adapter vanishes on disconnect —
// snapshotting them causes Restore to fail with InterfaceIndex not found.
func skipAdapterDNS(alias string) bool {
	return looksLikeTunnelInterface(alias)
}

func filterPhysicalAdapterDNS(adapters []adapterDNS) []adapterDNS {
	if len(adapters) == 0 {
		return nil
	}
	out := make([]adapterDNS, 0, len(adapters))
	for _, a := range adapters {
		if skipAdapterDNS(a.InterfaceAlias) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// isAdapterGoneError detects PowerShell/CIM failures when the interface
// index from an older snapshot no longer exists (typical after TUN teardown).
func isAdapterGoneError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "notfound_interfaceindex") ||
		strings.Contains(msg, "interfacenotfound") ||
		strings.Contains(msg, "objectnotfound") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "не найден") // localized PowerShell on RU Windows
}

// parseAdapterDNS accepts the output of `ConvertTo-Json -Compress`, which
// emits a JSON object for a single record and a JSON array for >1. Empty
// pipeline → empty output. We also tolerate Windows' UTF-16 LE BOM.
func parseAdapterDNS(raw []byte) ([]adapterDNS, error) {
	raw = stripBOM(raw)
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil, nil
	}

	switch raw[0] {
	case '[':
		var list []adapterDNS
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, fmt.Errorf("decode array: %w", err)
		}
		return list, nil
	case '{':
		var single adapterDNS
		if err := json.Unmarshal(raw, &single); err != nil {
			return nil, fmt.Errorf("decode object: %w", err)
		}
		return []adapterDNS{single}, nil
	default:
		return nil, fmt.Errorf("unexpected powershell output: %q", string(raw))
	}
}

func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		return b[2:]
	}
	return b
}

func setAdapterDNS(ifIdx int, servers []string) error {
	quoted := make([]string, 0, len(servers))
	for _, s := range servers {
		s = strings.TrimSpace(s)
		if !isSafeDNSToken(s) {
			return fmt.Errorf("unsafe dns server token: %q", s)
		}
		quoted = append(quoted, `"`+s+`"`)
	}
	script := fmt.Sprintf(
		`Set-DnsClientServerAddress -InterfaceIndex %d -ServerAddresses @(%s)`,
		ifIdx, strings.Join(quoted, ","),
	)
	if out, err := powerShellRun(script); err != nil {
		return fmt.Errorf("powershell: %w (out=%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func resetAdapterDNS(ifIdx int) error {
	script := fmt.Sprintf(
		`Set-DnsClientServerAddress -InterfaceIndex %d -ResetServerAddresses`,
		ifIdx,
	)
	if out, err := powerShellRun(script); err != nil {
		return fmt.Errorf("powershell: %w (out=%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// isSafeDNSToken keeps a tight whitelist (IPv4/IPv6 chars only). PowerShell
// runs the script via -Command, so anything passing through quoted strings
// is shell-interpreted; this rejects anything that isn't a numeric address.
func isSafeDNSToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		case r == '.' || r == ':':
		default:
			return false
		}
	}
	return true
}

func powerShellRun(script string) ([]byte, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.CombinedOutput()
}
