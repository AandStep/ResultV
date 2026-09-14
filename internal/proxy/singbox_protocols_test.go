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
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
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
			"jc":   7,
			"jmin": 10,
			"jmax": 20,
			"s1":   1,
			"h1":   11,
			"i1":   "abc",
			// j1/itime are engine-unsupported and must be dropped, see
			// TestAmneziaDropsKeysUnsupportedByEngine.
			"j1":    "def",
			"itime": 42,
		},
	}
	raw, err := json.Marshal(extra)
	if err != nil {
		t.Fatal(err)
	}
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
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
	if ep.Amnezia.I1 != "abc" {
		t.Fatalf("unexpected amnezia i1: %+v", ep.Amnezia)
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
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
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

func TestNaiveOutboundConfigParses(t *testing.T) {
	extra := map[string]interface{}{
		"sni": "tls.example.com",
	}
	raw, err := json.Marshal(extra)
	if err != nil {
		t.Fatal(err)
	}
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{
			IP:       "srv.example.com",
			Port:     443,
			Type:     "NAIVEPROXY",
			Username: "u1",
			Password: "p1",
			Extra:    raw,
		},
		Mode:       ProxyModeProxy,
		ListenAddr: "127.0.0.1:14081",
	})
	var proxyOB *SBOutbound
	for i := range cfg.Outbounds {
		if cfg.Outbounds[i].Tag == "proxy" {
			proxyOB = &cfg.Outbounds[i]
			break
		}
	}
	if proxyOB == nil || proxyOB.Type != "naive" {
		t.Fatalf("outbound: %+v", proxyOB)
	}
	if proxyOB.TLS == nil || !proxyOB.TLS.Enabled || proxyOB.TLS.ServerName != "tls.example.com" {
		t.Fatalf("tls: %+v", proxyOB.TLS)
	}
	if proxyOB.TLS.UTLS != nil {
		t.Fatal("utls must be omitted for sing-box naive outbound")
	}
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
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
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
// TestVMessOutboundConfigParses pins the VMess shape against the core's strict
// decoder: cipher, packet encoding and the VMess-only padding flags travel as
// their own JSON keys, and a rename on the core side would otherwise surface as
// a dead engine for every VMess node rather than a test failure.
func TestVMessOutboundConfigParses(t *testing.T) {
	cfg := tunnelConfigFromExtra(t, "VMESS", map[string]interface{}{
		"uuid":                 "af815621-b245-4149-89da-dd184cfc4b3d",
		"alterId":              0,
		"security_cipher":      "auto",
		"packet_encoding":      "xudp",
		"global_padding":       true,
		"authenticated_length": true,
		"network":              "ws",
		"path":                 "/ws",
		"host":                 "example.com",
		"security":             "tls",
		"sni":                  "example.com",
	})
	if len(cfg.Outbounds) == 0 {
		t.Fatal("no outbounds built")
	}
	assertCoreAcceptsConfig(t, cfg)
}

// TestTrojanOutboundConfigParses covers the branch that synthesises TLS when the
// node did not ask for it explicitly (see buildProxyOutboundRaw) — the ALPN list
// it picks there is computed, not copied, so it is exactly the kind of value a
// stricter enum check in a new core would reject.
func TestTrojanOutboundConfigParses(t *testing.T) {
	cfg := tunnelConfigFromExtra(t, "TROJAN", map[string]interface{}{
		"sni":      "example.com",
		"fp":       "chrome",
		"alpn":     "h2,http/1.1",
		"network":  "tcp",
		"insecure": false,
	})
	assertCoreAcceptsConfig(t, cfg)
}

// TestSocksOutboundConfigParses is the cheapest protocol we emit and the one
// most likely to be forgotten: version is a string ("5"), not a number, and the
// core validates it while decoding.
func TestSocksOutboundConfigParses(t *testing.T) {
	cfg := tunnelConfigFromExtra(t, "SOCKS5", map[string]interface{}{})
	assertCoreAcceptsConfig(t, cfg)
}
