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

func xhttpNodeExtra(extra map[string]interface{}) map[string]interface{} {
	base := map[string]interface{}{
		"uuid":     "af815621-b245-4149-89da-dd184cfc4b3d",
		"network":  "xhttp",
		"security": "tls",
		"sni":      "cdn.example.com",
		"path":     "/api",
		"mode":     "auto",
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}

func xmuxOf(t *testing.T, out SBOutbound) map[string]interface{} {
	t.Helper()
	if out.Transport == nil || len(out.Transport.Xmux) == 0 {
		t.Fatal("no xmux emitted")
	}
	var m map[string]interface{}
	if err := json.Unmarshal(out.Transport.Xmux, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestXmux_DropsKeysTheCoreDoesNotKnow: an unknown xmux key is not an ignored
// knob — the config decoder runs with DisallowUnknownFields, so it takes the
// whole engine down.
func TestXmux_DropsKeysTheCoreDoesNotKnow(t *testing.T) {
	extra := xhttpNodeExtra(map[string]interface{}{
		"xmux": map[string]interface{}{"maxConcurrency": 8, "xudpConcurrency": 4, "someFutureKnob": "x"},
	})
	m := xmuxOf(t, outboundFromExtra(t, "VLESS", extra))
	for _, k := range []string{"xudpConcurrency", "someFutureKnob"} {
		if _, ok := m[k]; ok {
			t.Fatalf("unknown xmux key %q forwarded to the core: %v", k, m)
		}
	}
	if got := m["max_concurrency"]; got != float64(8) {
		t.Fatalf("user max_concurrency = %v, want 8", got)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}

// TestXmux_MaxConnectionsWinsOverDefaultConcurrency: the core refuses a config
// carrying both knobs, and refusing means no engine at all.
func TestXmux_MaxConnectionsWinsOverDefaultConcurrency(t *testing.T) {
	extra := xhttpNodeExtra(map[string]interface{}{
		"xmux": map[string]interface{}{"maxConnections": 4},
	})
	m := xmuxOf(t, outboundFromExtra(t, "VLESS", extra))
	if _, ok := m["max_concurrency"]; ok {
		t.Fatalf("max_concurrency kept next to max_connections: %v", m)
	}
	if got := m["max_connections"]; got != float64(4) {
		t.Fatalf("max_connections = %v, want 4", got)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}
