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
	"testing"
)

func wsNodeExtra(extra map[string]interface{}) map[string]interface{} {
	base := map[string]interface{}{
		"uuid":     "af815621-b245-4149-89da-dd184cfc4b3d",
		"network":  "ws",
		"security": "tls",
		"sni":      "cdn.example.com",
		"path":     "/ws",
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}

// TestWSTransport_HostTravelsAsHeader is the P0 guard: sing-box's websocket
// transport has no "host" field at all, so emitting one made the whole config
// unparsable — every node in the session died, not just this one.
func TestWSTransport_HostTravelsAsHeader(t *testing.T) {
	out := outboundFromExtra(t, "VLESS", wsNodeExtra(map[string]interface{}{"host": "cdn.example.com"}))
	if out.Transport == nil {
		t.Fatal("no transport built")
	}
	if out.Transport.Host != "" {
		t.Fatalf("ws transport still carries a top-level host: %q", out.Transport.Host)
	}
	if got := out.Transport.Headers["Host"]; got != "cdn.example.com" {
		t.Fatalf("headers[Host] = %q, want cdn.example.com", got)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", wsNodeExtra(map[string]interface{}{"host": "cdn.example.com"})))
}

// TestWSTransport_MergesUserHeaders: a node's own headers survive next to Host.
func TestWSTransport_MergesUserHeaders(t *testing.T) {
	out := outboundFromExtra(t, "VLESS", wsNodeExtra(map[string]interface{}{
		"host":    "cdn.example.com",
		"headers": map[string]interface{}{"X-Trace": "abc", "host": "ignored.example"},
	}))
	if got := out.Transport.Headers["X-Trace"]; got != "abc" {
		t.Fatalf("user header lost: %q", got)
	}
	if got := out.Transport.Headers["Host"]; got != "cdn.example.com" {
		t.Fatalf("headers[Host] = %q, want the node's host, not the headers copy", got)
	}
}

// TestWSTransport_NetworkCaseInsensitive: applyTransportOnly's switch used to
// compare the network value against lowercase literals only, while the mKCP
// gate in parseVMessURI is already case-insensitive ("net":"KCP" stores
// seed/headerType into extra regardless of case) — so an upper/mixed-case
// network here built no transport at all even though the knobs for it were
// already present.
func TestWSTransport_NetworkCaseInsensitive(t *testing.T) {
	out := outboundFromExtra(t, "VLESS", wsNodeExtra(map[string]interface{}{"network": "WS", "host": "cdn.example.com"}))
	if out.Transport == nil || out.Transport.Type != "ws" {
		t.Fatalf("network=WS produced %+v, want a ws transport", out.Transport)
	}
}

// TestWSTransport_NoHostNoHeaders keeps the old wire shape for plain ws nodes.
func TestWSTransport_NoHostNoHeaders(t *testing.T) {
	out := outboundFromExtra(t, "VLESS", wsNodeExtra(nil))
	raw, err := json.Marshal(out.Transport)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["headers"]; ok {
		t.Fatalf("headers emitted for a node without any: %s", raw)
	}
	if _, ok := m["host"]; ok {
		t.Fatalf("host emitted for ws: %s", raw)
	}
}
