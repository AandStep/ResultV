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
	"encoding/base64"
	"encoding/json"
	"fmt"
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

// TestBuildRoute_SmartMode_FinalDirect pins the core of the Smart-mode wiring:
// in Smart mode the catch-all is direct, so any domain not explicitly tunneled
// (i.e. not on the block-list) leaves on the real IP. This is what stops a
// non-blocked site from seeing the datacenter IP (the BLOCK_403 symptom) and
// lets a video CDN load without a manual exception.
func TestBuildRoute_SmartMode_FinalDirect(t *testing.T) {
	cfg := EngineConfig{
		Mode:        ProxyModeTunnel,
		RoutingMode: ModeSmart,
		Proxy:       ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
	}
	route := buildRoute(cfg)
	if route == nil {
		t.Fatal("expected non-nil route")
	}
	if route.Final != "direct" {
		t.Fatalf("smart mode must default to direct, got Final=%q", route.Final)
	}
}

// TestBuildRoute_GlobalMode_FinalProxy guards the regression boundary: Global
// (and Whitelist) keep proxy as the catch-all, and the block-list must not
// leak route rules into non-Smart modes.
func TestBuildRoute_GlobalMode_FinalProxy(t *testing.T) {
	cfg := EngineConfig{
		Mode:           ProxyModeTunnel,
		RoutingMode:    ModeGlobal,
		Proxy:          ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		BlockedDomains: []string{"instagram.com"},
	}
	route := buildRoute(cfg)
	if route.Final != "proxy" {
		t.Fatalf("global mode must default to proxy, got Final=%q", route.Final)
	}
	for _, r := range route.Rules {
		for _, d := range r.DomainSuffix {
			if d == "instagram.com" {
				t.Fatalf("block-list must not produce route rules outside smart mode, rules=%+v", route.Rules)
			}
		}
	}
}

