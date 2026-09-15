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

package main

import (
	"encoding/json"
	"testing"
	"time"

	"resultproxy-wails/internal/config"
)

func TestPingOptionsFromSettingsDefaults(t *testing.T) {
	opts := pingOptionsFromSettings(config.AppSettings{})
	if opts.Type != config.PingTypeAuto {
		t.Fatalf("type %q", opts.Type)
	}
	if opts.URL != config.DefaultPingTestURL {
		t.Fatalf("url %q", opts.URL)
	}
	if opts.Timeout != 3*time.Second {
		t.Fatalf("timeout %v", opts.Timeout)
	}
}

func TestPingOptionsFromSettingsMapsMethod(t *testing.T) {
	head := pingOptionsFromSettings(config.AppSettings{PingType: config.PingTypeHTTPHead})
	if head.Method != "HEAD" {
		t.Fatalf("method %q", head.Method)
	}
	get := pingOptionsFromSettings(config.AppSettings{PingType: config.PingTypeHTTPGet})
	if get.Method != "GET" {
		t.Fatalf("method %q", get.Method)
	}
}

func TestPingOptionsFromSettingsPassesInvalidURLAsDefault(t *testing.T) {
	// A stored URL that would not work must degrade to the default rather
	// than failing every ping over a setting the user cannot see.
	opts := pingOptionsFromSettings(config.AppSettings{
		PingType:    config.PingTypeHTTPGet,
		PingTestURL: "http://insecure.example.com",
	})
	if opts.URL != config.DefaultPingTestURL {
		t.Fatalf("url %q", opts.URL)
	}
}

func TestNodeByIDCarriesWholeEntry(t *testing.T) {
	entries := []config.ProxyEntry{{
		ID:    "n1",
		IP:    "node.example.com",
		Port:  443,
		Type:  "VLESS",
		URI:   "vless://...",
		Extra: json.RawMessage(`{"uuid":"u"}`),
	}}

	node, ok := nodeByID(entries, "n1")
	if !ok {
		t.Fatal("node not found")
	}
	// buildOutbounds needs all of this; ip+port+type alone cannot describe
	// a VLESS node.
	if node.URI == "" || len(node.Extra) == 0 {
		t.Fatalf("entry arrived stripped: %+v", node)
	}
	if _, ok := nodeByID(entries, "missing"); ok {
		t.Fatal("unknown id must not resolve")
	}
	if _, ok := nodeByID(entries, ""); ok {
		t.Fatal("empty id must not resolve")
	}
}
