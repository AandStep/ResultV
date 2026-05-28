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
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

func TestBuildRoute_NestedDomainException_ProducesProxyOverride(t *testing.T) {
	cfg := EngineConfig{
		RoutingMode: ModeWhitelist,
		Whitelist:   []string{".ru", "2ip.ru"},
	}

	route := buildRoute(cfg)
	if route == nil {
		t.Fatal("expected non-nil route")
	}

	var ruDirect bool
	var twoIPProxy bool
	var twoIPRuleIndex = -1
	var ruRuleIndex = -1

	for i, r := range route.Rules {
		if len(r.DomainSuffix) != 1 {
			continue
		}
		switch r.DomainSuffix[0] {
		case "ru":
			if r.Outbound == "direct" {
				ruDirect = true
				ruRuleIndex = i
			}
		case "2ip.ru":
			if r.Outbound == "proxy" {
				twoIPProxy = true
				twoIPRuleIndex = i
			}
		}
	}

	if !ruDirect {
		t.Fatalf("expected direct rule for ru suffix, rules=%+v", route.Rules)
	}
	if !twoIPProxy {
		t.Fatalf("expected proxy override rule for 2ip.ru suffix, rules=%+v", route.Rules)
	}
	if twoIPRuleIndex > ruRuleIndex {
		t.Fatalf("expected more specific rule (2ip.ru) before ru: twoIP=%d ru=%d", twoIPRuleIndex, ruRuleIndex)
	}
}

func TestBuildRoute_TunnelMode_IncludesSelfDirectRule(t *testing.T) {
	cfg := EngineConfig{Mode: ProxyModeTunnel}
	route := buildRoute(cfg)
	if route == nil {
		t.Fatal("expected non-nil route")
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	base := filepath.Base(exe)
	want := `(?i)(^|[\\/])` + regexp.QuoteMeta(base) + `$`

	var found bool
	for _, r := range route.Rules {
		if r.Outbound != "direct" || len(r.ProcessPathRegex) == 0 {
			continue
		}
		for _, rx := range r.ProcessPathRegex {
			if rx == want {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatalf("expected self direct rule with process_path_regex %q, rules=%+v", want, route.Rules)
	}
}

func TestBuildRoute_TunnelMode_ProbeDomainsRoutedThroughProxyBeforeSelfDirect(t *testing.T) {
	cfg := EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "ss"},
	}
	route := buildRoute(cfg)
	if route == nil {
		t.Fatal("expected non-nil route")
	}

	probeIdx := -1
	selfDirectIdx := -1
	for i, r := range route.Rules {
		if r.Outbound == "proxy" && len(r.Domain) > 0 {
			for _, d := range r.Domain {
				if d == "connectivitycheck.gstatic.com" {
					probeIdx = i
					break
				}
			}
		}
		if r.Outbound == "direct" && len(r.ProcessPathRegex) > 0 {
			selfDirectIdx = i
		}
	}
	if probeIdx < 0 {
		t.Fatalf("expected probe-domain → proxy rule, rules=%+v", route.Rules)
	}
	if selfDirectIdx < 0 {
		t.Fatalf("expected self-direct rule for non-endpoint protocol")
	}
	if probeIdx > selfDirectIdx {
		t.Fatalf("probe-domain rule (idx=%d) must precede self-direct rule (idx=%d)", probeIdx, selfDirectIdx)
	}
}

// WG/AWG share the "proxy" tag with regular outbounds (see buildEndpoints),
// so the same probe-domain → proxy and self-direct rules must apply for them
// too: the post-start HTTP probe runs from our own process, and without these
// rules it would either escape through direct (false success) or race with
// strict_route's WFP filters (false failure) — the latter is what broke
// AmneziaWG connect in 3.2.1.
func TestBuildRoute_TunnelMode_WireGuardIncludesProbeAndSelfDirectRules(t *testing.T) {
	assertProbeAndSelfDirectRulesPresent(t, "wireguard")
}

func TestBuildRoute_TunnelMode_AmneziaWGIncludesProbeAndSelfDirectRules(t *testing.T) {
	assertProbeAndSelfDirectRulesPresent(t, "amneziawg")
}

func assertProbeAndSelfDirectRulesPresent(t *testing.T, proxyType string) {
	t.Helper()
	cfg := EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: proxyType},
	}
	route := buildRoute(cfg)
	if route == nil {
		t.Fatal("expected non-nil route")
	}

	probeIdx := -1
	selfDirectIdx := -1
	for i, r := range route.Rules {
		if r.Outbound == "proxy" && len(r.Domain) > 0 {
			for _, d := range r.Domain {
				if d == "connectivitycheck.gstatic.com" {
					probeIdx = i
					break
				}
			}
		}
		if r.Outbound == "direct" && len(r.ProcessPathRegex) > 0 {
			selfDirectIdx = i
		}
	}
	if probeIdx < 0 {
		t.Fatalf("%s: expected probe-domain → proxy rule, rules=%+v", proxyType, route.Rules)
	}
	if selfDirectIdx < 0 {
		t.Fatalf("%s: expected self-direct rule, rules=%+v", proxyType, route.Rules)
	}
	if probeIdx > selfDirectIdx {
		t.Fatalf("%s: probe-domain rule (idx=%d) must precede self-direct rule (idx=%d)", proxyType, probeIdx, selfDirectIdx)
	}
}

