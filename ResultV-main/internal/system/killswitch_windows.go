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
















package system

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

const (
	firewallRuleName = "ResultV_KillSwitch"
)








type WindowsKillSwitch struct {
	mu      sync.Mutex
	enabled bool
	isAdmin bool
	// dnsRuleCount tracks how many _AllowDNS_<i> rules were installed in the
	// last Enable() so Disable() can clean them up. Without this, narrowing
	// the DNS allow list (we used to have a single _AllowDNS) would leak
	// rules across enable/disable cycles.
	dnsRuleCount int
	// proxyRuleCount tracks how many _AllowProxy[_<i>] rules were installed
	// — multiple when the proxy host resolved to several A records. Disable
	// uses this to clean up trailing rules.
	proxyRuleCount int
}


func NewKillSwitch() KillSwitch {
	return &WindowsKillSwitch{
		isAdmin: IsAdmin(),
	}
}



func (ks *WindowsKillSwitch) Enable(proxyAddr string, dnsServers []string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if ks.enabled {
		return nil
	}

	if ks.isAdmin {
		if err := ks.enableFirewall(proxyAddr, extractDNSIPs(dnsServers)); err != nil {
			return fmt.Errorf("enabling firewall kill switch: %w", err)
		}
	} else {
		return fmt.Errorf("kill switch requires administrator privileges for firewall rules")
	}

	ks.enabled = true
	return nil
}


func (ks *WindowsKillSwitch) Disable() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	// Always attempt to remove rules regardless of in-memory state.
	// The enabled flag can be stale after a crash recovery or when kill switch
	// was toggled via a non-standard path. disableFirewall() is a no-op when
	// no rules are installed, so unconditional calls are safe.
	if err := ks.disableFirewall(); err != nil {
		return fmt.Errorf("disabling firewall kill switch: %w", err)
	}

	ks.enabled = false
	return nil
}


func (ks *WindowsKillSwitch) IsEnabled() bool {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.enabled
}

// HasLeftoverRules reports whether kill-switch firewall rules from a previous
// run are still installed. Unlike IsEnabled (in-memory, reset on every fresh
// process), this queries the OS, so it detects rules stranded by a crash or
// force-kill. We probe the _BlockAll rule — the one that actually severs the
// internet; if it's gone, any stray allow rules are harmless. netsh echoes the
// literal rule name when the rule exists and prints "No rules match..."
// otherwise, so a substring check on our ASCII name is locale-independent.
func (ks *WindowsKillSwitch) HasLeftoverRules() bool {
	out, _ := command("netsh", "advfirewall", "firewall", "show", "rule",
		"name="+firewallRuleName+"_BlockAll").CombinedOutput()
	return strings.Contains(string(out), firewallRuleName+"_BlockAll")
}

// RemoveLeftoverRules deletes all kill-switch firewall rules regardless of
// in-memory state. Used by the startup leftover-cleanup flow, where the rules
// exist in the OS but ks.enabled is false because the process is fresh.
// When the process lacks admin rights (typical after a crash relaunch), firewall
// rules cannot be deleted directly — netsh requires elevation. In that case we
// fall back to an elevated PowerShell (UAC prompt), mirroring the DNS restore
// path. Without this, a non-admin relaunch after a kill-switch crash leaves the
// _BlockAll rule in place and the internet stays dead.
func (ks *WindowsKillSwitch) RemoveLeftoverRules() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	var err error
	if ks.isAdmin {
		err = ks.disableFirewall()
	} else {
		err = ks.disableFirewallElevated()
	}
	ks.enabled = false
	return err
}

func (ks *WindowsKillSwitch) RestoreCommands() []string {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	script := ks.buildFirewallCleanupScript()
	var cmds []string
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			cmds = append(cmds, line)
		}
	}
	return cmds
}



