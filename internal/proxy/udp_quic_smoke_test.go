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

package proxy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

// TestVLESSXUDPOutboundShapeSmoke verifies that a default VLESS outbound
// includes packet_encoding=xudp (so UDP/QUIC traffic gets framed correctly)
// and that the config parses through sing-box's strict option decoder.
func TestVLESSXUDPOutboundShapeSmoke(t *testing.T) {
	extra := map[string]interface{}{
		"uuid":     "af815621-b245-4149-89da-dd184cfc4b3d",
		"network":  "tcp",
		"security": "tls",
		"sni":      "example.com",
		"flow":     "xtls-rprx-vision",
	}
	raw, _ := json.Marshal(extra)
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Proxy:      ProxyConfig{IP: "1.2.3.4", Port: 443, Type: "VLESS", Extra: raw},
		Mode:       ProxyModeProxy,
		ListenAddr: "127.0.0.1:14081",
	})
	j, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(j), `"packet_encoding":"xudp"`) {
		t.Fatalf("expected packet_encoding=xudp in config: %s", j)
	}
	ctx := include.Context(context.Background())
	var opt option.Options
	if err := singjson.UnmarshalContext(ctx, j, &opt); err != nil {
		t.Fatalf("strict decode of VLESS config failed: %v", err)
	}
}

// TestHysteria2QUICShapeSmoke verifies that a Hysteria2 outbound contains
// the required TLS block (QUIC depends on it) and parses strictly.
func TestHysteria2QUICShapeSmoke(t *testing.T) {
	extra := map[string]interface{}{
		"password":  "p",
		"sni":       "example.com",
		"alpn":      "h3",
		"up_mbps":   100,
		"down_mbps": 200,
	}
	raw, _ := json.Marshal(extra)
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Proxy:      ProxyConfig{IP: "example.com", Port: 443, Type: "HYSTERIA2", Extra: raw},
		Mode:       ProxyModeProxy,
		ListenAddr: "127.0.0.1:14082",
	})
	j, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	js := string(j)
	for _, want := range []string{`"type":"hysteria2"`, `"up_mbps":100`, `"down_mbps":200`, `"alpn":["h3"]`} {
		if !strings.Contains(js, want) {
			t.Fatalf("missing %q: %s", want, js)
		}
	}
	ctx := include.Context(context.Background())
	var opt option.Options
	if err := singjson.UnmarshalContext(ctx, j, &opt); err != nil {
		t.Fatalf("strict decode of Hysteria2 config failed: %v", err)
	}
}

// TestTunInboundUDPTimeoutShapeSmoke verifies that the tunnel-mode TUN
// inbound emits udp_timeout + endpoint_independent_nat, AND that sing-box's
// strict option decoder accepts those keys. Without this test a typo or a
// future version drop in our sing-box fork would silently break VPN startup
// for every user (strict decoder rejects unknown fields).
func TestTunInboundUDPTimeoutShapeSmoke(t *testing.T) {
	extra := map[string]interface{}{
		"uuid":     "af815621-b245-4149-89da-dd184cfc4b3d",
		"network":  "tcp",
		"security": "tls",
		"sni":      "example.com",
		"flow":     "xtls-rprx-vision",
	}
	raw, _ := json.Marshal(extra)
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy:    ProxyConfig{IP: "1.2.3.4", Port: 443, Type: "VLESS", Extra: raw},
		Mode:     ProxyModeTunnel,
		TunStack: "system",
	})
	j, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	js := string(j)
	for _, want := range []string{`"udp_timeout":"30s"`, `"endpoint_independent_nat":true`, `"type":"tun"`} {
		if !strings.Contains(js, want) {
			t.Fatalf("missing %q in tunnel-mode config: %s", want, js)
		}
	}
	ctx := include.Context(context.Background())
	var opt option.Options
	if err := singjson.UnmarshalContext(ctx, j, &opt); err != nil {
		t.Fatalf("strict decode of tunnel-mode config rejected new TUN fields: %v", err)
	}
}

// TestServerPinHostsDNSShapeSmoke verifies that a domain-addressed server with
// connect-time-resolved backends emits a `hosts` DNS server carrying the
// predefined IP set, the outbound KEEPS the domain (so sing-box can fail over),
// AND that sing-box's strict option decoder accepts the `hosts`/`predefined`
// shape. Without this, a typo or a fork version drop in the hosts-pin would
// silently break VPN startup for every domain/CDN server (strict decoder
// rejects unknown fields) — the very regression-fix path could become a worse
// regression.
func TestServerPinHostsDNSShapeSmoke(t *testing.T) {
	extra := map[string]interface{}{"password": "p", "sni": "k.example.com"}
	raw, _ := json.Marshal(extra)
	cfg := mustBuildTunnelModeConfig(t, EngineConfig{
		Proxy: ProxyConfig{
			IP:          "k.example.com",
			ResolvedIP:  "203.0.113.7",
			ResolvedIPs: []string{"203.0.113.7", "203.0.113.8"},
			Port:        443,
			Type:        "HYSTERIA2",
			Extra:       raw,
		},
		Mode:     ProxyModeTunnel,
		TunStack: "system",
	})
	j, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	js := string(j)
	for _, want := range []string{
		`"type":"hosts"`,
		`"tag":"server-pin"`,
		`"predefined":{"k.example.com":["203.0.113.7","203.0.113.8"]}`,
		`"server":"server-pin"`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("missing %q in pinned tunnel config: %s", want, js)
		}
	}
	// The outbound must dial the domain, not a single pinned IP.
	if !strings.Contains(js, `"server":"k.example.com"`) {
		t.Fatalf("outbound must keep the server domain for failover: %s", js)
	}
	ctx := include.Context(context.Background())
	var opt option.Options
	if err := singjson.UnmarshalContext(ctx, j, &opt); err != nil {
		t.Fatalf("strict decode rejected the hosts server-pin DNS shape: %v", err)
	}
}

// TestVMessUDPShapeSmoke verifies that VMess outbound also gets xudp by
// default and that global_padding / authenticated_length round-trip.
func TestVMessUDPShapeSmoke(t *testing.T) {
	extra := map[string]interface{}{
		"uuid":                 "af815621-b245-4149-89da-dd184cfc4b3d",
		"network":              "tcp",
		"security":             "none",
		"global_padding":       true,
		"authenticated_length": true,
		"scy":                  "auto",
	}
	raw, _ := json.Marshal(extra)
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Proxy:      ProxyConfig{IP: "1.2.3.4", Port: 443, Type: "VMESS", Extra: raw},
		Mode:       ProxyModeProxy,
		ListenAddr: "127.0.0.1:14083",
	})
	j, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	js := string(j)
	for _, want := range []string{`"packet_encoding":"xudp"`, `"global_padding":true`, `"authenticated_length":true`, `"security":"auto"`} {
		if !strings.Contains(js, want) {
			t.Fatalf("missing %q: %s", want, js)
		}
	}
	ctx := include.Context(context.Background())
	var opt option.Options
	if err := singjson.UnmarshalContext(ctx, j, &opt); err != nil {
		t.Fatalf("strict decode of VMess config failed: %v", err)
	}
}
