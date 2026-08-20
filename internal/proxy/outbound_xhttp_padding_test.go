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
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

// xhttpTransportFromExtra builds the VLESS outbound for an xhttp node whose
// extra map is `extra`, and returns its transport section.
func xhttpTransportFromExtra(t *testing.T, extra map[string]interface{}) *SBOutboundTransport {
	t.Helper()
	extra["network"] = "xhttp"
	raw, err := json.Marshal(extra)
	if err != nil {
		t.Fatal(err)
	}
	out := buildProxyOutbound(ProxyConfig{
		Type: "VLESS", IP: "cdn.example.com", Port: 443,
		Extra: raw,
	})
	if out.Transport == nil {
		t.Fatal("no transport built")
	}
	return out.Transport
}

// TestXHTTPPaddingObfs_MappedFromExtra is the core of the feature: the padding
// obfuscation profile reaches the engine JSON instead of being dropped on the
// way from the URI's extra map.
func TestXHTTPPaddingObfs_MappedFromExtra(t *testing.T) {
	cases := []struct {
		name  string
		extra map[string]interface{}
	}{
		{"camelCase (Xray spelling)", map[string]interface{}{
			"xPaddingObfsMode":  true,
			"xPaddingKey":       "sess",
			"xPaddingHeader":    "X-Trace",
			"xPaddingPlacement": "cookie",
			"xPaddingMethod":    "tokenish",
		}},
		{"snake_case (sing-box spelling)", map[string]interface{}{
			"x_padding_obfs_mode": true,
			"x_padding_key":       "sess",
			"x_padding_header":    "X-Trace",
			"x_padding_placement": "cookie",
			"x_padding_method":    "tokenish",
		}},
		{"stringified bool", map[string]interface{}{
			"xPaddingObfsMode":  "true",
			"xPaddingKey":       "sess",
			"xPaddingHeader":    "X-Trace",
			"xPaddingPlacement": "cookie",
			"xPaddingMethod":    "tokenish",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := xhttpTransportFromExtra(t, tc.extra)
			if tr.XPaddingObfsMode == nil || !*tr.XPaddingObfsMode {
				t.Fatalf("x_padding_obfs_mode not enabled: %v", tr.XPaddingObfsMode)
			}
			if tr.XPaddingKey != "sess" || tr.XPaddingHeader != "X-Trace" {
				t.Fatalf("key/header = %q/%q", tr.XPaddingKey, tr.XPaddingHeader)
			}
			if tr.XPaddingPlacement != "cookie" || tr.XPaddingMethod != "tokenish" {
				t.Fatalf("placement/method = %q/%q", tr.XPaddingPlacement, tr.XPaddingMethod)
			}
		})
	}
}

// TestXHTTPPaddingObfs_AbsentStaysAbsent guards the compatibility promise: an
// ordinary xhttp node must serialize byte-for-byte as before, so the core keeps
// applying its own defaults (Referer / x_padding / repeat-x).
func TestXHTTPPaddingObfs_AbsentStaysAbsent(t *testing.T) {
	tr := xhttpTransportFromExtra(t, map[string]interface{}{"path": "/xhttp"})
	raw, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{
		"x_padding_obfs_mode", "x_padding_key", "x_padding_header",
		"x_padding_placement", "x_padding_method",
	} {
		if _, ok := m[k]; ok {
			t.Fatalf("%s emitted for a node that never asked for padding obfs: %s", k, raw)
		}
	}
}

// TestXHTTPPaddingObfs_UnknownEnumsDropped: the core validates placement/method
// while parsing the config, and a bad value aborts the whole instance — every
// node with it. Unrecognized spellings must be dropped, not forwarded.
func TestXHTTPPaddingObfs_UnknownEnumsDropped(t *testing.T) {
	tr := xhttpTransportFromExtra(t, map[string]interface{}{
		"xPaddingObfsMode":  true,
		"xPaddingPlacement": "body",     // valid for uplink data, never for padding
		"xPaddingMethod":    "repeat-y", // typo
	})
	if tr.XPaddingPlacement != "" || tr.XPaddingMethod != "" {
		t.Fatalf("unsupported enums leaked: placement=%q method=%q", tr.XPaddingPlacement, tr.XPaddingMethod)
	}
}

// TestXHTTPPaddingObfs_EnumAliases accepts the spellings a config can realistically
// carry (case and separator variants) and canonicalizes them for the core.
func TestXHTTPPaddingObfs_EnumAliases(t *testing.T) {
	cases := map[string]string{
		"queryInHeader": "queryInHeader",
		"queryinheader": "queryInHeader",
		"QueryInHeader": "queryInHeader",
		"Header":        "header",
		"QUERY":         "query",
	}
	for in, want := range cases {
		tr := xhttpTransportFromExtra(t, map[string]interface{}{"xPaddingPlacement": in})
		if tr.XPaddingPlacement != want {
			t.Fatalf("placement %q -> %q, want %q", in, tr.XPaddingPlacement, want)
		}
	}
	for _, in := range []string{"repeat-x", "repeat_x", "RepeatX", "Tokenish"} {
		tr := xhttpTransportFromExtra(t, map[string]interface{}{"xPaddingMethod": in})
		if tr.XPaddingMethod == "" {
			t.Fatalf("method %q dropped", in)
		}
	}
}

// TestXHTTPPaddingObfs_EndToEndFromURI walks the whole chain the user's report
// describes: vless:// URI with an embedded extra JSON -> parser -> outbound ->
// engine JSON, and then hands that JSON to the REAL pinned core to parse. That
// last step is what proves the fields are understood: sing-box rejects unknown
// fields outright, so a stale core would fail here instead of silently ignoring.
func TestXHTTPPaddingObfs_EndToEndFromURI(t *testing.T) {
	extra := `{"xPaddingBytes":"200-800","xPaddingObfsMode":true,"xPaddingKey":"sid","xPaddingHeader":"X-Trace","xPaddingPlacement":"queryInHeader","xPaddingMethod":"tokenish"}`
	uri := "vless://af815621-b245-4149-89da-dd184cfc4b3d@cdn.example.com:443?type=xhttp&security=tls&sni=cdn.example.com&mode=auto&path=%2Fapi&extra=" + url.QueryEscape(extra) + "#obfs-node"

	entry, err := ParseProxyURI(uri)
	if err != nil {
		t.Fatalf("ParseProxyURI: %v", err)
	}
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{Type: entry.Type, IP: entry.IP, Port: entry.Port, Extra: entry.Extra, ResolvedIPs: []string{"203.0.113.7"}},
		Mode:  ProxyModeTunnel,
	})

	var transport *SBOutboundTransport
	for _, out := range cfg.Outbounds {
		if out.Tag == "proxy" {
			transport = out.Transport
		}
	}
	if transport == nil {
		t.Fatal("proxy outbound has no transport")
	}
	if transport.XPaddingObfsMode == nil || !*transport.XPaddingObfsMode {
		t.Fatal("x_padding_obfs_mode lost between URI and engine config")
	}
	if transport.XPaddingKey != "sid" || transport.XPaddingHeader != "X-Trace" ||
		transport.XPaddingPlacement != "queryInHeader" || transport.XPaddingMethod != "tokenish" {
		t.Fatalf("padding profile mangled: %+v", transport)
	}
	if transport.XPaddingBytes != "200-800" {
		t.Fatalf("x_padding_bytes = %q", transport.XPaddingBytes)
	}

	j, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var opts option.Options
	if err := singjson.UnmarshalContext(include.Context(context.Background()), j, &opts); err != nil {
		t.Fatalf("pinned core rejected the config: %v", err)
	}
}