func (ks *WindowsKillSwitch) enableFirewall(proxyAddr string, dnsIPs []string) error {

	_ = ks.disableFirewall()

	proxyIPs := resolveProxyIPs(proxyAddr)
	if len(proxyIPs) == 0 {
		return fmt.Errorf("kill switch: no proxy IP to allow (address %q)", proxyAddr)
	}

	blockCmd := command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+firewallRuleName+"_BlockAll",
		"dir=out",
		"action=block",
		"enable=yes",
		"profile=any",
		"protocol=any",
	)
	if out, err := blockCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("adding block rule: %s: %w", string(out), err)
	}

	
	// Allow the proxy server. We resolve hostnames here so CDN-fronted
	// proxies with multiple A records (or proxies addressed by domain) get
	// every IP in the allow list. Without this, the connection request
	// would itself be blocked because Windows enforces the rules before
	// the proxy connect attempt.
	for i, ip := range proxyIPs {
		ruleName := firewallRuleName + "_AllowProxy"
		if i > 0 {
			ruleName = fmt.Sprintf("%s_AllowProxy_%d", firewallRuleName, i)
		}
		cmd := command("netsh", "advfirewall", "firewall", "add", "rule",
			"name="+ruleName,
			"dir=out",
			"action=allow",
			"enable=yes",
			"profile=any",
			"protocol=any",
			"remoteip="+ip,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("adding allow proxy rule (%s): %s: %w", ip, string(out), err)
		}
	}
	ks.proxyRuleCount = len(proxyIPs)

	
	allowLocalCmd := command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+firewallRuleName+"_AllowLocal",
		"dir=out",
		"action=allow",
		"enable=yes",
		"profile=any",
		"protocol=any",
		"remoteip=127.0.0.0/8,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16",
	)
	if out, err := allowLocalCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("adding allow local rule: %s: %w", string(out), err)
	}

	// Narrow DNS: previously this allowed udp/53 to ANY remote, which is the
	// classic kill-switch DNS leak — when the tunnel drops the OS resolver
	// still queried ISP DNS in plaintext. Now we allow port 53 (udp+tcp) only
	// to the explicit resolver IPs from settings. extractDNSIPs guarantees at
	// least one fallback IP (1.1.1.1) so users without custom DNS still have
	// working name resolution under the kill switch.
	ks.dnsRuleCount = 0
	for i, dnsIP := range dnsIPs {
		for _, proto := range []string{"udp", "tcp"} {
			cmd := command("netsh", "advfirewall", "firewall", "add", "rule",
				fmt.Sprintf("name=%s_AllowDNS_%d_%s", firewallRuleName, i, proto),
				"dir=out",
				"action=allow",
				"enable=yes",
				"profile=any",
				"protocol="+proto,
				"remoteport=53",
				"remoteip="+dnsIP,
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				// Best-effort: a single failed DNS rule shouldn't break the
				// whole kill switch (the block-all rule is the safety net).
				_ = out
			}
		}
		ks.dnsRuleCount = i + 1
	}

	return nil
}




func extractValidIP(addr string) string {
	if addr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = strings.TrimSpace(addr)
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return ""
	}
	if net.ParseIP(host) == nil {
		return ""
	}
	return host
}


func (ks *WindowsKillSwitch) disableFirewall() error {
	// Delete the _BlockAll rule first — this is the one that kills the internet.
	// Check its error: "access denied" means we can't restore connectivity and
	// the caller needs to know. "No rules match" is success (already gone).
	blockCmd := command("netsh", "advfirewall", "firewall", "delete", "rule",
		"name="+firewallRuleName+"_BlockAll",
	)
	blockOut, blockErr := blockCmd.CombinedOutput()
	if blockErr != nil && !isNetshNoRulesMatch(blockOut) {
		return fmt.Errorf("delete _BlockAll: %s: %w", strings.TrimSpace(string(blockOut)), blockErr)
	}

	// Allow rules are best-effort: harmless if stranded (they only permit,
	// not block), so errors are swallowed.
	for _, suffix := range []string{"_AllowProxy", "_AllowLocal"} {
		_ = command("netsh", "advfirewall", "firewall", "delete", "rule",
			"name="+firewallRuleName+suffix,
		).Run()
	}
	// Clean up _AllowProxy_<i> rules (i >= 1) from a previous Enable() that
	// had multiple proxy IPs. We sweep up to max(stored count, 8) to handle
	// builds where the count wasn't tracked yet.
	proxyCount := max(ks.proxyRuleCount, 8)
	for i := 1; i < proxyCount; i++ {
		_ = command("netsh", "advfirewall", "firewall", "delete", "rule",
			fmt.Sprintf("name=%s_AllowProxy_%d", firewallRuleName, i),
		).Run()
	}
	ks.proxyRuleCount = 0
	// Drop legacy single-rule name in case we are disabling rules installed by
	// an older build. New per-IP rules are named _AllowDNS_<i>_<proto>.
	_ = command("netsh", "advfirewall", "firewall", "delete", "rule",
		"name="+firewallRuleName+"_AllowDNS",
	).Run()
	// Clean up _AllowDNS_<i>_<proto> rules from the previous Enable().
	count := max(ks.dnsRuleCount, len(fallbackDNS))
	for i := 0; i < count; i++ {
		for _, proto := range []string{"udp", "tcp"} {
			_ = command("netsh", "advfirewall", "firewall", "delete", "rule",
				fmt.Sprintf("name=%s_AllowDNS_%d_%s", firewallRuleName, i, proto),
			).Run()
		}
	}
	ks.dnsRuleCount = 0
	return nil
}