func TestBuildRoute_TunnelMode_DoesNotBlockUDP443(t *testing.T) {
	cfg := EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "http"},
	}
	route := buildRoute(cfg)
	if route == nil {
		t.Fatal("expected non-nil route")
	}

	for _, r := range route.Rules {
		if r.Outbound != "block" || r.Action != "route" {
			continue
		}
		if len(r.Network) == 1 && r.Network[0] == "udp" && len(r.Port) == 1 && r.Port[0] == 443 {
			t.Fatalf("did not expect udp:443 block rule in tunnel mode, rules=%+v", route.Rules)
		}
	}
}

func TestBuildRoute_TunnelMode_Hysteria2DoesNotBlockUDP443(t *testing.T) {
	cfg := EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "hysteria2"},
	}
	route := buildRoute(cfg)
	if route == nil {
		t.Fatal("expected non-nil route")
	}
	for _, r := range route.Rules {
		if r.Outbound != "block" || r.Action != "route" {
			continue
		}
		if len(r.Network) == 1 && r.Network[0] == "udp" && len(r.Port) == 1 && r.Port[0] == 443 {
			t.Fatalf("did not expect udp:443 block for hysteria2, rules=%+v", route.Rules)
		}
	}
}

func TestBuildTunnelModeConfig_WireGuardFinalTargetDefined(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "wireguard"},
	})
	if cfg.Route == nil {
		t.Fatal("expected route")
	}
	if cfg.Route.Final != "proxy" {
		t.Fatalf("unexpected final tag: %s", cfg.Route.Final)
	}
	if err := validateRouteFinalTarget(cfg); err != nil {
		t.Fatalf("expected valid final target: %v", err)
	}
}

func TestBuildTunnelModeConfig_TunStackRespectsConfig(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:     ProxyModeTunnel,
		Proxy:    ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		TunStack: "gvisor",
	})
	if len(cfg.Inbounds) == 0 {
		t.Fatal("expected TUN inbound")
	}
	if got := cfg.Inbounds[0].Stack; got != "gvisor" {
		t.Fatalf("TUN stack: want gvisor, got %q", got)
	}
}

func TestBuildTunnelModeConfig_TunStackDefaultsToSystem(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
	})
	if len(cfg.Inbounds) == 0 {
		t.Fatal("expected TUN inbound")
	}
	if got := cfg.Inbounds[0].Stack; got != "system" {
		t.Fatalf("default TUN stack: want system, got %q", got)
	}
}

func TestBuildTunnelModeConfig_DNSServersPresent(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "hysteria2"},
	})
	if cfg.DNS == nil || len(cfg.DNS.Servers) == 0 {
		t.Fatal("dns servers missing")
	}
	foundNonLocal := false
	for _, s := range cfg.DNS.Servers {
		if s.Type != "local" {
			foundNonLocal = true
		}
	}
	if !foundNonLocal {
		t.Fatal("expected at least one non-local dns server")
	}
}

