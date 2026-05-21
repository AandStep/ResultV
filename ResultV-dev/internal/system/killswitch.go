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
}

// fallbackDNS is applied when the user has no custom resolver configured. The
// trade-off (acknowledged in the security audit): DNS queries leave through
// Cloudflare/Google instead of the ISP, but they no longer leak to ANY
// destination as the previous open udp/53 rule allowed.
var fallbackDNS = []string{"1.1.1.1", "8.8.8.8"}

// resolveProxyIPs accepts a "host:port" string and returns the IP literals
// the kill switch should allow. Pure-IP hosts are returned as-is. Hostnames
// are resolved through the system resolver; we return ALL addresses (both
// v4 and v6) so CDN-fronted proxies with multiple A records still connect.
// Resolution uses the current OS DNS — this must happen BEFORE the kill
// switch arms, otherwise the very DNS query needed to bootstrap the firewall
// rules would be blocked.
func resolveProxyIPs(addr string) []string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = strings.TrimSpace(addr)
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return []string{ip.String()}
	}
	// Hostname path: cap at 2 seconds so a flaky resolver can't stall the
	// connect flow. Anything longer should be treated as a transient
	// network problem and surfaced — we don't want to silently fall back
	// to "no allow rule for the proxy".
	resolved, err := net.DefaultResolver.LookupIPAddr(context.Background(), host)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(resolved))
	seen := make(map[string]struct{}, len(resolved))
	for _, a := range resolved {
		s := a.IP.String()
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
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