// isNetshNoRulesMatch returns true when netsh output indicates there was nothing
// to delete — locale-independent because we check the ASCII rule name echo plus
// the English-only "No rules" string that netsh prints regardless of UI lang.
func isNetshNoRulesMatch(out []byte) bool {
	s := string(out)
	return strings.Contains(s, "No rules") || !strings.Contains(s, firewallRuleName)
}

// disableFirewallElevated builds a netsh script that deletes all kill-switch
// rules and runs it through an elevated PowerShell process (UAC prompt).
// Used by RemoveLeftoverRules when the app lacks admin rights — netsh firewall
// commands require elevation, and without this the _BlockAll rule would stay
// in place after a crash, leaving the user with no internet.
func (ks *WindowsKillSwitch) disableFirewallElevated() error {
	script := ks.buildFirewallCleanupScript()

	dir := filepath.Join(UserDataDir(), "elevated")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create elevated dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "ks-cleanup-*.ps1")
	if err != nil {
		return fmt.Errorf("create temp script: %w", err)
	}
	path := tmp.Name()
	if _, err := tmp.WriteString(script); err != nil {
		tmp.Close()
		_ = os.Remove(path)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	defer os.Remove(path)

	esc := strings.ReplaceAll(path, "'", "''")
	launcher := fmt.Sprintf(
		`Start-Process -FilePath 'powershell.exe' -Verb RunAs -Wait -WindowStyle Hidden -ArgumentList '-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass','-File','%s'`,
		esc,
	)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", launcher)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("elevated kill switch cleanup: %w (out=%s)", err, strings.TrimSpace(string(out)))
	}
	ks.proxyRuleCount = 0
	ks.dnsRuleCount = 0
	return nil
}

// buildFirewallCleanupScript generates a PowerShell script that deletes every
// known kill-switch rule name via netsh. The script is self-contained (no
// external deps) and suitable for running elevated.
func (ks *WindowsKillSwitch) buildFirewallCleanupScript() string {
	var b strings.Builder
	// Block rule — the critical one.
	fmt.Fprintf(&b, "netsh advfirewall firewall delete rule name=%s_BlockAll\n", firewallRuleName)
	// Static allow rules.
	for _, suffix := range []string{"_AllowProxy", "_AllowLocal"} {
		fmt.Fprintf(&b, "netsh advfirewall firewall delete rule name=%s%s\n", firewallRuleName, suffix)
	}
	// Numbered proxy rules.
	proxyCount := max(ks.proxyRuleCount, 8)
	for i := 1; i < proxyCount; i++ {
		fmt.Fprintf(&b, "netsh advfirewall firewall delete rule name=%s_AllowProxy_%d\n", firewallRuleName, i)
	}
	// Legacy single DNS rule.
	fmt.Fprintf(&b, "netsh advfirewall firewall delete rule name=%s_AllowDNS\n", firewallRuleName)
	// Numbered DNS rules.
	count := max(ks.dnsRuleCount, len(fallbackDNS))
	for i := 0; i < count; i++ {
		for _, proto := range []string{"udp", "tcp"} {
			fmt.Fprintf(&b, "netsh advfirewall firewall delete rule name=%s_AllowDNS_%d_%s\n", firewallRuleName, i, proto)
		}
	}
	return b.String()
}



func DetectGPOConflict() bool {
	
	cmd := command("reg", "query",
		`HKLM\Software\Policies\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxySettingsPerUser",
	)
	out, err := cmd.CombinedOutput()
	if err == nil && len(out) > 0 {
		
		return true
	}

	
	cmd2 := command("reg", "query",
		`HKLM\Software\Policies\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyEnable",
	)
	out2, err2 := cmd2.CombinedOutput()
	if err2 == nil && len(out2) > 0 {
		return true
	}

	return false
}