func TestBuildTunnelModeConfig_SSTunnelHasTCPDNSDetour(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "ss"},
	})
	if cfg.DNS == nil {
		t.Fatal("dns missing")
	}
	hasTCP := false
	for _, s := range cfg.DNS.Servers {
		if s.Type == "tcp" && s.Detour == "proxy" {
			hasTCP = true
			break
		}
	}
	if !hasTCP {
		t.Fatalf("expected at least one tcp dns server with proxy detour, got: %+v", cfg.DNS.Servers)
	}
}

func TestBuildTunnelModeConfig_CustomDNSUniqueTagsAndTCPForSSTunnel(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:       ProxyModeTunnel,
		Proxy:      ProxyConfig{Type: "SS"},
		DNSServers: []string{"8.8.8.8", "1.1.1.1"},
	})
	if cfg.DNS == nil {
		t.Fatal("dns missing")
	}
	seenTags := map[string]struct{}{}
	tcpCount := 0
	for _, s := range cfg.DNS.Servers {
		if _, ok := seenTags[s.Tag]; ok {
			t.Fatalf("duplicate dns tag found: %q in %+v", s.Tag, cfg.DNS.Servers)
		}
		seenTags[s.Tag] = struct{}{}
		if s.Type == "tcp" && s.Detour == "proxy" {
			tcpCount++
		}
	}
	if tcpCount < 2 {
		t.Fatalf("expected tcp detour servers for each custom dns, got %+v", cfg.DNS.Servers)
	}
}

func TestBuildTunnelModeConfig_IPv4OnlyServerForcesIPv4DNS(t *testing.T) {

	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{IP: "185.126.67.168", Port: 443, Type: "hysteria2"},
	})
	if cfg.DNS == nil {
		t.Fatal("dns missing")
	}
	if cfg.DNS.Strategy != "ipv4_only" {
		t.Fatalf("expected ipv4_only DNS strategy for IPv4-only server, got: %q", cfg.DNS.Strategy)
	}

	cfg2 := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{IP: "1.2.3.4", Port: 443, Type: "vless"},
	})
	if cfg2.DNS.Strategy != "ipv4_only" {
		t.Fatalf("expected ipv4_only for VLESS IPv4 server, got: %q", cfg2.DNS.Strategy)
	}
}

func TestBuildRoute_TunnelMode_ServerIPBypassBeforeSniff(t *testing.T) {

	cfg := EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{IP: "185.126.67.168", Port: 443, Type: "hysteria2"},
	}
	route := buildRoute(cfg)
	if route == nil {
		t.Fatal("expected non-nil route")
	}

	bypassIdx := -1
	sniffIdx := -1
	for i, r := range route.Rules {
		if r.Action == "sniff" && sniffIdx == -1 {
			sniffIdx = i
		}
		if r.Outbound == "direct" && len(r.IPCidr) > 0 {
			for _, cidr := range r.IPCidr {
				if cidr == "185.126.67.168/32" {
					bypassIdx = i
					break
				}
			}
		}
	}

	if bypassIdx == -1 {
		t.Fatalf("expected server IP bypass rule, rules=%+v", route.Rules)
	}
	if sniffIdx == -1 {
		t.Fatalf("expected sniff rule, rules=%+v", route.Rules)
	}
	if bypassIdx >= sniffIdx {
		t.Fatalf("server IP bypass (idx=%d) must come BEFORE sniff (idx=%d) to prevent routing loops, rules=%+v",
			bypassIdx, sniffIdx, route.Rules)
	}
}

func TestSplitDNSServer(t *testing.T) {
	cases := []struct {
		in       string
		wantHost string
		wantPort int
	}{
		{in: "8.8.8.8", wantHost: "8.8.8.8", wantPort: 0},
		{in: "1.1.1.1:5353", wantHost: "1.1.1.1", wantPort: 5353},
		{in: "[2606:4700:4700::1111]:53", wantHost: "2606:4700:4700::1111", wantPort: 53},
	}
	for _, tc := range cases {
		host, port := splitDNSServer(tc.in)
		if host != tc.wantHost || port != tc.wantPort {
			t.Fatalf("splitDNSServer(%q) = (%q,%d), want (%q,%d)", tc.in, host, port, tc.wantHost, tc.wantPort)
		}
	}
}

