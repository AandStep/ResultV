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
	"encoding/base64"
	"testing"
)

// TestSSPlugin_FromSIP002URI: the query used to be cut off before it was ever
// read, so an obfuscated node connected without the obfuscation its server
// requires — i.e. it did not connect.
func TestSSPlugin_FromSIP002URI(t *testing.T) {
	auth := base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:secret"))
	uri := "ss://" + auth + "@203.0.113.7:8388?plugin=obfs-local%3Bobfs%3Dhttp%3Bobfs-host%3Dwww.bing.com#ss-obfs"
	entry, err := ParseProxyURI(uri)
	if err != nil {
		t.Fatalf("ParseProxyURI: %v", err)
	}
	out := buildProxyOutbound(ProxyConfig{Type: entry.Type, IP: entry.IP, Port: entry.Port, Password: entry.Password, Extra: entry.Extra})
	if out.Plugin != "obfs-local" {
		t.Fatalf("plugin = %q, want obfs-local", out.Plugin)
	}
	if out.PluginOptions != "obfs=http;obfs-host=www.bing.com" {
		t.Fatalf("plugin_opts = %q", out.PluginOptions)
	}
}

// TestSSPlugin_CoreAccepts checks the wire shape against the pinned core.
func TestSSPlugin_CoreAccepts(t *testing.T) {
	extra := map[string]interface{}{
		"method":      "aes-256-gcm",
		"plugin":      "v2ray-plugin",
		"plugin_opts": "tls;host=cdn.example.com",
	}
	out := outboundFromExtra(t, "SS", extra)
	if out.Plugin != "v2ray-plugin" || out.PluginOptions != "tls;host=cdn.example.com" {
		t.Fatalf("plugin mapping lost: %+v", out)
	}
	assertCoreAcceptsConfig(t, tunnelConfigFromExtra(t, "SS", extra))
}

// TestSSPlugin_UnknownPluginDropped: an unregistered plugin name fails outbound
// creation, which takes the whole engine down — drop it instead.
func TestSSPlugin_UnknownPluginDropped(t *testing.T) {
	out := outboundFromExtra(t, "SS", map[string]interface{}{
		"method":      "aes-256-gcm",
		"plugin":      "xray-plugin",
		"plugin_opts": "tls",
	})
	if out.Plugin != "" || out.PluginOptions != "" {
		t.Fatalf("unknown plugin forwarded: %q / %q", out.Plugin, out.PluginOptions)
	}
}

// TestSSPlugin_LegacyLinkStillParses: the legacy base64 form used to fail the
// whole link, because the "?plugin=..." tail was fed into base64Decode.
func TestSSPlugin_LegacyLinkStillParses(t *testing.T) {
	blob := base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:secret@203.0.113.7:8388"))
	entry, err := ParseProxyURI("ss://" + blob + "?plugin=obfs-local%3Bobfs%3Dtls#legacy")
	if err != nil {
		t.Fatalf("ParseProxyURI: %v", err)
	}
	if entry.IP != "203.0.113.7" || entry.Port != 8388 {
		t.Fatalf("legacy link mangled: %+v", entry)
	}
}
