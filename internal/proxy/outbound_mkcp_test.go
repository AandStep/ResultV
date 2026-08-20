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

func mkcpNodeExtra(extra map[string]interface{}) map[string]interface{} {
	base := map[string]interface{}{
		"uuid":     "af815621-b245-4149-89da-dd184cfc4b3d",
		"network":  "kcp",
		"security": "none",
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}

// TestMKCP_TransportBuilt: an unmapped network silently fell back to raw TCP, so
// the node simply did not work. The core's transport name is "mkcp", not "kcp".
func TestMKCP_TransportBuilt(t *testing.T) {
	extra := mkcpNodeExtra(map[string]interface{}{
		"seed":       "secret-seed",
		"headerType": "srtp",
		"mtu":        1350,
		"tti":        50,
		"congestion": true,
	})
	tr := outboundFromExtra(t, "VLESS", extra).Transport
	if tr == nil {
		t.Fatal("no transport built for kcp")
	}
	if tr.Type != "mkcp" {
		t.Fatalf("type = %q, want mkcp", tr.Type)
	}
	if tr.Seed != "secret-seed" || tr.HeaderType != "srtp" || tr.MTU != 1350 || tr.TTI != 50 || !tr.Congestion {
		t.Fatalf("mkcp knobs mangled: %+v", tr)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}

// TestMKCP_UnknownHeaderTypeDropped: NewPacketHeader knows a closed set.
func TestMKCP_UnknownHeaderTypeDropped(t *testing.T) {
	tr := outboundFromExtra(t, "VLESS", mkcpNodeExtra(map[string]interface{}{"headerType": "quake3"})).Transport
	if tr.HeaderType != "" {
		t.Fatalf("unknown header type forwarded: %q", tr.HeaderType)
	}
}

// TestMKCP_AliasSpelling: links say type=kcp, the core says mkcp.
func TestMKCP_AliasSpelling(t *testing.T) {
	for _, network := range []string{"kcp", "mkcp"} {
		tr := outboundFromExtra(t, "VLESS", mkcpNodeExtra(map[string]interface{}{"network": network})).Transport
		if tr == nil || tr.Type != "mkcp" {
			t.Fatalf("network=%s produced %+v", network, tr)
		}
	}
}

// TestMKCP_NegativeNumbersDropped: every numeric mKCP knob is uint32 in the
// core (option/v2ray_transport.go V2RayKCPOptions); a negative value fails to
// decode ("cannot unmarshal number -5 into ... uint32"), which aborts the
// engine start for every node, not just this one.
func TestMKCP_NegativeNumbersDropped(t *testing.T) {
	extra := mkcpNodeExtra(map[string]interface{}{
		"mtu":              -5,
		"tti":              -1,
		"uplinkCapacity":   -1,
		"downlinkCapacity": -1,
		"readBufferSize":   -1,
		"writeBufferSize":  -1,
	})
	tr := outboundFromExtra(t, "VLESS", extra).Transport
	if tr == nil {
		t.Fatal("no transport built for kcp")
	}
	if tr.MTU != 0 || tr.TTI != 0 || tr.UplinkCapacity != 0 || tr.DownlinkCapacity != 0 ||
		tr.ReadBufferSize != 0 || tr.WriteBufferSize != 0 {
		t.Fatalf("negative mkcp knobs forwarded: %+v", tr)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}
