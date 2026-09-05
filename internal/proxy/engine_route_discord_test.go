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

package proxy

import (
	"regexp"
	"strings"
	"testing"
)

// Root cause these tests lock in (measured 2026-08-26 from the user's own
// %APPDATA%\discord\logs\renderer_js*.log, 64 real voice sessions):
//
//	media backend            direct (RU IP)   via VPN   never connected
//	104.29.x.x  :19294-19335        8            19            1
//	35.217.x.x  :50003-50008        0            19           17
//
// Discord hands the media endpoint out at call time as a bare IP, so no domain
// rule can catch it, and the IPs live in shared Cloudflare / Google Cloud space
// that an ip_cidr block-list must not swallow whole. The GCP backend on the
// classic 50000+ voice ports never once survived the direct path. Only a
// process rule catches this reliably.

func discordRuleIndex(rules []SBRouteRule) int {
	for i, r := range rules {
		if r.Action != "route" || r.Outbound != "proxy" {
			continue
		}
		for _, rx := range r.ProcessPathRegex {
			if strings.Contains(strings.ToLower(rx), "discord") {
				return i
			}
		}
	}
	return -1
}

func smartTunnelCfg() EngineConfig {
	return EngineConfig{
		Mode:           ProxyModeTunnel,
		RoutingMode:    ModeSmart,
		Proxy:          ProxyConfig{Type: "ss", IP: "1.2.3.4", Port: 443, Password: "p"},
		BlockedDomains: []string{"discord.com"},
		BlockedCIDRs:   []string{"149.154.160.0/20"},
	}
}

