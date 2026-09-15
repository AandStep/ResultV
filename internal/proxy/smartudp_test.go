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
	"testing"
)

// blanketQUICReject reports whether the route carries the catch-all udp/443
// reject — the one with no selector of its own.
func blanketQUICReject(rules []SBRouteRule) bool {
	for _, rule := range rules {
		if rule.Action != "reject" {
			continue
		}
		if len(rule.Network) == 1 && rule.Network[0] == "udp" &&
			len(rule.Port) == 1 && rule.Port[0] == 443 &&
			len(rule.Domain) == 0 && len(rule.RuleSet) == 0 &&
			len(rule.ProcessPathRegex) == 0 && len(rule.IPCidr) == 0 {
			return true
		}
	}
	return false
}

// The blanket reject exists because the sniffer cannot name a QUIC connection.
// FakeIP names it before any rule runs, so with the adaptive engine on the
// reject stops being the only option — and HTTP/3 stops being collateral.
func TestAdaptiveSmartDropsTheBlanketQUICReject(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, adaptiveTunnelConfig())
	if blanketQUICReject(cfg.Route.Rules) {
		t.Fatal("the unconditional udp/443 reject survived with the adaptive engine on")
	}
}

// With the switch off the reject is the only thing between Chrome's HTTP/3 and
// an untunnelled request, so it must still be there.
func TestSwitchOffKeepsTheBlanketQUICReject(t *testing.T) {
	cfg := adaptiveTunnelConfig()
	cfg.AdaptiveSmart = false
	built := mustBuildTunnelModeConfig(t, cfg)
	if !blanketQUICReject(built.Route.Rules) {
		t.Fatal("the unconditional udp/443 reject disappeared with the switch off")
	}
}

// The targeted rejects — the ones shadowing a route-to-proxy rule for a
// specific app or list — are about the node's unreliable UDP, not about naming,
// so they stay in both positions of the switch.
func TestTargetedQUICRejectsSurviveAdaptiveSmart(t *testing.T) {
	cfg := adaptiveTunnelConfig()
	cfg.AppForceVPN = []string{"discord.exe"}
	built := mustBuildTunnelModeConfig(t, cfg)
	var found bool
	for _, rule := range built.Route.Rules {
		if rule.Action == "reject" && len(rule.ProcessPathRegex) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("the per-app udp/443 reject was dropped along with the blanket one")
	}
}