// TestBuildRoute_SmartMode_BlockedDomainsTunneled verifies the block-list is
// routed through the proxy outbound in Smart mode.
func TestBuildRoute_SmartMode_BlockedDomainsTunneled(t *testing.T) {
	cfg := EngineConfig{
		Mode:           ProxyModeTunnel,
		RoutingMode:    ModeSmart,
		Proxy:          ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		BlockedDomains: []string{"instagram.com", "discord.com"},
	}
	route := buildRoute(cfg)
	var found bool
	for _, r := range route.Rules {
		if r.Outbound != "proxy" || len(r.DomainSuffix) == 0 {
			continue
		}
		has := map[string]bool{}
		for _, d := range r.DomainSuffix {
			has[d] = true
		}
		if has["instagram.com"] && has["discord.com"] {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("smart mode must route blocked domains through proxy, rules=%+v", route.Rules)
	}
}

// TestBuildRoute_SmartMode_NestedExceptionTunneled mirrors
// router_test.go TestShouldProxy_SmartMode_NestedExceptions: with whitelist
// [.ru, avito.ru] the even (double) match makes avito.ru a nested exception
// that must tunnel, while the catch-all stays direct.
func TestBuildRoute_SmartMode_NestedExceptionTunneled(t *testing.T) {
	cfg := EngineConfig{
		Mode:        ProxyModeTunnel,
		RoutingMode: ModeSmart,
		Proxy:       ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		Whitelist:   []string{".ru", "avito.ru"},
	}
	route := buildRoute(cfg)
	var avitoProxy bool
	for _, r := range route.Rules {
		if r.Outbound == "proxy" && len(r.DomainSuffix) == 1 && r.DomainSuffix[0] == "avito.ru" {
			avitoProxy = true
		}
	}
	if !avitoProxy {
		t.Fatalf("smart mode nested exception avito.ru must tunnel, rules=%+v", route.Rules)
	}
	if route.Final != "direct" {
		t.Fatalf("smart mode must keep Final=direct, got %q", route.Final)
	}
}

// TestBuildRoute_SmartMode_BlockedWinsOverWhitelist encodes Router.ShouldProxy's
// precedence: a blocked domain under an odd (single) whitelist match still
// tunnels. In first-match routing that means the blocked rule must precede the
// whitelist direct rule.
func TestBuildRoute_SmartMode_BlockedWinsOverWhitelist(t *testing.T) {
	cfg := EngineConfig{
		Mode:           ProxyModeTunnel,
		RoutingMode:    ModeSmart,
		Proxy:          ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		Whitelist:      []string{".com"},
		BlockedDomains: []string{"instagram.com"},
	}
	route := buildRoute(cfg)
	blockedIdx, comDirectIdx := -1, -1
	for i, r := range route.Rules {
		if r.Outbound == "proxy" {
			for _, d := range r.DomainSuffix {
				if d == "instagram.com" {
					blockedIdx = i
				}
			}
		}
		if r.Outbound == "direct" {
			for _, d := range r.DomainSuffix {
				if d == "com" {
					comDirectIdx = i
				}
			}
		}
	}
	if blockedIdx == -1 {
		t.Fatalf("expected blocked instagram.com → proxy rule, rules=%+v", route.Rules)
	}
	if comDirectIdx == -1 {
		t.Fatalf("expected whitelist com → direct rule, rules=%+v", route.Rules)
	}
	if blockedIdx > comDirectIdx {
		t.Fatalf("blocked rule (idx=%d) must precede whitelist direct rule (idx=%d) so blocked wins, rules=%+v",
			blockedIdx, comDirectIdx, route.Rules)
	}
}

// TestBuildRoute_SmartMode_BlockedCIDRsTunneled verifies IP-only blocked ranges
// (Telegram MTProto) get an ip_cidr → proxy rule in Smart mode. Telegram's
// native client has no domain/SNI, so this rule is the only thing that pulls it
// through the tunnel.
func TestBuildRoute_SmartMode_BlockedCIDRsTunneled(t *testing.T) {
	cfg := EngineConfig{
		Mode:         ProxyModeTunnel,
		RoutingMode:  ModeSmart,
		Proxy:        ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		BlockedCIDRs: []string{"149.154.160.0/20", "91.108.4.0/22"},
	}
	route := buildRoute(cfg)
	var found bool
	for _, r := range route.Rules {
		if r.Outbound != "proxy" || len(r.IPCidr) == 0 {
			continue
		}
		has := map[string]bool{}
		for _, c := range r.IPCidr {
			has[c] = true
		}
		if has["149.154.160.0/20"] && has["91.108.4.0/22"] {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("smart mode must route blocked CIDRs through proxy, rules=%+v", route.Rules)
	}
}

// TestBuildRoute_GlobalMode_NoBlockedCIDRRule guards that the IP block-list
// stays inert outside Smart mode.
func TestBuildRoute_GlobalMode_NoBlockedCIDRRule(t *testing.T) {
	cfg := EngineConfig{
		Mode:         ProxyModeTunnel,
		RoutingMode:  ModeGlobal,
		Proxy:        ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		BlockedCIDRs: []string{"149.154.160.0/20"},
	}
	route := buildRoute(cfg)
	for _, r := range route.Rules {
		for _, c := range r.IPCidr {
			if c == "149.154.160.0/20" {
				t.Fatalf("global mode must not emit blocked-CIDR rule, rules=%+v", route.Rules)
			}
		}
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

// TestBuildRoute_TunnelMode_DomainServerExcludesResolvedIPs covers the domain-
// addressed-server gap behind the 2026-06 github-issue (EOF flood / x509-github on
// a domain VLESS server in TUN, while a literal-IP server worked). When the server
// is a domain whose backend IPs were pinned at connect (ResolvedIPs), buildRoute
// must emit an ip_cidr → direct rule for EACH pinned IP — not only the fragile
// domain → direct rule — so the outbound's own dial-by-IP to the server can never
// loop back into the TUN. Mirrors TestBuildRoute_TunnelMode_ServerIPBypassBeforeSniff.
func TestBuildRoute_TunnelMode_DomainServerExcludesResolvedIPs(t *testing.T) {
	cfg := EngineConfig{
		Mode: ProxyModeTunnel,
		Proxy: ProxyConfig{
			Type:        "vless",
			IP:          "frankfurt.example.com",
			Port:        443,
			ResolvedIPs: []string{"203.0.113.7", "203.0.113.8"},
		},
	}
	route := buildRoute(cfg)
	if route == nil {
		t.Fatal("expected non-nil route")
	}

	foundCidr := map[string]int{}
	sniffIdx := -1
	for i, r := range route.Rules {
		if r.Action == "sniff" && sniffIdx == -1 {
			sniffIdx = i
		}
		if r.Outbound == "direct" {
			for _, cidr := range r.IPCidr {
				foundCidr[cidr] = i
			}
		}
	}
	for _, cidr := range []string{"203.0.113.7/32", "203.0.113.8/32"} {
		idx, ok := foundCidr[cidr]
		if !ok {
			t.Fatalf("expected ip_cidr %s → direct for domain server's resolved IP, rules=%+v", cidr, route.Rules)
		}
		if sniffIdx == -1 || idx >= sniffIdx {
			t.Fatalf("server IP exclude %s (idx=%d) must precede sniff (idx=%d) to prevent routing loops", cidr, idx, sniffIdx)
		}
	}
}

// TestOutboundTLSDiagnostic verifies the connect-time diagnostic that exposes
// whether the BUILT outbound carries an active Reality block. A vless+reality
// server logging "tls" (plain TLS, no reality) is the smoking gun for the
// x509-github failure class — the reporter's log reveals the strip without a repro.
func TestOutboundTLSDiagnostic(t *testing.T) {
	cases := []struct {
		name  string
		proxy ProxyConfig
		want  string
	}{
		{
			name:  "vless reality",
			proxy: ProxyConfig{Type: "vless", IP: "1.2.3.4", Port: 443, Extra: json.RawMessage(`{"uuid":"u","security":"reality","pbk":"k","sid":"5678","sni":"example.com"}`)},
			want:  "reality",
		},
		{
			name:  "vless plain tls",
			proxy: ProxyConfig{Type: "vless", IP: "1.2.3.4", Port: 443, Extra: json.RawMessage(`{"uuid":"u","security":"tls","sni":"example.com"}`)},
			want:  "tls",
		},
		{
			name:  "shadowsocks no tls",
			proxy: ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p", Extra: json.RawMessage(`{"method":"aes-256-gcm"}`)},
			want:  "none",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := outboundTLSDiagnostic(c.proxy); got != c.want {
				t.Fatalf("outboundTLSDiagnostic(%s) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestResolvedFingerprint_UnknownFallsBackToChrome guards the engine against a
// typo'd/truncated uTLS fingerprint (e.g. a copy-paste that clipped "chrome" to
// "ch"): sing-box rejects an unknown fingerprint at instance creation with
// "unknown uTLS fingerprint" and the whole connect fails. Unknown values must
// fall back to a valid fingerprint instead of crashing the start.
func TestResolvedFingerprint_UnknownFallsBackToChrome(t *testing.T) {
	fp := func(v string) string { return resolvedFingerprint(map[string]interface{}{"fp": v}) }
	cases := map[string]string{
		"ch":       "chrome", // clipped paste — the real-world failure
		"bogus123": "chrome",
		"ChRoMe":   "chrome", // normalised
		"qq":       "qq",
		"firefox":  "firefox",
		"randomized": "randomized",
		"none":     "", // explicit opt-out keeps bare Go fingerprint
	}
	for in, want := range cases {
		if got := fp(in); got != want {
			t.Fatalf("resolvedFingerprint(fp=%q) = %q, want %q", in, got, want)
		}
	}
}

// TestParseSubscription_PreservesRealityAcrossEntries is the decisive check for
// the issue's "reality stripped → x509" class: it runs a diverse base64
// subscription (the real path the issue used) through ParseSubscriptionBody and
// verifies, per server, that the BUILT outbound keeps the right TLS state AND the
// exact reality params (pbk/sni) belong to the right host — i.e. nothing shifts
// between entries. Two different reality key-sets + security=none across five
// transports. If this passes, the parser is not the culprit; if it fails, we found it.
func TestParseSubscription_PreservesRealityAcrossEntries(t *testing.T) {
	links := []string{
		`vless://02af3b31-2696-4b1c-a7d3-19ecf6318f64@84.75.161.19:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=auto.malina24.xyz&fp=chrome&pbk=V6FabatADtcX7aO9KMjGCadJC4LuQ_5nRViab-z-nFQ&sid=5678&type=tcp&headerType=none#dk`,
		`vless://0c986712-ccb1-4c8f-8e25-6a209db3c466@45.138.101.53:443?encryption=none&fp=chrome&mode=auto&path=%2Fxhttp-proxy&pbk=akcAXaW7GNwXT6bTrtD6oZIY9rdEzW4TfZvYqDRzsTc&security=reality&sid=a8f3c1d7e9b4c2f6&sni=5post-gate.x5.ru&type=xhttp#ee`,
		`vless://0c986712-ccb1-4c8f-8e25-6a209db3c466@5.39.250.173:443?encryption=none&security=reality&sni=5post-gate.x5.ru&fp=qq&pbk=akcAXaW7GNwXT6bTrtD6oZIY9rdEzW4TfZvYqDRzsTc&sid=a8f3c1d7e9b4c2f6&spx=%2F&type=xhttp&path=%2Fxhttp-proxy&mode=auto#fi`,
		`vless://2b446337-5874-4d9c-8d27-9ce0eac2389c@185.216.71.193:10928?mode=auto&path=/&security=none&encryption=none&type=xhttp#x1`,
		`vless://2b446337-5874-4d9c-8d27-9ce0eac2389c@185.216.71.193:26093?mode=gun&security=none&encryption=none&type=grpc#x2`,
		`vless://2b446337-5874-4d9c-8d27-9ce0eac2389c@185.216.71.193:43144?path=/&security=none&encryption=none&type=httpupgrade#x3`,
		`vless://2b446337-5874-4d9c-8d27-9ce0eac2389c@185.216.71.193:52526?security=none&encryption=none&headerType=none&type=tcp#x4`,
		`vless://2b446337-5874-4d9c-8d27-9ce0eac2389c@185.216.71.193:57499?path=/&security=none&encryption=none&type=ws#x5`,
	}

	type want struct {
		tls string // "reality" | "tls" | "none"
		pbk string // expected reality public key (reality only)
		sni string // expected reality server_name (reality only)
	}
	expected := map[string]want{
		"84.75.161.19:443":     {tls: "reality", pbk: "V6FabatADtcX7aO9KMjGCadJC4LuQ_5nRViab-z-nFQ", sni: "auto.malina24.xyz"},
		"45.138.101.53:443":    {tls: "reality", pbk: "akcAXaW7GNwXT6bTrtD6oZIY9rdEzW4TfZvYqDRzsTc", sni: "5post-gate.x5.ru"},
		"5.39.250.173:443":     {tls: "reality", pbk: "akcAXaW7GNwXT6bTrtD6oZIY9rdEzW4TfZvYqDRzsTc", sni: "5post-gate.x5.ru"},
		"185.216.71.193:10928": {tls: "none"},
		"185.216.71.193:26093": {tls: "none"},
		"185.216.71.193:43144": {tls: "none"},
		"185.216.71.193:52526": {tls: "none"},
		"185.216.71.193:57499": {tls: "none"},
	}

	body := base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))
	entries, err := ParseSubscriptionBody(body)
	if err != nil {
		t.Fatalf("ParseSubscriptionBody: %v", err)
	}

	seen := map[string]bool{}
	for _, e := range entries {
		key := fmt.Sprintf("%s:%d", e.IP, e.Port)
		exp, ok := expected[key]
		if !ok {
			t.Fatalf("unexpected parsed server %s (name=%q) — possible entry shift, extra=%s", key, e.Name, string(e.Extra))
		}
		seen[key] = true

		pc := ProxyConfig{Type: e.Type, IP: e.IP, Port: e.Port, Username: e.Username, Password: e.Password, URI: e.URI, Extra: e.Extra}
		if got := outboundTLSDiagnostic(pc); got != exp.tls {
			t.Fatalf("%s: tls=%q, want %q (reality stripped/shifted?) extra=%s", key, got, exp.tls, string(e.Extra))
		}
		if exp.tls == "reality" {
			var ex map[string]any
			if err := json.Unmarshal(e.Extra, &ex); err != nil {
				t.Fatalf("%s: unmarshal extra: %v", key, err)
			}
			if got, _ := ex["pbk"].(string); got != exp.pbk {
				t.Fatalf("%s: pbk=%q, want %q — reality key shifted between entries", key, got, exp.pbk)
			}
			if got, _ := ex["sni"].(string); got != exp.sni {
				t.Fatalf("%s: sni=%q, want %q — reality SNI shifted between entries", key, got, exp.sni)
			}
		}
	}
	for key := range expected {
		if !seen[key] {
			t.Fatalf("server %s was dropped by the parser, entries=%d", key, len(entries))
		}
	}
}

// TestServerEndpointUnresolvable guards the connect-time fail-fast: a domain
// server in TUN with no pinned IP would force sing-box to dial it via the
// censored OS resolver (loop → EOF, or poisoned IP → x509-github). The connect
// path aborts up front in that case; literal-IP servers and proxy mode are never
// gated.
func TestServerEndpointUnresolvable(t *testing.T) {
	cases := []struct {
		name  string
		proxy ProxyConfig
		mode  ProxyMode
		want  bool
	}{
		{name: "domain no pin tunnel", proxy: ProxyConfig{IP: "frankfurt.example.com"}, mode: ProxyModeTunnel, want: true},
		{name: "domain pinned tunnel", proxy: ProxyConfig{IP: "frankfurt.example.com", ResolvedIPs: []string{"203.0.113.7"}}, mode: ProxyModeTunnel, want: false},
		{name: "domain single-pin tunnel", proxy: ProxyConfig{IP: "frankfurt.example.com", ResolvedIP: "203.0.113.7"}, mode: ProxyModeTunnel, want: false},
		{name: "literal ip tunnel", proxy: ProxyConfig{IP: "203.0.113.7"}, mode: ProxyModeTunnel, want: false},
		{name: "domain no pin proxy mode", proxy: ProxyConfig{IP: "frankfurt.example.com"}, mode: ProxyModeProxy, want: false},
		{name: "empty ip tunnel", proxy: ProxyConfig{IP: ""}, mode: ProxyModeTunnel, want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := serverEndpointUnresolvable(c.proxy, c.mode); got != c.want {
				t.Fatalf("serverEndpointUnresolvable(%+v, %v) = %v, want %v", c.proxy, c.mode, got, c.want)
			}
		})
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

// TestBuildTunnelModeConfig_LoopbackProbeInbound pins the 2026-06 fix for
// false kill-switch trips: tunnel mode must expose a loopback-only mixed
// inbound so health probes resolve hostnames remotely via sing-box instead of
// the OS resolver (which the session itself degrades via the DNS override +
// strict_route). The TUN inbound must stay first — other tests and the engine
// teardown logic index Inbounds[0].
func TestBuildTunnelModeConfig_LoopbackProbeInbound(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:      ProxyModeTunnel,
		Proxy:     ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		LocalPort: 14081,
	})
	if len(cfg.Inbounds) != 2 {
		t.Fatalf("expected tun + probe inbounds, got %+v", cfg.Inbounds)
	}
	if cfg.Inbounds[0].Type != "tun" {
		t.Fatalf("tun inbound must stay first, got %+v", cfg.Inbounds[0])
	}
	probe := cfg.Inbounds[1]
	if probe.Type != "mixed" {
		t.Fatalf("expected mixed probe inbound, got %+v", probe)
	}
	if probe.Listen != "127.0.0.1" {
		t.Fatalf("probe inbound must bind loopback only, got listen=%q", probe.Listen)
	}
	if probe.ListenPort != 14081 {
		t.Fatalf("probe inbound must use EngineConfig.LocalPort, got %d", probe.ListenPort)
	}
}

// Without an explicit LocalPort the probe inbound still comes up on a free
// port instead of silently disappearing (the watchdog falls back to the
// OS-resolver probe only when no local port exists).
func TestBuildTunnelModeConfig_LoopbackProbeInboundDefaultPort(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:  ProxyModeTunnel,
		Proxy: ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
	})
	if len(cfg.Inbounds) != 2 {
		t.Fatalf("expected tun + probe inbounds, got %+v", cfg.Inbounds)
	}
	if cfg.Inbounds[1].ListenPort == 0 {
		t.Fatalf("probe inbound must get a free port, got %+v", cfg.Inbounds[1])
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

// TestBuildRoute_ExplicitBrowserExclusionGetsDirectRule guards the other half
// of the browser fix: descendant auto-capture filters browsers out (so a
// launcher can't drag the user's browser out of the VPN), but a browser the
// user EXPLICITLY lists as an excluded app must still route direct. The
// process-tree filter only touches discovered descendants, never user roots,
// so the root flows straight into the direct process_path_regex rule.
func TestBuildRoute_ExplicitBrowserExclusionGetsDirectRule(t *testing.T) {
	cfg := EngineConfig{
		Mode:         ProxyModeProxy,
		AppWhitelist: []string{"chrome.exe"},
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
		for _, rx := range r.ProcessPathRegex {
			if strings.Contains(strings.ToLower(rx), "chrome") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("explicit chrome.exe exclusion must produce a direct process rule, rules=%+v", route.Rules)
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

func TestBuildRouteAppForceVPN(t *testing.T) {
	cfg := EngineConfig{
		Mode:         ProxyModeTunnel,
		RoutingMode:  ModeSmart,
		AppForceVPN:  []string{"Discord.exe"},
		AppWhitelist: []string{"steam.exe"},
	}
	route := buildRoute(cfg)

	forceIdx, whitelistIdx := -1, -1
	for i, rule := range route.Rules {
		if len(rule.ProcessPathRegex) == 0 {
			continue
		}
		if rule.Outbound == "proxy" && strings.Contains(rule.ProcessPathRegex[0], "Discord") {
			forceIdx = i
		}
		if rule.Outbound == "direct" && strings.Contains(rule.ProcessPathRegex[0], "steam") {
			whitelistIdx = i
		}
	}
	if forceIdx == -1 {
		t.Fatal("force-VPN process rule missing in tunnel mode")
	}
	if whitelistIdx == -1 {
		t.Fatal("app-whitelist rule missing")
	}
	if forceIdx > whitelistIdx {
		t.Errorf("force-VPN rule (%d) must precede app-whitelist direct rule (%d)", forceIdx, whitelistIdx)
	}
	if !route.FindProcess {
		t.Error("find_process must be enabled when force-VPN list is set")
	}

	cfg.Mode = ProxyModeProxy
	route = buildRoute(cfg)
	for _, rule := range route.Rules {
		if rule.Outbound == "proxy" && len(rule.ProcessPathRegex) > 0 {
			t.Error("force-VPN rule must not be emitted in proxy mode")
		}
	}
}

func TestRoutingListRulesRestrictiveOrder(t *testing.T) {
	dir := t.TempDir()
	// Three cache files so the stat-guard passes.
	for _, id := range []string{"p", "d", "b"} {
		if err := WriteRoutingListRuleSet(dir, id, ParsedRoutingList{Domains: []string{id + ".test"}}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	cfg := EngineConfig{
		Mode:        ProxyModeTunnel,
		RoutingMode: ModeGlobal,
		DataDir:     dir,
		RoutingLists: []RoutingListSpec{
			{Tag: "rl-d", Path: RoutingListCachePath(dir, "d"), Action: "direct"},
			{Tag: "rl-p", Path: RoutingListCachePath(dir, "p"), Action: "proxy"},
			{Tag: "rl-b", Path: RoutingListCachePath(dir, "b"), Action: "block"},
		},
	}
	route := buildRoute(cfg)

	// rule_set entries exist for all three, as local source rule-sets.
	tags := map[string]bool{}
	for _, rs := range route.RuleSet {
		tags[rs.Tag] = true
		if rs.Tag == "rl-p" {
			if rs.Type != "local" || rs.Format != "source" {
				t.Errorf("rl-p rule_set: type=%q format=%q, want local/source", rs.Type, rs.Format)
			}
		}
	}
	for _, want := range []string{"rl-p", "rl-d", "rl-b"} {
		if !tags[want] {
			t.Errorf("missing rule_set tag %q", want)
		}
	}

	// Route-rule order among the three: block before proxy before direct.
	idx := func(tag string) int {
		for i, r := range route.Rules {
			if len(r.RuleSet) == 1 && r.RuleSet[0] == tag {
				return i
			}
		}
		return -1
	}
	ib, ip, id := idx("rl-b"), idx("rl-p"), idx("rl-d")
	if ib < 0 || ip < 0 || id < 0 {
		t.Fatalf("routing-list rules missing: b=%d p=%d d=%d", ib, ip, id)
	}
	if !(ib < ip && ip < id) {
		t.Errorf("restrictive-first order violated: b=%d p=%d d=%d", ib, ip, id)
	}

	// Action/outbound mapping.
	for _, r := range route.Rules {
		if len(r.RuleSet) != 1 {
			continue
		}
		switch r.RuleSet[0] {
		case "rl-b":
			if r.Action != "reject" {
				t.Errorf("block list: action=%q, want reject", r.Action)
			}
		case "rl-p":
			if r.Outbound != "proxy" {
				t.Errorf("proxy list: outbound=%q, want proxy", r.Outbound)
			}
		case "rl-d":
			if r.Outbound != "direct" {
				t.Errorf("direct list: outbound=%q, want direct", r.Outbound)
			}
		}
	}
}

func TestRoutingListRulesBeforeBuiltins(t *testing.T) {
	dir := t.TempDir()
	if err := WriteRoutingListRuleSet(dir, "u", ParsedRoutingList{Domains: []string{"user.test"}}); err != nil {
		t.Fatal(err)
	}
	cfg := EngineConfig{
		Mode:           ProxyModeTunnel,
		RoutingMode:    ModeSmart,
		DataDir:        dir,
		BlockedDomains: []string{"blocked.test"},
		RoutingLists:   []RoutingListSpec{{Tag: "rl-u", Path: RoutingListCachePath(dir, "u"), Action: "direct"}},
	}
	route := buildRoute(cfg)
	userIdx, smartIdx := -1, -1
	for i, r := range route.Rules {
		if len(r.RuleSet) == 1 && r.RuleSet[0] == "rl-u" {
			userIdx = i
		}
		if len(r.DomainSuffix) == 1 && r.DomainSuffix[0] == "blocked.test" {
			smartIdx = i
		}
	}
	if userIdx < 0 || smartIdx < 0 {
		t.Fatalf("rules missing: user=%d smart=%d", userIdx, smartIdx)
	}
	if userIdx > smartIdx {
		t.Errorf("user list rule (%d) must come before built-in Smart rule (%d)", userIdx, smartIdx)
	}
}

func TestRoutingListMissingCacheSkipped(t *testing.T) {
	dir := t.TempDir() // no cache file written
	cfg := EngineConfig{
		Mode:         ProxyModeTunnel,
		RoutingMode:  ModeGlobal,
		DataDir:      dir,
		RoutingLists: []RoutingListSpec{{Tag: "rl-x", Path: RoutingListCachePath(dir, "x"), Action: "proxy"}},
	}
	route := buildRoute(cfg)
	for _, rs := range route.RuleSet {
		if rs.Tag == "rl-x" {
			t.Error("rule_set for a missing cache file must be skipped")
		}
	}
}
