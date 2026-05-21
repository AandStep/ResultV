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

func (k *DarwinKillSwitch) Enable(proxyAddr string, dnsServers []string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.enabled {
		return nil
	}

	proxyIPs := resolveProxyIPs(proxyAddr)
	if len(proxyIPs) == 0 {
		return fmt.Errorf("kill switch: no proxy IP to allow (address %q)", proxyAddr)
	}
	rules := buildPFRules(proxyIPs, extractDNSIPs(dnsServers))
	if err := os.WriteFile(pfRulesPath, []byte(rules), 0o600); err != nil {
		return fmt.Errorf("write pf rules: %w", err)
	}

	if out, err := runPrivileged([]string{"pfctl", "-E", "-f", pfRulesPath}); err != nil {
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
	_, _ = runPrivileged([]string{"pfctl", "-d"})
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

func buildPFRules(proxyIPs []string, dnsIPs []string) string {
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
	// Per-resolver DNS allow rules. The old "pass out ... to any port 53"
	// allowed every DNS query out, defeating the kill switch on a tunnel
	// drop. Only the configured (or fallback public) resolvers are now
	// reachable on port 53.
	for _, ip := range dnsIPs {
		b.WriteString(fmt.Sprintf("pass out proto udp from any to %s port 53\n", ip))
		b.WriteString(fmt.Sprintf("pass out proto tcp from any to %s port 53\n", ip))
	}
	for _, proxyIP := range proxyIPs {
		b.WriteString(fmt.Sprintf("pass out from any to %s\n", proxyIP))
	}
	return b.String()
}