func TestBuildProxyModeConfig_CustomDNSHaveUniqueTags(t *testing.T) {
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Mode:       ProxyModeProxy,
		ListenAddr: "127.0.0.1:14081",
		Proxy:      ProxyConfig{Type: "SS", IP: "example.com", Port: 443, Password: "p"},
		DNSServers: []string{"8.8.8.8", "1.1.1.1"},
	})
	if cfg.DNS == nil {
		t.Fatal("dns missing")
	}
	seenTags := map[string]struct{}{}
	nonLocal := 0
	for _, s := range cfg.DNS.Servers {
		if _, ok := seenTags[s.Tag]; ok {
			t.Fatalf("duplicate dns tag found: %q in %+v", s.Tag, cfg.DNS.Servers)
		}
		seenTags[s.Tag] = struct{}{}
		if s.Type != "local" {
			nonLocal++
		}
	}
	if nonLocal < 2 {
		t.Fatalf("expected at least two custom dns servers, got %+v", cfg.DNS.Servers)
	}
}

func TestBuildProxyModeConfig_CustomDNSDirectUDP(t *testing.T) {
	// proxy-режим: custom DNS servers должны быть прямыми UDP без detour.
	// DNS через detour: proxy создавал circular dependency и ломал все соединения.
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Mode:       ProxyModeProxy,
		ListenAddr: "127.0.0.1:14081",
		Proxy:      ProxyConfig{Type: "TROJAN", IP: "docs.meowmeowcat.top", Port: 7443, Password: "p"},
		DNSServers: []string{"8.8.8.8", "1.1.1.1"},
	})
	if cfg.DNS == nil {
		t.Fatal("dns missing")
	}
	custom := 0
	for _, s := range cfg.DNS.Servers {
		if strings.HasPrefix(s.Tag, "custom-") {
			custom++
			if s.Type != "udp" || s.Detour != "" {
				t.Fatalf("expected direct udp dns (no detour) in proxy mode, got %+v", s)
			}
		}
	}
	if custom != 2 {
		t.Fatalf("expected 2 custom dns servers, got %+v", cfg.DNS.Servers)
	}
}

func TestBuildProxyModeConfig_NoDNSRulesInProxyMode(t *testing.T) {
	// proxy-режим: без detour не нужны DNS-правила для домена прокси-сервера.
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Mode:       ProxyModeProxy,
		ListenAddr: "127.0.0.1:14081",
		Proxy:      ProxyConfig{Type: "TROJAN", IP: "docs.meowmeowcat.top", Port: 7443, Password: "p"},
	})
	if cfg.DNS == nil {
		t.Fatal("dns missing")
	}
	if len(cfg.DNS.Rules) != 0 {
		t.Fatalf("expected no dns rules in proxy mode, got rules=%+v", cfg.DNS.Rules)
	}
}

// TestBuildProxyModeConfig_AppWhitelistEnablesFindProcess verifies that when
// the user has app exclusions, the proxy-mode config tells sing-box to
// resolve PID/process for every connection. Without find_process the
// process_path_regex rules wouldn't fire and excluded apps would still
// route through the proxy.
func TestBuildProxyModeConfig_AppWhitelistEnablesFindProcess(t *testing.T) {
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Mode:         ProxyModeProxy,
		ListenAddr:   "127.0.0.1:14081",
		Proxy:        ProxyConfig{Type: "TROJAN", IP: "1.2.3.4", Port: 443, Password: "p"},
		AppWhitelist: []string{"steam.exe", "steamwebhelper.exe"},
	})
	if cfg.Route == nil {
		t.Fatal("route missing")
	}
	if !cfg.Route.FindProcess {
		t.Fatal("expected find_process=true when app whitelist is set, got false")
	}
}

