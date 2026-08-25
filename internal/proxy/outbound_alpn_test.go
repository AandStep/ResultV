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

func TestXHTTPPreferH2ALPN(t *testing.T) {
	got := xhttpPreferH2ALPN([]string{"h3", "h2", "http/1.1"}, false)
	if len(got) < 3 || got[0] != "h2" || got[1] != "h3" {
		t.Fatalf("got %v", got)
	}
	if xhttpPreferH2ALPN([]string{"h2", "h3"}, false)[0] != "h2" {
		t.Fatal("h2 first should stay")
	}
	empty := xhttpPreferH2ALPN(nil, false)
	if len(empty) < 1 || empty[0] != "h2" {
		t.Fatalf("default: %v", empty)
	}
}

// TestDefaultALPN_NetworkCaseInsensitive: an upper/mixed-case "network" value
// in extra (reachable via embedded ?extra={...}, which now passes values
// through as-is instead of always normalizing them) must not be compared
// case-sensitively against "tcp" in applyTLSAndTransport's default-ALPN
// branch — that used to force ALPN=["http/1.1"] on a plain-TCP node instead
// of leaving it empty for the core's own default, the same class of bug
// applyTransportOnly's own case-insensitive compare already guards against.
func TestDefaultALPN_NetworkCaseInsensitive(t *testing.T) {
	extra := map[string]interface{}{
		"uuid":     "af815621-b245-4149-89da-dd184cfc4b3d",
		"security": "tls",
		"network":  "TCP",
	}
	out := outboundFromExtra(t, "VLESS", extra)
	if out.TLS == nil {
		t.Fatal("no TLS built")
	}
	if len(out.TLS.ALPN) != 0 {
		t.Fatalf("ALPN = %v, want empty (core default) for plain TCP", out.TLS.ALPN)
	}
}
