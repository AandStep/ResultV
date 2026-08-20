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
	"reflect"
	"testing"
)

// TestHysteria2Hop_FromURI: hy2 links spell ranges with a dash, sing-quic parses
// only colons ("bad port range" otherwise, which aborts outbound creation).
func TestHysteria2Hop_FromURI(t *testing.T) {
	entry, err := ParseProxyURI("hysteria2://secret@203.0.113.7:443?sni=example.com&mport=10000-20000%2C30000&hop-interval=30s#hop")
	if err != nil {
		t.Fatalf("ParseProxyURI: %v", err)
	}
	out := buildProxyOutbound(ProxyConfig{Type: entry.Type, IP: entry.IP, Port: entry.Port, Extra: entry.Extra})
	want := []string{"10000:20000", "30000:30000"}
	if !reflect.DeepEqual(out.ServerPorts, want) {
		t.Fatalf("server_ports = %v, want %v", out.ServerPorts, want)
	}
	if out.HopInterval != "30s" {
		t.Fatalf("hop_interval = %q", out.HopInterval)
	}
}

// TestHysteria2Hop_CoreAccepts checks the wire shape against the pinned core.
func TestHysteria2Hop_CoreAccepts(t *testing.T) {
	extra := map[string]interface{}{"sni": "example.com", "mport": "10000-20000", "hop_interval": "30s"}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "HYSTERIA2", extra))
}

// TestHysteria2Hop_JunkDropped: a range the core cannot parse must not reach it.
func TestHysteria2Hop_JunkDropped(t *testing.T) {
	out := outboundFromExtra(t, "HYSTERIA2", map[string]interface{}{
		"sni":   "example.com",
		"mport": "abc,70000-80000,5000-4000,8000",
	})
	want := []string{"8000:8000"}
	if !reflect.DeepEqual(out.ServerPorts, want) {
		t.Fatalf("server_ports = %v, want %v", out.ServerPorts, want)
	}
}

// TestHysteria2Hop_AbsentStaysAbsent keeps the old wire shape for plain nodes.
func TestHysteria2Hop_AbsentStaysAbsent(t *testing.T) {
	out := outboundFromExtra(t, "HYSTERIA2", map[string]interface{}{"sni": "example.com"})
	if out.ServerPorts != nil || out.HopInterval != "" {
		t.Fatalf("hop fields emitted without being asked: %v / %q", out.ServerPorts, out.HopInterval)
	}
}

// TestHysteria2Hop_BareSecondsCanonicalized: hy2 links historically spell the
// interval as a bare number of seconds ("hop-interval=30"). The core's
// badoption.Duration (option/hysteria2.go) requires a Go duration string with
// a unit and fails to decode ("missing unit in duration") without one — this
// is the regression that made a previously-working node stop connecting.
func TestHysteria2Hop_BareSecondsCanonicalized(t *testing.T) {
	extra := map[string]interface{}{"sni": "example.com", "hop_interval": "30"}
	out := outboundFromExtra(t, "HYSTERIA2", extra)
	if out.HopInterval != "30s" {
		t.Fatalf("hop_interval = %q, want 30s", out.HopInterval)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "HYSTERIA2", extra))
}

// TestHysteria2Hop_UnitStringPassesThrough keeps an already-valid duration
// string untouched.
func TestHysteria2Hop_UnitStringPassesThrough(t *testing.T) {
	out := outboundFromExtra(t, "HYSTERIA2", map[string]interface{}{"sni": "example.com", "hop_interval": "30s"})
	if out.HopInterval != "30s" {
		t.Fatalf("hop_interval = %q, want 30s", out.HopInterval)
	}
}

// TestHysteria2Hop_GarbageDropped: a value that is neither a valid duration
// nor a bare positive number must not reach the core — badoption.Duration
// would fail to decode it and abort the engine start.
func TestHysteria2Hop_GarbageDropped(t *testing.T) {
	extra := map[string]interface{}{"sni": "example.com", "hop_interval": "abc"}
	out := outboundFromExtra(t, "HYSTERIA2", extra)
	if out.HopInterval != "" {
		t.Fatalf("hop_interval = %q, want empty", out.HopInterval)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "HYSTERIA2", extra))
}
