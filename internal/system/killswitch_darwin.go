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

//go:build darwin

package system

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
)

// pfRulesPath is the on-disk location of the kill-switch ruleset that pfctl
// loads. Living in /tmp keeps the write step root-free; only the pfctl load
// itself requires elevation.
const pfRulesPath = "/tmp/resultv-killswitch.conf"

// HasStaleKillSwitchFile reports whether a kill-switch ruleset file from a
// previous run is still on disk. /tmp is cleared on reboot, so the file's
// presence within a session is a strong hint that the previous process
// exited (Force Quit, crash) without calling Disable() — in which case pf
// may still be loaded with our blocking rules and the user has no internet.
func HasStaleKillSwitchFile() bool {
	_, err := os.Stat(pfRulesPath)
	return err == nil
}

// RemoveStaleKillSwitchFile deletes the rules file. It does NOT touch the
// running pf ruleset (`pfctl -d` requires root and would prompt for sudo on
// every launch); the caller should surface a warning so the user can run
// `sudo pfctl -d` manually if their internet is blocked.
func RemoveStaleKillSwitchFile() {
	_ = os.Remove(pfRulesPath)
}

// DarwinKillSwitch implements the kill switch via macOS's built-in Packet
// Filter (pf). When engaged it replaces the active pf ruleset with one that
// drops all outbound traffic except: loopback, RFC1918/link-local LAN, DNS
// (UDP/53), and explicit traffic to the configured proxy IP. Disable() turns
// pf off entirely, restoring the OS default (pf disabled by default on macOS).
//
// Caveats:
//   - Replaces the entire active pf ruleset. If the user runs another tool
//     that programs pf (rare on consumer macOS), those rules are clobbered
//     for the duration of the kill switch.
//   - Requires admin privileges (pfctl needs root). Elevation is requested
//     via the privileged helper, which prompts via osascript on macOS.
type DarwinKillSwitch struct {
	mu      sync.Mutex
	enabled bool
}

func NewKillSwitch() KillSwitch {
	return &DarwinKillSwitch{}
}

func (k *DarwinKillSwitch) Enable(proxyAddr string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.enabled {
		return nil
	}

	rules := buildPFRules(extractValidIP(proxyAddr))
	if err := os.WriteFile(pfRulesPath, []byte(rules), 0o644); err != nil {
		return fmt.Errorf("write pf rules: %w", err)
	}

	if out, err := RunPrivileged([]string{"pfctl", "-E", "-f", pfRulesPath}); err != nil {
		return fmt.Errorf("pfctl load: %s: %w", strings.TrimSpace(string(out)), err)
	}
	k.enabled = true
	return nil
}

func (k *DarwinKillSwitch) Disable() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.enabled {
		return nil
	}
	// Best-effort: disable pf and remove the ruleset file. Errors here are
	// not fatal — the user can always disable pf manually with `sudo pfctl -d`.
	_, _ = RunPrivileged([]string{"pfctl", "-d"})
	_ = os.Remove(pfRulesPath)
	k.enabled = false
	return nil
}

func (k *DarwinKillSwitch) IsEnabled() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.enabled
}

// DetectGPOConflict has no analogue on macOS; group-policy proxy lockdown is
// a Windows-specific concept.
func DetectGPOConflict() bool { return false }

func extractValidIP(addr string) string {
	if addr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = strings.TrimSpace(addr)
	}
	host = strings.Trim(host, "[]")
	if host == "" || net.ParseIP(host) == nil {
		return ""
	}
	return host
}

func buildPFRules(proxyIP string) string {
	var b strings.Builder
	b.WriteString("# ResultV kill switch — replaced on each Enable, removed on Disable\n")
	b.WriteString("set block-policy drop\n")
	b.WriteString("set skip on lo0\n")
	b.WriteString("block out all\n")
	b.WriteString("pass out from any to 127.0.0.0/8\n")
	b.WriteString("pass out from any to 10.0.0.0/8\n")
	b.WriteString("pass out from any to 172.16.0.0/12\n")
	b.WriteString("pass out from any to 192.168.0.0/16\n")
	b.WriteString("pass out from any to 169.254.0.0/16\n")
	b.WriteString("pass out proto udp from any to any port 53\n")
	if proxyIP != "" {
		b.WriteString(fmt.Sprintf("pass out from any to %s\n", proxyIP))
	}
	return b.String()
}
