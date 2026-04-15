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
	"testing"

	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

func TestWireGuardEndpointConfigParses(t *testing.T) {
	extra := map[string]interface{}{
		"address":     []string{"10.0.0.2/32"},
		"private_key": "priv",
		"public_key":  "pub",
		"allowed_ips": []string{"0.0.0.0/0"},
	}
	raw, err := json.Marshal(extra)
	if err != nil {
		t.Fatal(err)
	}
	cfg := BuildTunnelModeConfig(EngineConfig{
		Proxy: ProxyConfig{
			IP:    "127.0.0.1",
			Port:  51820,
			Type:  "WIREGUARD",
			Extra: raw,
		},
		Mode: ProxyModeTunnel,
	})
	j, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := include.Context(context.Background())
	var opt option.Options
	if err := singjson.UnmarshalContext(ctx, j, &opt); err != nil {
		t.Fatalf("parsing options: %v", err)
	}
	if cfg.Route == nil || cfg.Route.Final != "proxy" {
		t.Fatalf("unexpected route final: %+v", cfg.Route)
	}
	if err := validateRouteFinalTarget(cfg); err != nil {
		t.Fatalf("invalid route final target: %v", err)
	}
}

func TestAmneziaWGEndpointConfigIncludesAmneziaSection(t *testing.T) {
	extra := map[string]interface{}{
		"address":     []string{"10.0.0.2/32"},
		"private_key": "priv",
		"public_key":  "pub",
		"allowed_ips": []string{"0.0.0.0/0"},
		"amnezia": map[string]interface{}{
			"jc":    7,
			"jmin":  10,
			"jmax":  20,
			"s1":    1,
			"h1":    11,
			"i1":    "abc",
			"j1":    "def",
			"itime": 42,
		},
	}
	raw, err := json.Marshal(extra)
	if err != nil {
		t.Fatal(err)
	}
	cfg := BuildTunnelModeConfig(EngineConfig{
		Proxy: ProxyConfig{
			IP:    "127.0.0.1",
			Port:  51820,
			Type:  "AMNEZIAWG",
			Extra: raw,
		},
		Mode: ProxyModeTunnel,
	})
	if len(cfg.Endpoints) != 1 {
		t.Fatalf("expected single endpoint, got %d", len(cfg.Endpoints))
	}
	ep := cfg.Endpoints[0]
	if ep.Amnezia == nil {
		t.Fatalf("expected amnezia section in endpoint, got nil")
	}
	if ep.Amnezia.JC != 7 || ep.Amnezia.JMin != 10 || ep.Amnezia.JMax != 20 {
		t.Fatalf("unexpected amnezia jitter values: %+v", ep.Amnezia)
	}
	if ep.Amnezia.ITime != 42 {
		t.Fatalf("unexpected amnezia itime: %+v", ep.Amnezia)
	}
}

func TestHysteria2OutboundConfigParses(t *testing.T) {
	extra := map[string]interface{}{
		"password":      "p",
		"sni":           "example.com",
		"alpn":          "h3",
		"up_mbps":       10,
		"down_mbps":     20,
		"obfs_type":     "salamander",
		"obfs_password": "x",
	}
	raw, err := json.Marshal(extra)
	if err != nil {
		t.Fatal(err)
	}
	cfg := BuildProxyModeConfig(EngineConfig{
		Proxy: ProxyConfig{
			IP:    "example.com",
			Port:  443,
			Type:  "HYSTERIA2",
			Extra: raw,
		},
		Mode:       ProxyModeProxy,
		ListenAddr: "127.0.0.1:14081",
	})
	j, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := include.Context(context.Background())
	var opt option.Options
	if err := singjson.UnmarshalContext(ctx, j, &opt); err != nil {
		t.Fatalf("parsing options: %v", err)
	}
}

func TestSSTunnelConfigParsesWithDNS(t *testing.T) {
	extra := map[string]interface{}{
		"method": "chacha20-ietf-poly1305",
	}
	raw, err := json.Marshal(extra)
	if err != nil {
		t.Fatal(err)
	}
	cfg := BuildTunnelModeConfig(EngineConfig{
		Proxy: ProxyConfig{
			IP:       "example.com",
			Port:     443,
			Type:     "SS",
			Password: "pass",
			Extra:    raw,
		},
		Mode:       ProxyModeTunnel,
		DNSServers: []string{"8.8.8.8", "1.1.1.1"},
	})
	j, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := include.Context(context.Background())
	var opt option.Options
	if err := singjson.UnmarshalContext(ctx, j, &opt); err != nil {
		t.Fatalf("parsing options: %v", err)
	}
}
