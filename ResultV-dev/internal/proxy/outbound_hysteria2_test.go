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
	"testing"
)

func TestBuildProxyOutboundHysteria2_UsesServerNameAndPasswordFallback(t *testing.T) {
	extraRaw, err := json.Marshal(map[string]interface{}{
		"server_name": "edge.example.com",
		"insecure":    true,
	})
	if err != nil {
		t.Fatalf("marshal extra: %v", err)
	}
	out := buildProxyOutbound(ProxyConfig{
		IP:       "example.com",
		Port:     443,
		Type:     "hysteria2",
		Password: "from-proxy",
		Extra:    extraRaw,
	})
	if out.Type != "hysteria2" {
		t.Fatalf("unexpected outbound type: %s", out.Type)
	}
	if out.Password != "from-proxy" {
		t.Fatalf("expected password fallback from proxy, got %q", out.Password)
	}
	if out.TLS == nil || out.TLS.ServerName != "edge.example.com" || !out.TLS.Insecure {
		t.Fatalf("unexpected tls config: %+v", out.TLS)
	}
}

func TestBuildProxyOutboundHysteria2_ParsesALPNAndObfs(t *testing.T) {
	extraRaw, err := json.Marshal(map[string]interface{}{
		"password":      "secret",
		"sni":           "sni.example.com",
		"alpn":          "h3,hysteria",
		"obfs_type":     "salamander",
		"obfs_password": "mask",
	})
	if err != nil {
		t.Fatalf("marshal extra: %v", err)
	}
	out := buildProxyOutbound(ProxyConfig{
		IP:    "example.com",
		Port:  443,
		Type:  "HYSTERIA2",
		Extra: extraRaw,
	})
	if out.TLS == nil {
		t.Fatal("expected tls config")
	}
	if len(out.TLS.ALPN) != 2 || out.TLS.ALPN[0] != "h3" || out.TLS.ALPN[1] != "hysteria" {
		t.Fatalf("unexpected alpn: %+v", out.TLS.ALPN)
	}
	if out.Obfs == nil || out.Obfs.Type != "salamander" || out.Obfs.Password != "mask" {
		t.Fatalf("unexpected obfs: %+v", out.Obfs)
	}
}

func TestBuildProxyOutboundHysteria2_DefaultALPNAndSNI(t *testing.T) {
	extraRaw, err := json.Marshal(map[string]interface{}{
		"password": "secret",
	})
	if err != nil {
		t.Fatalf("marshal extra: %v", err)
	}
	out := buildProxyOutbound(ProxyConfig{
		IP:    "hy.example.com",
		Port:  443,
		Type:  "hysteria2",
		Extra: extraRaw,
	})
	if out.TLS == nil {
		t.Fatal("expected tls")
	}
	if out.TLS.ServerName != "hy.example.com" {
		t.Fatalf("expected server_name fallback to host, got %q", out.TLS.ServerName)
	}
	if len(out.TLS.ALPN) != 2 || out.TLS.ALPN[0] != "h3" || out.TLS.ALPN[1] != "hysteria" {
		t.Fatalf("unexpected default ALPN: %+v", out.TLS.ALPN)
	}
}