func TestBuildProxyModeConfig_NoAppWhitelistOmitsFindProcess(t *testing.T) {
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Mode:       ProxyModeProxy,
		ListenAddr: "127.0.0.1:14081",
		Proxy:      ProxyConfig{Type: "TROJAN", IP: "1.2.3.4", Port: 443, Password: "p"},
	})
	if cfg.Route == nil {
		t.Fatal("route missing")
	}
	if cfg.Route.FindProcess {
		t.Fatal("expected find_process=false when whitelist empty, got true")
	}
}

func TestOverlappingProbeDomains(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "empty", in: nil, want: nil},
		{name: "no overlap", in: []string{"example.com", "github.com"}, want: nil},
		{name: "exact match", in: []string{"connectivitycheck.gstatic.com"}, want: []string{"connectivitycheck.gstatic.com"}},
		{name: "parent suffix matches", in: []string{"gstatic.com"}, want: []string{"gstatic.com"}},
		{name: "leading dot tolerated", in: []string{".gstatic.com"}, want: []string{".gstatic.com"}},
		{name: "case insensitive", in: []string{"GSTATIC.COM"}, want: []string{"GSTATIC.COM"}},
		{name: "non-parent does not match", in: []string{"gstatic.example.com"}, want: nil},
		{name: "dedupes", in: []string{"gstatic.com", ".gstatic.com", "gstatic.com"}, want: []string{"gstatic.com", ".gstatic.com"}},
		{name: "multiple probe roots", in: []string{"gstatic.com", "cloudflare.com"}, want: []string{"gstatic.com", "cloudflare.com"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := OverlappingProbeDomains(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

// TestBuildTunnelModeConfig_AppWhitelistGetsLocalDNSRule verifies that
// whitelisted processes resolve DNS via the local system resolver
// instead of the proxy detour. Without this rule, SFTP/SSH clients added
// to the whitelist would still leak DNS through the tunnel and fail with
// connection timeouts (the DNS detour is slower than their handshake).
func TestBuildTunnelModeConfig_AppWhitelistGetsLocalDNSRule(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:         ProxyModeTunnel,
		Proxy:        ProxyConfig{Type: "SS", IP: "1.2.3.4", Port: 443, Password: "p"},
		AppWhitelist: []string{"WinSCP.exe"},
	})
	if cfg.DNS == nil {
		t.Fatal("dns missing")
	}
	var found bool
	for _, r := range cfg.DNS.Rules {
		if r.Server != "local" || len(r.ProcessPathRegex) == 0 {
			continue
		}
		for _, rx := range r.ProcessPathRegex {
			if strings.Contains(rx, "WinSCP") {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatalf("expected dns rule routing whitelisted process to local resolver, got rules=%+v", cfg.DNS.Rules)
	}
}

// TestBuildTunnelModeConfig_NoAppWhitelistNoProcessDNSRule verifies that
// no process-DNS rule is emitted when the whitelist is empty.
func TestBuildTunnelModeConfig_NoAppWhitelistNoProcessDNSRule(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "SS", IP: "1.2.3.4", Port: 443, Password: "p"},
	})
	if cfg.DNS == nil {
		t.Fatal("dns missing")
	}
	for _, r := range cfg.DNS.Rules {
		if len(r.ProcessPathRegex) > 0 {
			t.Fatalf("unexpected process_path_regex dns rule with empty whitelist: %+v", r)
		}
	}
}

// TestBuildTunnelModeConfig_StrictRouteRespectsConfig verifies that the
// strict_route flag on the TUN inbound is driven by EngineConfig.DNSLeakProtection.
// strict_route on Windows installs WFP filters that drop outbound packets
// bypassing the TUN — without it, Windows' Smart Multi-Homed Name Resolution
// leaks parallel DNS queries via the LAN adapter where Russian ISPs
// transparently hijack UDP/53 (Rostelecom, MSK-IX).
func TestBuildTunnelModeConfig_StrictRouteRespectsConfig(t *testing.T) {
	on := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:              ProxyModeTunnel,
		Proxy:             ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		DNSLeakProtection: true,
	})
	if len(on.Inbounds) == 0 || on.Inbounds[0].Type != "tun" {
		t.Fatalf("expected tun inbound, got %+v", on.Inbounds)
	}
	if !on.Inbounds[0].StrictRoute {
		t.Fatalf("expected strict_route=true when DNSLeakProtection on, got %+v", on.Inbounds[0])
	}

	off := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:              ProxyModeTunnel,
		Proxy:             ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		DNSLeakProtection: false,
	})
	if off.Inbounds[0].StrictRoute {
		t.Fatalf("expected strict_route=false when DNSLeakProtection off, got %+v", off.Inbounds[0])
	}
}

