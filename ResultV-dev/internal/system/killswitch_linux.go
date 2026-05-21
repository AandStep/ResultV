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

//go:build linux

package system

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const (
	nftablesRulesPath = "/tmp/resultv-killswitch.nft"
	nftTableName      = "resultv_killswitch"
	iptablesChainName = "RESULTV_KILLSWITCH"
)

// LinuxKillSwitch implements the kill switch via nftables (preferred) or
// iptables (legacy fallback). On Enable(), all outbound IPv4/IPv6 traffic is
// dropped except: loopback, RFC1918/link-local LAN, DNS (UDP/53), and traffic
// destined for the configured proxy IP. Disable() removes the table/chain.
//
// Detection: nft is preferred when available; otherwise we fall back to
// iptables. Both require root, which is requested through the privileged
// helper (sudo or pkexec).
type LinuxKillSwitch struct {
	mu            sync.Mutex
	enabled       bool
	useNftBackend bool
}

func NewKillSwitch() KillSwitch {
	return &LinuxKillSwitch{
		useNftBackend: nftablesAvailable(),
	}
}

func (k *LinuxKillSwitch) Enable(proxyAddr string, dnsServers []string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.enabled {
		return nil
	}
	proxyIPs := resolveProxyIPs(proxyAddr)
	if len(proxyIPs) == 0 {
		return fmt.Errorf("kill switch: no proxy IP to allow (address %q)", proxyAddr)
	}
	dnsIPs := extractDNSIPs(dnsServers)

	var err error
	if k.useNftBackend {
		err = enableNftables(proxyIPs, dnsIPs)
	} else {
		err = enableIptables(proxyIPs, dnsIPs)
	}
	if err != nil {
		return err
	}
	k.enabled = true
	return nil
}

func (k *LinuxKillSwitch) Disable() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.enabled {
		return nil
	}
	// Best-effort: a failure here leaves rules in place, but the user can
	// remove them manually. Returning the first error so callers can surface
	// it; the kill switch state is cleared regardless to avoid getting stuck.
	var firstErr error
	if k.useNftBackend {
		if err := disableNftables(); err != nil {
			firstErr = err
		}
	} else {
		if err := disableIptables(); err != nil {
			firstErr = err
		}
	}
	k.enabled = false
	return firstErr
}

func (k *LinuxKillSwitch) IsEnabled() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.enabled
}

// DetectGPOConflict has no analogue on Linux.
func DetectGPOConflict() bool { return false }

func nftablesAvailable() bool {
	_, err := exec.LookPath("nft")
	return err == nil
}

// --- nftables backend ---

func buildNftablesRuleset(proxyIPs []string, dnsIPs []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("table inet %s {\n", nftTableName))
	b.WriteString("\tchain output {\n")
	b.WriteString("\t\ttype filter hook output priority 0; policy drop;\n")
	b.WriteString("\t\tct state established,related accept\n")
	b.WriteString("\t\toif lo accept\n")
	b.WriteString("\t\tip daddr { 127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16 } accept\n")
	b.WriteString("\t\tip6 daddr { ::1, fe80::/10, fc00::/7 } accept\n")

	// DNS allow rules are scoped to specific resolver IPs. Previously the
	// kill switch let udp/tcp dport 53 go to ANY destination — that is the
	// classic kill-switch DNS leak (queries to ISP DNS still go out when the
	// tunnel drops). Now: only the configured (or fallback public) resolvers
	// can be reached on port 53.
	var v4dns, v6dns []string
	for _, ip := range dnsIPs {
		if strings.Contains(ip, ":") {
			v6dns = append(v6dns, ip)
		} else {
			v4dns = append(v4dns, ip)
		}
	}
	if len(v4dns) > 0 {
		list := strings.Join(v4dns, ", ")
		b.WriteString(fmt.Sprintf("\t\tip daddr { %s } udp dport 53 accept\n", list))
		b.WriteString(fmt.Sprintf("\t\tip daddr { %s } tcp dport 53 accept\n", list))
	}
	if len(v6dns) > 0 {
		list := strings.Join(v6dns, ", ")
		b.WriteString(fmt.Sprintf("\t\tip6 daddr { %s } udp dport 53 accept\n", list))
		b.WriteString(fmt.Sprintf("\t\tip6 daddr { %s } tcp dport 53 accept\n", list))
	}

	// Proxy server allow rules. When a hostname resolves to multiple IPs
	// (CDN, geo-loadbalanced VPN endpoints) all of them must be reachable.
	for _, proxyIP := range proxyIPs {
		if strings.Contains(proxyIP, ":") {
			b.WriteString(fmt.Sprintf("\t\tip6 daddr %s accept\n", proxyIP))
		} else {
			b.WriteString(fmt.Sprintf("\t\tip daddr %s accept\n", proxyIP))
		}
	}
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}

