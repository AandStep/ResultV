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

// With the switch on, everything that no list claimed lands on the smart
// outbound instead of going straight out — that is the entire change.
func TestAdaptiveSmartRoutesFinalToSmartOutbound(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, adaptiveTunnelConfig())
	if cfg.Route == nil {
		t.Fatal("no route built")
	}
	if cfg.Route.Final != smartOutboundTag {
		t.Fatalf("route.final = %q, want %q", cfg.Route.Final, smartOutboundTag)
	}

	var found bool
	for _, out := range cfg.Outbounds {
		if out.Tag != smartOutboundTag {
			continue
		}
		found = true
		if out.Type != smartOutboundTag {
			t.Errorf("smart outbound has type %q", out.Type)
		}
		if len(out.Outbounds) != 2 || out.Outbounds[0] != "direct" || out.Outbounds[1] != "proxy" {
			t.Errorf("smart members = %v, want [direct proxy]", out.Outbounds)
		}
	}
	if !found {
		t.Fatal("no smart outbound emitted")
	}
}

// The rollback story, restated for the route: with the switch off Smart mode
// still goes direct by default and no smart outbound exists.
func TestSwitchOffKeepsDirectFinal(t *testing.T) {
	cfg := adaptiveTunnelConfig()
	cfg.AdaptiveSmart = false
	built := mustBuildTunnelModeConfig(t, cfg)
	if built.Route.Final != "direct" {
		t.Fatalf("route.final = %q with the switch off, want direct", built.Route.Final)
	}
	for _, out := range built.Outbounds {
		if out.Tag == smartOutboundTag || out.Type == smartOutboundTag {
			t.Fatal("a smart outbound leaked into a config with the switch off")
		}
	}
}

// A node that carries no "proxy" outbound at all — WireGuard and AmneziaWG are
// endpoints, and buildOutbounds emits only direct+block for them — must not get
// a group pointing at a tag that does not exist: the core refuses to start.
//
// Asserted on the predicate directly: this is the single condition the whole
// feature hangs off, and what a WireGuard config must NOT contain as a result
// is checked against a real built config in
// TestWireGuardNodeGetsNoFakeIPEvenWithTheSwitchOn.
func TestSmartOutboundIsSkippedWhenThereIsNoProxyOutbound(t *testing.T) {
	cfg := adaptiveTunnelConfig()
	for _, pt := range []string{"wireguard", "WireGuard", "amneziawg", "AMNEZIAWG"} {
		cfg.Proxy.Type = pt
		if adaptiveSmartActive(cfg) {
			t.Errorf("%s has no proxy outbound to group, but the smart group was still active", pt)
		}
	}
	cfg.Proxy.Type = "vless"
	if !adaptiveSmartActive(cfg) {
		t.Fatal("a normal node lost its smart group")
	}
}

// The pinned core is the final judge, and it only knows the types in the
// registry its context was built with.
func TestExtendedCoreAcceptsSmartOutbound(t *testing.T) {
	assertCoreAcceptsConfig(t, mustBuildTunnelModeConfig(t, adaptiveTunnelConfig()))
}
