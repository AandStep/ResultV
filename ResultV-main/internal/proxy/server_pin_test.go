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

package proxy

import (
	"strings"
	"testing"
)

// The outbound must keep the server DOMAIN (not a single pinned IP): sing-box
// re-resolves it against the static hosts record (see buildDNS) and fails over
// across the CDN's backends. Pinning the outbound to one IP was the regression
// that collapsed the whole session when that backend reset. The TLS SNI is the
// domain regardless, so the cert still validates.
func TestBuildProxyOutbound_KeepsServerDomainForFailover(t *testing.T) {
	out := buildProxyOutbound(ProxyConfig{
		IP:          "k.example.com",
		ResolvedIP:  "203.0.113.7",
		ResolvedIPs: []string{"203.0.113.7", "203.0.113.8"},
		Port:        3443,
		Type:        "hysteria2",
		Password:    "pw",
	})
	if out.Server != "k.example.com" {
		t.Fatalf("outbound must keep the domain so sing-box can fail over, got %q", out.Server)
	}
	if out.TLS == nil || out.TLS.ServerName != "k.example.com" {
		t.Fatalf("expected SNI to stay the original domain, got %+v", out.TLS)
	}
}

// The server domain resolves from a static `hosts` record seeded with EVERY
// connect-time backend IP, routed there by a dedicated DNS rule — never the
// fragile redirected OS `local` resolver. This is what restores pre-regression
// CDN failover without reintroducing false kill-switch trips.
func TestBuildDNS_PinsServerDomainToAllBackendsViaHosts(t *testing.T) {
	dns := buildDNS(EngineConfig{
		Mode: ProxyModeTunnel,
		Proxy: ProxyConfig{
			IP:          "k.example.com",
			ResolvedIPs: []string{"203.0.113.7", "203.0.113.8"},
			Type:        "hysteria2",
		},
	})
	var hosts *SBDNSServer
	for i := range dns.Servers {
		if dns.Servers[i].Tag == "server-pin" {
			hosts = &dns.Servers[i]
		}
	}
	if hosts == nil {
		t.Fatalf("expected a hosts server-pin DNS server, got %+v", dns.Servers)
	}
	if hosts.Type != "hosts" {
		t.Fatalf("server-pin must be a hosts server, got type %q", hosts.Type)
	}
	got := hosts.Predefined["k.example.com"]
	if len(got) != 2 || got[0] != "203.0.113.7" || got[1] != "203.0.113.8" {
		t.Fatalf("hosts record must hold every backend IP, got %v", got)
	}
	routed := false
	for _, r := range dns.Rules {
		if len(r.Domain) == 1 && r.Domain[0] == "k.example.com" {
			if r.Server != "server-pin" {
				t.Fatalf("server domain must route to server-pin, not %q (the fragile local resolver)", r.Server)
			}
			routed = true
		}
	}
	if !routed {
		t.Fatal("expected a DNS rule routing the server domain to server-pin")
	}
}

// Without any resolved IPs (literal-IP server, or a censored resolver) there is
// nothing to pin: the server domain falls back to the `local` resolver rule and
// no hosts server is emitted — never worse than the pre-fix behaviour.
func TestBuildDNS_NoPinFallsBackToLocal(t *testing.T) {
	dns := buildDNS(EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{IP: "k.example.com", Type: "hysteria2"},
	})
	for _, s := range dns.Servers {
		if s.Tag == "server-pin" {
			t.Fatal("no resolved IPs → no hosts pin should be emitted")
		}
	}
	found := false
	for _, r := range dns.Rules {
		if len(r.Domain) == 1 && r.Domain[0] == "k.example.com" {
			found = true
			if r.Server != "local" {
				t.Fatalf("unpinned server domain must fall back to local, got %q", r.Server)
			}
		}
	}
	if !found {
		t.Fatal("expected the server domain DNS rule")
	}
}

// Without a pinned IP the outbound keeps the raw host (domain or literal),
// so the change is inert for manually-entered IP servers.
func TestBuildProxyOutbound_NoPinKeepsRawServer(t *testing.T) {
	out := buildProxyOutbound(ProxyConfig{
		IP:       "198.51.100.10",
		Port:     443,
		Type:     "hysteria2",
		Password: "pw",
	})
	if out.Server != "198.51.100.10" {
		t.Fatalf("expected raw server preserved, got %q", out.Server)
	}
}

func TestResolvePinnedServerIP_LiteralIPReturnsEmpty(t *testing.T) {
	if got := resolvePinnedServerIP("203.0.113.7"); got != "" {
		t.Fatalf("literal IP needs no pin, got %q", got)
	}
	if got := resolvePinnedServerIP(""); got != "" {
		t.Fatalf("empty host returns empty, got %q", got)
	}
}

// Subscription servers must never leak the provider backend address into logs —
// including every failover backend in ResolvedIPs, not just the primary one.
func TestSingBoxLogWriter_RedactsSubscriptionServer(t *testing.T) {
	w := newSingBoxLogWriter(nil, ProxyConfig{
		IP:              "k.example.com",
		ResolvedIP:      "203.0.113.7",
		ResolvedIPs:     []string{"203.0.113.7", "203.0.113.8"},
		SubscriptionURL: "https://sub.example/abc",
	})
	in := "connection upload closed: raw-read tcp4 ...->203.0.113.8: forcibly closed (lookup k.example.com) [203.0.113.7]"
	got := w.redactServer(in)
	for _, leak := range []string{"k.example.com", "203.0.113.7", "203.0.113.8"} {
		if strings.Contains(got, leak) {
			t.Fatalf("server identifier %q leaked after redaction: %q", leak, got)
		}
	}
	if !strings.Contains(got, "<сервер>") {
		t.Fatalf("expected placeholder in redacted message: %q", got)
	}
}

// Every pinned backend IP of a CDN domain server must get a route-exclude CIDR
// so none of the server's own traffic loops back into the TUN.
func TestBuildTunnelConfig_RouteExcludesAllBackends(t *testing.T) {
	cfg, err := BuildTunnelModeConfig(EngineConfig{
		Mode: ProxyModeTunnel,
		Proxy: ProxyConfig{
			IP:          "k.example.com",
			ResolvedIPs: []string{"203.0.113.7", "203.0.113.8"},
			Port:        443,
			Type:        "hysteria2",
			Password:    "pw",
		},
	})
	if err != nil {
		t.Fatalf("BuildTunnelModeConfig: %v", err)
	}
	if len(cfg.Inbounds) == 0 || cfg.Inbounds[0].Type != "tun" {
		t.Fatalf("expected tun inbound first, got %+v", cfg.Inbounds)
	}
	excl := strings.Join(cfg.Inbounds[0].RouteExcludeAddress, ",")
	for _, want := range []string{"203.0.113.7/32", "203.0.113.8/32"} {
		if !strings.Contains(excl, want) {
			t.Fatalf("route-exclude must cover every backend, missing %q in %q", want, excl)
		}
	}
}

// Manual (non-subscription) servers keep full detail — the user owns them.
func TestSingBoxLogWriter_ManualServerNotRedacted(t *testing.T) {
	w := newSingBoxLogWriter(nil, ProxyConfig{IP: "198.51.100.10", Port: 443})
	in := "lookup 198.51.100.10 failed"
	if got := w.redactServer(in); got != in {
		t.Fatalf("manual server should not be redacted, got %q", got)
	}
}
