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
	"net/url"
	"testing"
)

const testVLESSEncryption = "mlkem768x25519plus.native.0rtt.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// TestVLESSEncryption_ReachesTheCore: a node with VLESS Encryption cannot connect
// at all unless the handshake string reaches the engine.
func TestVLESSEncryption_ReachesTheCore(t *testing.T) {
	extra := map[string]interface{}{
		"uuid":       "af815621-b245-4149-89da-dd184cfc4b3d",
		"network":    "tcp",
		"security":   "none",
		"encryption": testVLESSEncryption,
	}
	out := outboundFromExtra(t, "VLESS", extra)
	if out.Encryption != testVLESSEncryption {
		t.Fatalf("encryption = %q, want %q", out.Encryption, testVLESSEncryption)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "VLESS", extra))
}

// TestVLESSEncryption_FromURI: the parameter is on every vless:// link, so the
// parser has to carry it — today its whitelist drops it before extra.
func TestVLESSEncryption_FromURI(t *testing.T) {
	uri := "vless://af815621-b245-4149-89da-dd184cfc4b3d@203.0.113.7:443?type=tcp&security=none&encryption=" + url.QueryEscape(testVLESSEncryption) + "#enc"
	entry, err := ParseProxyURI(uri)
	if err != nil {
		t.Fatalf("ParseProxyURI: %v", err)
	}
	out := buildProxyOutbound(ProxyConfig{Type: entry.Type, IP: entry.IP, Port: entry.Port, Extra: entry.Extra})
	if out.Encryption != testVLESSEncryption {
		t.Fatalf("encryption lost between URI and outbound: %q", out.Encryption)
	}
}

// TestVLESSEncryption_FromEmbeddedExtra: parseVLESSURI keeps the query-param
// assignment conditional so an embedded ?extra={"encryption":...} JSON blob
// survives when the link has no query-level encryption= of its own — a naive
// unconditional assignment would silently overwrite it with "".
func TestVLESSEncryption_FromEmbeddedExtra(t *testing.T) {
	q := url.Values{}
	q.Set("type", "tcp")
	q.Set("security", "none")
	q.Set("extra", `{"encryption":"`+testVLESSEncryption+`"}`)

	uri := "vless://af815621-b245-4149-89da-dd184cfc4b3d@203.0.113.7:443?" + q.Encode() + "#enc"
	entry, err := ParseProxyURI(uri)
	if err != nil {
		t.Fatalf("ParseProxyURI: %v", err)
	}
	out := buildProxyOutbound(ProxyConfig{Type: entry.Type, IP: entry.IP, Port: entry.Port, Extra: entry.Extra})
	if out.Encryption != testVLESSEncryption {
		t.Fatalf("embedded-extra encryption lost: %q", out.Encryption)
	}
}

// TestVLESSEncryption_JunkDropped: "none" is on nearly every link, and the core
// validates the string when it builds the outbound — a bad one kills the start.
func TestVLESSEncryption_JunkDropped(t *testing.T) {
	for _, v := range []string{"none", "", "aes-128-gcm", "auto", "mlkem768"} {
		out := outboundFromExtra(t, "VLESS", map[string]interface{}{
			"uuid":       "af815621-b245-4149-89da-dd184cfc4b3d",
			"network":    "tcp",
			"security":   "none",
			"encryption": v,
		})
		if out.Encryption != "" {
			t.Fatalf("encryption=%q forwarded as %q", v, out.Encryption)
		}
	}
}
