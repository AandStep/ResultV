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
	"regexp"
	"slices"
	"testing"
)

const torrentClientExe = `C:\Program Files\qBittorrent\qbittorrent.exe`

func torrentProtocolRuleIndex(rules []SBRouteRule) int {
	for i, r := range rules {
		if r.Action == "route" && r.Outbound == "direct" && slices.Contains(r.Protocol, "bittorrent") {
			return i
		}
	}
	return -1
}

func torrentClientRuleIndex(t *testing.T, rules []SBRouteRule) int {
	t.Helper()
	for i, r := range rules {
		if r.Action != "route" || r.Outbound != "direct" {
			continue
		}
		for _, raw := range r.ProcessPathRegex {
			rx, err := regexp.Compile(raw)
			if err != nil {
				t.Fatalf("regex does not compile: %q: %v", raw, err)
			}
			if rx.MatchString(torrentClientExe) {
				return i
			}
		}
	}
	return -1
}

func blockListProxyRuleIndex(rules []SBRouteRule) int {
	for i, r := range rules {
		if r.Action == "route" && r.Outbound == "proxy" && slices.Contains(r.DomainSuffix, "blocked.test") {
			return i
		}
	}
	return -1
}

func adaptiveTorrentConfig() EngineConfig {
	cfg := adaptiveTunnelConfig()
	cfg.BlockedDomains = []string{"blocked.test"}
	return cfg
}

// A peer is a bare address nothing knows about, so the adaptive engine races
// it through the node. The node's own torrent blocker then bans the client's
// address for fifteen minutes and every tunnelled connection dies with it.
func TestAdaptiveSmartSendsBitTorrentDirect(t *testing.T) {
	route := buildRoute(adaptiveTorrentConfig())

	proto := torrentProtocolRuleIndex(route.Rules)
	if proto < 0 {
		t.Fatalf("no bittorrent -> direct rule, rules=%+v", route.Rules)
	}
	if list := blockListProxyRuleIndex(route.Rules); list < 0 || proto > list {
		t.Errorf("sniffed bittorrent (idx=%d) must precede the block-list (idx=%d)", proto, list)
	}

	app := torrentClientRuleIndex(t, route.Rules)
	if app < 0 {
		t.Fatalf("no torrent client -> direct rule, rules=%+v", route.Rules)
	}
	// A tracker on the block-list still has to reach the node.
	if list := blockListProxyRuleIndex(route.Rules); app < list {
		t.Errorf("torrent client rule (idx=%d) must come after the block-list (idx=%d)", app, list)
	}
	if !route.FindProcess {
		t.Error("find_process must be on for the torrent client rule")
	}
}

func TestTorrentClientsForcedThroughVPNStayInTheTunnel(t *testing.T) {
	cfg := adaptiveTorrentConfig()
	cfg.AppForceVPN = []string{"qbittorrent.exe"}
	route := buildRoute(cfg)

	vpn := familyRuleIndex(route.Rules, familyTagVPN, "proxy")
	if vpn < 0 {
		t.Fatal("force-VPN family rule missing")
	}
	if app := torrentClientRuleIndex(t, route.Rules); vpn > app {
		t.Errorf("the user's via-VPN list (idx=%d) must outrank the torrent rule (idx=%d)", vpn, app)
	}
	if proto := torrentProtocolRuleIndex(route.Rules); vpn > proto {
		t.Errorf("the user's via-VPN list (idx=%d) must outrank the bittorrent rule (idx=%d)", vpn, proto)
	}
}


func globalTorrentConfig(t *testing.T) EngineConfig {
	t.Helper()
	dir := t.TempDir()
	if err := WriteRoutingListRuleSet(dir, "p", ParsedRoutingList{CIDRs: []string{"198.51.100.0/24"}}); err != nil {
		t.Fatal(err)
	}
	return EngineConfig{
		Mode:        ProxyModeTunnel,
		RoutingMode: ModeGlobal,
		DataDir:     dir,
		Proxy:       ProxyConfig{IP: "203.0.113.7", Port: 443, Type: "vless"},
		RoutingLists: []RoutingListSpec{
			{Tag: "rl-p", Path: RoutingListCachePath(dir, "p"), Action: "proxy"},
		},
	}
}

func routingListRuleIndex(rules []SBRouteRule, tag string) int {
	for i, r := range rules {
		if slices.Contains(r.RuleSet, tag) {
			return i
		}
	}
	return -1
}

// Global sends everything to the node, and the node bans BitTorrent.
func TestGlobalSendsTorrentsDirect(t *testing.T) {
	route := buildRoute(globalTorrentConfig(t))

	list := routingListRuleIndex(route.Rules, "rl-p")
	if list < 0 {
		t.Fatalf("routing list rule missing, rules=%+v", route.Rules)
	}
	// A peer that happens to sit in a "via VPN" range would still be banned.
	if proto := torrentProtocolRuleIndex(route.Rules); proto < 0 || proto > list {
		t.Errorf("bittorrent rule (idx=%d) must precede the user's lists (idx=%d)", proto, list)
	}
	if app := torrentClientRuleIndex(t, route.Rules); app < 0 || app < list {
		t.Errorf("torrent client rule (idx=%d) must follow the user's lists (idx=%d)", app, list)
	}
	if !route.FindProcess {
		t.Error("find_process must be on for the torrent client rule")
	}
}

func TestTorrentRulesInEveryTunnelModeOnly(t *testing.T) {
	smart := adaptiveTorrentConfig()
	smart.AdaptiveSmart = false
	whitelist := globalTorrentConfig(t)
	whitelist.RoutingMode = ModeWhitelist
	for name, cfg := range map[string]EngineConfig{"smart": smart, "whitelist": whitelist} {
		route := buildRoute(cfg)
		if torrentProtocolRuleIndex(route.Rules) < 0 || torrentClientRuleIndex(t, route.Rules) < 0 {
			t.Errorf("%s: torrent rules missing", name)
		}
	}

	// A torrent client set to use the local proxy asked for it.
	for _, mode := range []RoutingMode{ModeSmart, ModeGlobal} {
		cfg := globalTorrentConfig(t)
		cfg.Mode = ProxyModeProxy
		cfg.RoutingMode = mode
		route := buildRoute(cfg)
		if i := torrentProtocolRuleIndex(route.Rules); i >= 0 {
			t.Errorf("proxy mode %s: unexpected bittorrent rule at %d", mode, i)
		}
		if i := torrentClientRuleIndex(t, route.Rules); i >= 0 {
			t.Errorf("proxy mode %s: unexpected torrent client rule at %d", mode, i)
		}
	}
}
