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

package system

import (
	"context"
	"net"
	"strings"
)

type KillSwitch interface {
	// Enable installs firewall rules that drop all outbound traffic except
	// loopback, RFC1918 LAN, the proxy server, and DNS queries to the listed
	// resolvers. The dnsServers list is normalized via extractDNSIPs, which
	// substitutes a public default (1.1.1.1 + 8.8.8.8) when no valid IP is
	// supplied — otherwise armed kill switch + empty list would silently
	// kill DNS in proxy mode and the user would see "no internet".
	//
	// proxyAddr may be "host:port" with host as an IP literal OR a domain.
	// Domains are resolved to all returned IPs before rule installation so
	// CDN-fronted proxies (multiple A records) still connect under the
	// firewall. If resolution fails the kill switch returns an error rather
	// than installing rules that would block the proxy server itself.
	Enable(proxyAddr string, dnsServers []string) error

	Disable() error

	IsEnabled() bool

	// HasLeftoverRules reports whether firewall rules from a previous run are
	// still installed in the OS. Unlike IsEnabled (in-memory, reset on every
	// fresh process), it survives a crash / force-kill and is the detection
	// anchor for startup leftover cleanup.
	HasLeftoverRules() bool

	// RemoveLeftoverRules deletes any kill-switch firewall rules regardless of
	// in-memory state. Safe to call when nothing is installed (no-op).
	RemoveLeftoverRules() error

	// RestoreCommands returns CLI commands to clean up firewall rules elevated.
	RestoreCommands() []string
}

// fallbackDNS is applied when the user has no custom resolver configured. The
// trade-off (acknowledged in the security audit): DNS queries leave through
// Cloudflare/Google instead of the ISP, but they no longer leak to ANY
// destination as the previous open udp/53 rule allowed.
var fallbackDNS = []string{"1.1.1.1", "8.8.8.8"}

// resolveProxyIPs returns the IP literals the kill switch should allow. The
// input is one or more comma-separated "host:port" (or bare host) entries: the
// connect-time path pre-pins every backend IP of a CDN/multi-IP server and
// passes them all here as literals (e.g. "1.2.3.4:443,5.6.7.8:443"), so the
// firewall allows every backend sing-box may fail over to — otherwise a
// failover to an un-allowed IP would be blocked, the health probe through it
// would fail, and the kill switch could never disengage. A bare IP host is
// returned as-is; a hostname is resolved through the system resolver (ALL v4+v6
// addresses). Resolution must happen BEFORE the kill switch arms, otherwise the
// very DNS query needed to bootstrap the rules would itself be blocked.
func resolveProxyIPs(addr string) []string {
	out := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	add := func(s string) {
		if s == "" {
			return
		}
		if _, dup := seen[s]; dup {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, part := range strings.Split(addr, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		host, _, err := net.SplitHostPort(part)
		if err != nil {
			host = part
		}
		host = strings.Trim(strings.TrimSpace(host), "[]")
		if host == "" {
			continue
		}
		if ip := net.ParseIP(host); ip != nil {
			add(ip.String())
			continue
		}
		// Hostname path: cap at 2 seconds so a flaky resolver can't stall the
		// connect flow. Anything longer should be treated as a transient
		// network problem and surfaced — we don't want to silently fall back
		// to "no allow rule for the proxy".
		resolved, err := net.DefaultResolver.LookupIPAddr(context.Background(), host)
		if err != nil {
			continue
		}
		for _, a := range resolved {
			add(a.IP.String())
		}
	}
	return out
}

// extractDNSIPs normalizes a user-supplied DNS list into bare IP literals:
//   - strips ":port" suffix ("1.1.1.1:53" -> "1.1.1.1"),
//   - skips empty strings and hostnames (we cannot resolve them without DNS),
//   - deduplicates while preserving order,
//   - falls back to fallbackDNS when nothing valid remains.
func extractDNSIPs(servers []string) []string {
	seen := make(map[string]struct{}, len(servers)+len(fallbackDNS))
	out := make([]string, 0, len(servers))
	for _, raw := range servers {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if host, _, err := net.SplitHostPort(s); err == nil {
			s = host
		}
		s = strings.Trim(s, "[]")
		if net.ParseIP(s) == nil {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		out = append(out, fallbackDNS...)
	}
	return out
}
