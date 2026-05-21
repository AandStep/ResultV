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
	"strings"
	"sync"
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

	if !ks.enabled {
		return nil
	}

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
	suffixes := []string{"_BlockAll", "_AllowProxy", "_AllowLocal"}
	for _, suffix := range suffixes {
		cmd := command("netsh", "advfirewall", "firewall", "delete", "rule",
			"name="+firewallRuleName+suffix,
		)
		_ = cmd.Run()
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