// Smart mode must tunnel Discord by process: its voice media is UDP to a bare
// IP that neither the domain rule-set nor the ip_cidr list can match.
func TestBuildRoute_SmartMode_DiscordRoutedByProcess(t *testing.T) {
	route := buildRoute(smartTunnelCfg())
	idx := discordRuleIndex(route.Rules)
	if idx < 0 {
		t.Fatalf("smart+tunnel must route Discord through proxy by process, rules=%+v", route.Rules)
	}
	rx := route.Rules[idx].ProcessPathRegex
	// The rule must cover the real Discord binaries, matched on the full path
	// exactly the way sing-box reports it on Windows.
	for _, path := range []string{
		`C:\Users\andbe\AppData\Local\Discord\app-1.0.9253\Discord.exe`,
		`C:\Users\andbe\AppData\Local\DiscordPTB\app-1.0.99\DiscordPTB.exe`,
		`C:\Users\andbe\AppData\Local\DiscordCanary\app-1.0.99\DiscordCanary.exe`,
	} {
		matched := false
		for _, expr := range rx {
			re, err := regexp.Compile(expr)
			if err != nil {
				t.Fatalf("process_path_regex %q does not compile: %v", expr, err)
			}
			if re.MatchString(path) {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("no process_path_regex in %v matches %s", rx, path)
		}
	}
}

// The rule carries no network filter: the user asked for the whole Discord
// process family through the tunnel, not just its media half.
func TestBuildRoute_SmartMode_DiscordRuleCoversAllNetworks(t *testing.T) {
	route := buildRoute(smartTunnelCfg())
	idx := discordRuleIndex(route.Rules)
	if idx < 0 {
		t.Fatal("discord rule missing")
	}
	if len(route.Rules[idx].Network) != 0 {
		t.Fatalf("discord route rule must not filter by network, got %v", route.Rules[idx].Network)
	}
}

// find_process gates every process_path_regex rule. Without it the built-in
// Discord rule is inert and the bug silently comes back.
func TestBuildTunnelModeConfig_DiscordRuleEnablesFindProcess(t *testing.T) {
	route := buildRoute(smartTunnelCfg())
	if !route.FindProcess {
		t.Fatal("expected find_process=true so the built-in Discord rule can match")
	}
}

// An explicit user exclusion must still win: the app-whitelist direct rule is
// emitted before the built-in, so "Discord в обход VPN" stays honoured.
func TestBuildRoute_SmartMode_UserExclusionBeatsBuiltinDiscordRule(t *testing.T) {
	cfg := smartTunnelCfg()
	cfg.AppWhitelist = []string{"discord.exe"}
	route := buildRoute(cfg)

	directIdx := -1
	for i, r := range route.Rules {
		if r.Outbound != "direct" || r.Action != "route" {
			continue
		}
		for _, rx := range r.ProcessPathRegex {
			if strings.Contains(strings.ToLower(rx), "discord") {
				directIdx = i
			}
		}
	}
	if directIdx < 0 {
		t.Fatal("user exclusion for discord.exe produced no direct rule")
	}
	proxyIdx := discordRuleIndex(route.Rules)
	if proxyIdx >= 0 && proxyIdx < directIdx {
		t.Fatalf("built-in Discord rule at %d shadows the user exclusion at %d", proxyIdx, directIdx)
	}
}

// The built-in must land before the smart rule-set / blocked-CIDR rules so the
// process decision is taken first, and after the sniff/DNS infrastructure.
func TestBuildRoute_SmartMode_DiscordRuleOrdering(t *testing.T) {
	route := buildRoute(smartTunnelCfg())
	idx := discordRuleIndex(route.Rules)
	if idx < 0 {
		t.Fatal("discord rule missing")
	}
	for i, r := range route.Rules {
		if r.Action == "hijack-dns" && i > idx {
			t.Fatalf("discord rule at %d precedes hijack-dns at %d", idx, i)
		}
		if len(r.IPCidr) > 0 && r.Outbound == "proxy" && i < idx {
			t.Fatalf("blocked-CIDR rule at %d precedes discord rule at %d", i, idx)
		}
		if len(r.DomainSuffix) > 0 && r.Outbound == "proxy" && i < idx {
			t.Fatalf("blocked-domain rule at %d precedes discord rule at %d", i, idx)
		}
	}
}

// Discord media never uses UDP/443 — the observed ports are 19294-19335 and
// 50003-50008 — so the QUIC reject that shadows every other proxy-bound
// selector must shadow this one too, keeping the CDN h3 fallback working.
func TestBuildRoute_SmartMode_DiscordGetsQUICReject(t *testing.T) {
	route := buildRoute(smartTunnelCfg())
	idx := discordRuleIndex(route.Rules)
	if idx < 0 {
		t.Fatal("discord rule missing")
	}
	found := false
	for i, r := range route.Rules {
		if r.Action != "reject" || len(r.ProcessPathRegex) == 0 {
			continue
		}
		for _, rx := range r.ProcessPathRegex {
			if !strings.Contains(strings.ToLower(rx), "discord") {
				continue
			}
			if i > idx {
				t.Fatalf("QUIC reject at %d must precede the route rule at %d", i, idx)
			}
			if len(r.Port) != 1 || r.Port[0] != 443 {
				t.Fatalf("QUIC reject must target port 443 only, got %v", r.Port)
			}
			if len(r.Network) != 1 || r.Network[0] != "udp" {
				t.Fatalf("QUIC reject must target udp only, got %v", r.Network)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no udp/443 reject shadowing the built-in Discord rule")
	}
}

// Global mode already sends everything through the proxy, and proxy mode never
// sees apps that ignore the system proxy — the built-in belongs to neither.
func TestBuildRoute_DiscordRuleIsSmartTunnelOnly(t *testing.T) {
	global := smartTunnelCfg()
	global.RoutingMode = ModeGlobal
	if idx := discordRuleIndex(buildRoute(global).Rules); idx >= 0 {
		t.Fatal("global mode must not emit the built-in Discord rule")
	}

	proxyMode := smartTunnelCfg()
	proxyMode.Mode = ProxyModeProxy
	if idx := discordRuleIndex(buildRoute(proxyMode).Rules); idx >= 0 {
		t.Fatal("proxy mode must not emit the built-in Discord rule")
	}
}

// The rule reaches the real pinned core, not just our struct: sing-box decodes
// with DisallowUnknownFields, so a malformed rule is a dead engine for every
// node, not a knob that quietly does nothing.
func TestBuildTunnelModeConfig_DiscordRuleAcceptedByCore(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Mode:           ProxyModeTunnel,
		RoutingMode:    ModeSmart,
		Proxy:          ProxyConfig{Type: "TROJAN", IP: "1.2.3.4", Port: 443, Password: "p"},
		BlockedDomains: []string{"discord.com"},
	})
	if idx := discordRuleIndex(cfg.Route.Rules); idx < 0 {
		t.Fatal("built-in Discord rule missing from the tunnel config")
	}
	assertCoreAcceptsConfig(t, cfg)
}
