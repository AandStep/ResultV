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

import "testing"

// TestMultiplex_ExplicitProfile: only a sing-box-shaped multiplex object is
// honoured, and only with a protocol the core speaks.
func TestMultiplex_ExplicitProfile(t *testing.T) {
	extra := map[string]interface{}{
		"uuid":     "af815621-b245-4149-89da-dd184cfc4b3d",
		"network":  "tcp",
		"security": "none",
		"multiplex": map[string]interface{}{
			"enabled":     true,
			"protocol":    "smux",
			"max_streams": 8,
			"padding":     true,
		},
	}
	out := outboundFromExtra(t, "VLESS", extra)
	if out.Multiplex == nil {
		t.Fatal("multiplex dropped")
	}
	if !out.Multiplex.Enabled || out.Multiplex.Protocol != "smux" || out.Multiplex.MaxStreams != 8 || !out.Multiplex.Padding {
		t.Fatalf("multiplex mangled: %+v", out.Multiplex)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}

// TestMultiplex_XrayMuxIgnored: Xray's mux is a different wire protocol from
// smux/yamux/h2mux. Mapping it would turn a working node into a broken one, so
// it stays ignored on purpose.
func TestMultiplex_XrayMuxIgnored(t *testing.T) {
	out := outboundFromExtra(t, "VLESS", map[string]interface{}{
		"uuid":     "af815621-b245-4149-89da-dd184cfc4b3d",
		"network":  "tcp",
		"security": "none",
		"mux":      map[string]interface{}{"enabled": true, "concurrency": 8},
	})
	if out.Multiplex != nil {
		t.Fatalf("Xray mux mapped onto sing-box multiplex: %+v", out.Multiplex)
	}
}

// TestMultiplex_UnknownProtocolDropped: an unknown protocol fails outbound
// creation, which takes the engine start with it.
func TestMultiplex_UnknownProtocolDropped(t *testing.T) {
	out := outboundFromExtra(t, "VLESS", map[string]interface{}{
		"uuid":      "af815621-b245-4149-89da-dd184cfc4b3d",
		"network":   "tcp",
		"security":  "none",
		"multiplex": map[string]interface{}{"enabled": true, "protocol": "xraymux"},
	})
	if out.Multiplex != nil {
		t.Fatalf("unknown multiplex protocol forwarded: %+v", out.Multiplex)
	}
}
