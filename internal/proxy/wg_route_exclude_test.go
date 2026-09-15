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
	"encoding/json"
	"slices"
	"testing"
)

func awgNodeWithPin() ProxyConfig {
	return ProxyConfig{
		Type:        "AMNEZIAWG",
		IP:          "node.example.com",
		Port:        3306,
		ResolvedIP:  "198.51.100.7",
		ResolvedIPs: []string{"198.51.100.7", "198.51.100.8"},
		Extra: json.RawMessage(`{
			"private_key": "priv", "public_key": "pub",
			"address": ["10.8.2.143/24"], "allowed_ips": ["0.0.0.0/0"]
		}`),
	}
}

// Default behaviour: a WireGuard node's own address stays inside the tunnel's
// routes, and its UDP loops through the TUN inbound.
func TestWGRouteExcludeOffByDefault(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: awgNodeWithPin(),
		Mode:  ProxyModeTunnel,
	})
	if got := cfg.Inbounds[0].RouteExcludeAddress; len(got) != 0 {
		t.Errorf("без переменной исключений быть не должно, получено %v", got)
	}
}

// With the switch on, every pinned backend is excluded — not just the first.
// A CDN node answers with several addresses and sing-box may fail over among
// them mid-session, so excluding one would leave the rest looping.
func TestWGRouteExcludeCoversEveryPinnedBackend(t *testing.T) {
	t.Setenv("RESULTV_WG_ROUTE_EXCLUDE", "1")
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: awgNodeWithPin(),
		Mode:  ProxyModeTunnel,
	})
	got := cfg.Inbounds[0].RouteExcludeAddress
	for _, want := range []string{"198.51.100.7/32", "198.51.100.8/32"} {
		if !slices.Contains(got, want) {
			t.Errorf("нет исключения %s в %v", want, got)
		}
	}
}

// Non-WireGuard nodes are unaffected by the switch: they already get their
// exclusions, and the switch must not change that path.
func TestWGRouteExcludeLeavesOtherProtocolsAlone(t *testing.T) {
	node := awgNodeWithPin()
	node.Type = "VLESS"
	node.Extra = json.RawMessage(`{}`)
	for _, env := range []string{"", "1"} {
		if env != "" {
			t.Setenv("RESULTV_WG_ROUTE_EXCLUDE", env)
		}
		cfg := mustBuildTunnelModeConfig(t, EngineConfig{Proxy: node, Mode: ProxyModeTunnel})
		if got := cfg.Inbounds[0].RouteExcludeAddress; len(got) == 0 {
			t.Errorf("RESULTV_WG_ROUTE_EXCLUDE=%q: VLESS всегда исключает адрес сервера, получено %v", env, got)
		}
	}
}