// TestBuildTunnelModeConfig_StrictRouteSerializesToJSON guards against a
// silent regression where the strict_route field stops marshalling — that
// would let a "DNS leak protection on" config slip through with strict_route
// missing from the sing-box JSON.
func TestBuildTunnelModeConfig_StrictRouteSerializesToJSON(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:              ProxyModeTunnel,
		Proxy:             ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		DNSLeakProtection: true,
	})
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"strict_route":true`) {
		t.Fatalf("strict_route missing from json:\n%s", string(raw))
	}
}

// TestBuildTunnelModeConfig_TunHasIPv6Address verifies that the TUN inbound
// carries both an IPv4 and an IPv6 address by default. Without an IPv6
// address, strict_route's WFP filter would silently blackhole all IPv6
// traffic — leaving the user without IPv6 connectivity while connected.
func TestBuildTunnelModeConfig_TunHasIPv6Address(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
	})
	if len(cfg.Inbounds) == 0 {
		t.Fatal("missing tun inbound")
	}
	var v4, v6 bool
	for _, a := range cfg.Inbounds[0].Address {
		if strings.Contains(a, ":") {
			v6 = true
		} else if strings.Contains(a, ".") {
			v4 = true
		}
	}
	if !v4 {
		t.Fatalf("expected default IPv4 address in tun, got %+v", cfg.Inbounds[0].Address)
	}
	if !v6 {
		t.Fatalf("expected default IPv6 address in tun (otherwise strict_route blackholes v6), got %+v", cfg.Inbounds[0].Address)
	}
}

// TestBuildTunnelModeConfig_CustomTunIPv6Respected verifies that
// EngineConfig.TunIPv6 overrides the default ULA prefix.
func TestBuildTunnelModeConfig_CustomTunIPv6Respected(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:    ProxyModeTunnel,
		Proxy:   ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		TunIPv6: "fd00:dead:beef::1/64",
	})
	if len(cfg.Inbounds) == 0 {
		t.Fatal("missing tun inbound")
	}
	var found bool
	for _, a := range cfg.Inbounds[0].Address {
		if a == "fd00:dead:beef::1/64" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("custom TunIPv6 not propagated to tun.address, got %+v", cfg.Inbounds[0].Address)
	}
}

// TestBuildRoute_ProxyMode_AppWhitelistGetsDirectRule covers the actual
// excluded-process rule. In proxy mode the mixed inbound terminates the
// connection locally; sing-box looks up the originating PID and matches
// against process_path_regex. The rule must point at the direct outbound
// so the excluded app bypasses the tunnel.
func TestBuildRoute_ProxyMode_AppWhitelistGetsDirectRule(t *testing.T) {
	cfg := EngineConfig{
		Mode:         ProxyModeProxy,
		AppWhitelist: []string{"steam.exe", "steamwebhelper.exe"},
	}
	route := buildRoute(cfg)
	if route == nil {
		t.Fatal("route missing")
	}
	var found bool
	for _, r := range route.Rules {
		if r.Outbound != "direct" || len(r.ProcessPathRegex) == 0 {
			continue
		}
		if len(r.ProcessPathRegex) >= 2 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected process_path_regex direct rule with both entries, rules=%+v", route.Rules)
	}
}

func TestBuildAdBlockRuleSets_PrefersLocalFiles(t *testing.T) {
	dir := t.TempDir()
	rsDir := filepath.Join(dir, "filter", "rulesets")
	if err := os.MkdirAll(rsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, minLocalSRSBytes+1)
	for i := range payload {
		payload[i] = 0x01
	}
	if err := os.WriteFile(filepath.Join(rsDir, "ads.srs"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	sets := buildAdBlockRuleSets(dir)
	if len(sets) < 1 {
		t.Fatal("expected rule sets")
	}
	if sets[0].Type != "local" || sets[0].LocalOptions.Path == "" {
		t.Fatalf("expected local ads rule-set, got %+v", sets[0])
	}
}

func TestBuildRoute_AdBlock_RejectRuleSets(t *testing.T) {
	cfg := EngineConfig{AdBlock: true}
	route := buildRoute(cfg)
	if route == nil {
		t.Fatal("expected route")
	}
	if len(route.RuleSet) < 1 {
		t.Fatalf("expected rule_set definitions, got %+v", route.RuleSet)
	}
	var rejectFound bool
	for _, r := range route.Rules {
		if r.Action == "reject" && len(r.RuleSet) > 0 {
			rejectFound = true
			break
		}
	}
	if !rejectFound {
		t.Fatalf("expected reject rule with rule_set, rules=%+v", route.Rules)
	}
}

func TestBuildDNS_AdBlock_RejectRuleSets(t *testing.T) {
	cfg := EngineConfig{Mode: ProxyModeProxy, AdBlock: true}
	dns := buildDNS(cfg)
	if dns == nil {
		t.Fatal("expected dns")
	}
	var rejectFound bool
	for _, r := range dns.Rules {
		if r.Action == "reject" && len(r.RuleSet) > 0 {
			rejectFound = true
			break
		}
	}
	if !rejectFound {
		t.Fatalf("expected dns reject rule, rules=%+v", dns.Rules)
	}
}

func TestBuildRoute_AdBlockMITM_BrowserRules(t *testing.T) {
	cfg := EngineConfig{AdBlock: true, MITMPort: 18080}
	route := buildRoute(cfg)
	if !route.FindProcess {
		t.Fatal("expected find_process for MITM browser rules")
	}
	var mitmTCP, quicReject bool
	for _, r := range route.Rules {
		if r.Outbound == adBlockMITMOutbound && len(r.ProcessPathRegex) > 0 {
			mitmTCP = true
		}
		if r.Action == "reject" && len(r.Network) == 1 && r.Network[0] == "udp" {
			quicReject = true
		}
	}
	if !mitmTCP {
		t.Fatalf("expected MITM outbound rule, rules=%+v", route.Rules)
	}
	if !quicReject {
		t.Fatalf("expected QUIC reject for browsers, rules=%+v", route.Rules)
	}
}

func TestBuildProxyModeConfig_AdBlock_OutboundMITM(t *testing.T) {
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		AdBlock:  true,
		MITMPort: 18080,
		LocalPort: 14081,
		Proxy: ProxyConfig{IP: "1.2.3.4", Port: 1080, Type: "socks5"},
	})
	var mitmOutbound bool
	for _, o := range cfg.Outbounds {
		if o.Tag == adBlockMITMOutbound {
			mitmOutbound = true
			if o.ServerPort != 18080 {
				t.Fatalf("mitm port: got %d", o.ServerPort)
			}
		}
	}
	if !mitmOutbound {
		t.Fatalf("expected adblock-mitm outbound, outbounds=%+v", cfg.Outbounds)
	}
}

func TestBuildProxyModeConfig_AdBlock_SingBoxParses(t *testing.T) {
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		AdBlock:   true,
		MITMPort:  18080,
		LocalPort: 14081,
		Mode:      ProxyModeProxy,
		ListenAddr: "127.0.0.1:14081",
		Proxy:     ProxyConfig{IP: "1.2.3.4", Port: 1080, Type: "socks5"},
	})
	j, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(j), `"rule_set"`) {
		t.Fatalf("expected rule_set in config: %s", string(j))
	}
	ctx := include.Context(context.Background())
	var opt option.Options
	if err := singjson.UnmarshalContext(ctx, j, &opt); err != nil {
		t.Fatalf("sing-box decode adblock config: %v\n%s", err, j)
	}
}
