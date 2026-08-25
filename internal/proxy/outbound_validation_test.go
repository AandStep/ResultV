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

// TestXHTTPMode_CanonicalizedAndGated: mode drives both the emitted "mode"
// field and the uplinkDataPlacement gate. A mixed-case spelling like
// "Packet-Up" must canonicalize to "packet-up" everywhere it is compared,
// otherwise the gate silently closes even though the mode itself is valid.
func TestXHTTPMode_CanonicalizedAndGated(t *testing.T) {
	extra := xhttpNodeExtra(map[string]interface{}{
		"mode":                "Packet-Up",
		"uplinkDataPlacement": "cookie",
	})
	out := outboundFromExtra(t, "VLESS", extra)
	if out.Transport == nil {
		t.Fatal("no transport built")
	}
	if out.Transport.Mode != "packet-up" {
		t.Fatalf("mode = %q, want packet-up", out.Transport.Mode)
	}
	if out.Transport.UplinkDataPlacement != "cookie" {
		t.Fatalf("uplink_data_placement = %q, want cookie (mode gate missed the canonicalized mode)", out.Transport.UplinkDataPlacement)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}

// TestXHTTPMode_UnknownFallsBackToAuto: V2RayXHTTPOptions.UnmarshalJSON fails
// decoding ("unsupported mode: turbo") on anything outside
// auto|packet-up|stream-up|stream-one, aborting the engine start.
func TestXHTTPMode_UnknownFallsBackToAuto(t *testing.T) {
	extra := xhttpNodeExtra(map[string]interface{}{"mode": "turbo"})
	out := outboundFromExtra(t, "VLESS", extra)
	if out.Transport == nil || out.Transport.Mode != "auto" {
		t.Fatalf("mode = %+v, want auto", out.Transport)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}

// TestXHTTPUplinkMethod_GetOnlyInPacketUpMode: checkV2RayXHTTPBaseOptions
// rejects "GET" outside packet-up mode ("uplink_http_method can be GET only
// in packet-up mode"), aborting the engine start.
func TestXHTTPUplinkMethod_GetOnlyInPacketUpMode(t *testing.T) {
	extraAuto := xhttpNodeExtra(map[string]interface{}{
		"mode":               "auto",
		"uplink_http_method": "GET",
	})
	outAuto := outboundFromExtra(t, "VLESS", extraAuto)
	if outAuto.Transport == nil || outAuto.Transport.UplinkHTTPMethod != "" {
		t.Fatalf("GET forwarded outside packet-up mode: %+v", outAuto.Transport)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extraAuto))

	extraPacketUp := xhttpNodeExtra(map[string]interface{}{
		"mode":               "packet-up",
		"uplink_http_method": "GET",
	})
	outPacketUp := outboundFromExtra(t, "VLESS", extraPacketUp)
	if outPacketUp.Transport == nil || outPacketUp.Transport.UplinkHTTPMethod != "GET" {
		t.Fatalf("GET dropped in packet-up mode: %+v", outPacketUp.Transport)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extraPacketUp))
}

// TestXHTTPUplinkMethod_OtherValuesDropped: only POST and (packet-up-only) GET
// are values this project has ever seen used; anything else is dropped so the
// core's own POST default applies instead of forwarding an unverified value.
func TestXHTTPUplinkMethod_OtherValuesDropped(t *testing.T) {
	extra := xhttpNodeExtra(map[string]interface{}{
		"mode":               "packet-up",
		"uplink_http_method": "PUT",
	})
	out := outboundFromExtra(t, "VLESS", extra)
	if out.Transport == nil || out.Transport.UplinkHTTPMethod != "" {
		t.Fatalf("PUT forwarded: %+v", out.Transport)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}

// TestHTTPTransport_HostPathFallback: the "http"/"h2" branch used to read
// "http-host"/"http-path"/"http-method", keys no parser in this project ever
// writes into extra. The real fields links and JSON subscriptions carry are
// "host"/"path"/"method" — after Task 4 stripped "host" out of the generic
// headers map, an h2 node lost its host entirely.
func TestHTTPTransport_HostPathFallback(t *testing.T) {
	extra := map[string]interface{}{
		"uuid":     "af815621-b245-4149-89da-dd184cfc4b3d",
		"network":  "h2",
		"security": "tls",
		"sni":      "cdn.example.com",
		"host":     "cdn.example.com",
		"path":     "/api",
		"method":   "PUT",
	}
	out := outboundFromExtra(t, "VLESS", extra)
	if out.Transport == nil || out.Transport.Type != "http" {
		t.Fatalf("transport = %+v", out.Transport)
	}
	if out.Transport.Host != "cdn.example.com" {
		t.Fatalf("host = %q, want cdn.example.com", out.Transport.Host)
	}
	if out.Transport.Path != "/api" {
		t.Fatalf("path = %q, want /api", out.Transport.Path)
	}
	if out.Transport.Method != "PUT" {
		t.Fatalf("method = %q, want PUT", out.Transport.Method)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}

// TestHTTPTransport_PathDefaultsToSlash keeps the old default when neither
// "http-path" nor "path" is present.
func TestHTTPTransport_PathDefaultsToSlash(t *testing.T) {
	out := outboundFromExtra(t, "VLESS", map[string]interface{}{
		"uuid":     "af815621-b245-4149-89da-dd184cfc4b3d",
		"network":  "h2",
		"security": "tls",
		"sni":      "cdn.example.com",
	})
	if out.Transport == nil || out.Transport.Path != "/" {
		t.Fatalf("path = %+v, want /", out.Transport)
	}
}
