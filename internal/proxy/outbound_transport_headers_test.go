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

// TestTransportHeaders_HttpUpgradeAndHTTP: custom headers are standard practice
// for CDN fronting, and the core accepts them for both transports.
func TestTransportHeaders_HttpUpgradeAndHTTP(t *testing.T) {
	for _, network := range []string{"httpupgrade", "http"} {
		extra := map[string]interface{}{
			"uuid":     "af815621-b245-4149-89da-dd184cfc4b3d",
			"network":  network,
			"security": "tls",
			"sni":      "cdn.example.com",
			"host":     "cdn.example.com",
			"path":     "/up",
			"headers":  map[string]interface{}{"X-Trace": "abc", "Host": "ignored.example"},
		}
		out := outboundFromExtra(t, "VLESS", extra)
		if out.Transport == nil {
			t.Fatalf("%s: no transport", network)
		}
		if got := out.Transport.Headers["X-Trace"]; got != "abc" {
			t.Fatalf("%s: header dropped, got %q", network, got)
		}
		if _, ok := out.Transport.Headers["Host"]; ok {
			t.Fatalf("%s: Host must stay in the dedicated field, not in headers", network)
		}
		assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
	}
}
