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

// The exclusion that keeps a WireGuard node's own UDP out of the tunnel it
// carries. Measured back to back on one node (tunrepro bench, 15.09.2026):
// without it three 25 MB downloads failed outright and tcp_established never
// left zero; with it the same downloads ran at 170, 101 and 142 Mbit/s.
func TestWGRouteExcludeCoversEveryPinnedBackend(t *testing.T) {
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

// Other protocols kept this exclusion all along, and the WireGuard fix must not
// disturb them.
func TestRouteExcludeStillCoversOtherProtocols(t *testing.T) {
	node := awgNodeWithPin()
	node.Type = "VLESS"
	node.Extra = json.RawMessage(`{}`)
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{Proxy: node, Mode: ProxyModeTunnel})
	if got := cfg.Inbounds[0].RouteExcludeAddress; len(got) == 0 {
		t.Errorf("VLESS всегда исключает адрес сервера, получено %v", got)
	}
}

// Without a pin there is nothing to exclude: a node addressed by a name that
// never resolved must not produce a bogus CIDR.
func TestRouteExcludeEmptyWithoutPin(t *testing.T) {
	node := awgNodeWithPin()
	node.ResolvedIP = ""
	node.ResolvedIPs = nil
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{Proxy: node, Mode: ProxyModeTunnel})
	if got := cfg.Inbounds[0].RouteExcludeAddress; len(got) != 0 {
		t.Errorf("без пина исключений быть не может, получено %v", got)
	}
}