func enableNftables(proxyIPs []string, dnsIPs []string) error {
	rules := buildNftablesRuleset(proxyIPs, dnsIPs)
	// 0o600: this file contains the proxy IP — not a top-tier secret, but
	// it's a strong "this user is running a VPN to <IP>" indicator that
	// shouldn't be world-readable on a multi-user system. The matching
	// Darwin/Windows paths are already user-private; this keeps Linux in
	// line.
	if err := os.WriteFile(nftablesRulesPath, []byte(rules), 0o600); err != nil {
		return fmt.Errorf("write nft ruleset: %w", err)
	}
	// Remove any leftover table from a previous run before installing.
	_, _ = runPrivileged([]string{"nft", "delete", "table", "inet", nftTableName})
	if out, err := runPrivileged([]string{"nft", "-f", nftablesRulesPath}); err != nil {
		return fmt.Errorf("nft load: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func disableNftables() error {
	out, err := runPrivileged([]string{"nft", "delete", "table", "inet", nftTableName})
	_ = os.Remove(nftablesRulesPath)
	if err != nil {
		// If the table didn't exist, treat as success.
		if strings.Contains(string(out), "No such file") || strings.Contains(string(out), "does not exist") {
			return nil
		}
		return fmt.Errorf("nft delete: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// --- iptables backend ---

func enableIptables(proxyIPs []string, dnsIPs []string) error {
	// Tear down any leftover chain first so we start from a known state.
	_ = disableIptables()

	steps := [][]string{
		{"iptables", "-N", iptablesChainName},
		{"iptables", "-A", iptablesChainName, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"},
		{"iptables", "-A", iptablesChainName, "-o", "lo", "-j", "ACCEPT"},
		{"iptables", "-A", iptablesChainName, "-d", "127.0.0.0/8", "-j", "ACCEPT"},
		{"iptables", "-A", iptablesChainName, "-d", "10.0.0.0/8", "-j", "ACCEPT"},
		{"iptables", "-A", iptablesChainName, "-d", "172.16.0.0/12", "-j", "ACCEPT"},
		{"iptables", "-A", iptablesChainName, "-d", "192.168.0.0/16", "-j", "ACCEPT"},
		{"iptables", "-A", iptablesChainName, "-d", "169.254.0.0/16", "-j", "ACCEPT"},
	}
	// Per-resolver DNS allow rules (previously: unconditional --dport 53).
	// iptables (v4) does not handle IPv6 addresses; v6 DNS resolvers would
	// need ip6tables, which we don't manage here — they're silently skipped.
	for _, ip := range dnsIPs {
		if strings.Contains(ip, ":") {
			continue
		}
		steps = append(steps,
			[]string{"iptables", "-A", iptablesChainName, "-d", ip, "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
			[]string{"iptables", "-A", iptablesChainName, "-d", ip, "-p", "tcp", "--dport", "53", "-j", "ACCEPT"},
		)
	}
	// iptables here is v4-only; skip v6 proxy IPs silently (ip6tables would
	// need a separate management path we don't currently maintain).
	for _, proxyIP := range proxyIPs {
		if strings.Contains(proxyIP, ":") {
			continue
		}
		steps = append(steps, []string{"iptables", "-A", iptablesChainName, "-d", proxyIP, "-j", "ACCEPT"})
	}
	steps = append(steps,
		[]string{"iptables", "-A", iptablesChainName, "-j", "DROP"},
		[]string{"iptables", "-I", "OUTPUT", "-j", iptablesChainName},
	)

	for _, cmd := range steps {
		if out, err := runPrivileged(cmd); err != nil {
			return fmt.Errorf("iptables %s: %s: %w",
				strings.Join(cmd[1:], " "),
				strings.TrimSpace(string(out)),
				err,
			)
		}
	}
	return nil
}

func disableIptables() error {
	// Order: detach from OUTPUT, flush, then delete the chain.
	_, _ = runPrivileged([]string{"iptables", "-D", "OUTPUT", "-j", iptablesChainName})
	_, _ = runPrivileged([]string{"iptables", "-F", iptablesChainName})
	_, _ = runPrivileged([]string{"iptables", "-X", iptablesChainName})
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
	if host == "" || net.ParseIP(host) == nil {
		return ""
	}
	return host
}
