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
	"strings"
	"testing"
)

// TestDNSOptimisticCacheEmitted pins the optimistic cache on, and pins the
// core's acceptance of the shape: it is an object here rather than a bare
// `true`, because the default window (3d) is longer than this client wants to
// serve a stale answer after a network change.
func TestDNSOptimisticCacheEmitted(t *testing.T) {
	cfg := tunnelConfigFromExtra(t, "TROJAN", map[string]interface{}{"sni": "example.com"})
	j, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	js := string(j)
	if !strings.Contains(js, `"optimistic":{"enabled":true,"timeout":"6h"}`) {
		t.Fatalf("optimistic DNS cache missing from config: %s", js)
	}
	if strings.Contains(js, `"disable_cache"`) || strings.Contains(js, `"disable_expire"`) {
		t.Fatalf("optimistic conflicts with disable_cache/disable_expire, both must stay absent: %s", js)
	}
	assertCoreAcceptsConfig(t, cfg)
}

// Proxy mode builds its DNS block on a separate path. It got the option too,
// via the shared constructor — a second exit that quietly kept the default
// would make the two modes resolve differently for no stated reason.
func TestDNSOptimisticCacheEmittedInProxyMode(t *testing.T) {
	raw, err := json.Marshal(map[string]interface{}{"sni": "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{Type: "TROJAN", IP: "203.0.113.7", Port: 443, Password: "p", Extra: raw},
		Mode:  ProxyModeProxy,
	})
	j, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if js := string(j); !strings.Contains(js, `"optimistic":{"enabled":true,"timeout":"6h"}`) {
		t.Fatalf("optimistic DNS cache missing from proxy-mode config: %s", js)
	}
	assertCoreAcceptsConfig(t, cfg)
}
