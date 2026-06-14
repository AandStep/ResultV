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
	"strings"
	"testing"
)

// When a domain server is pinned to an IP at connect time, the sing-box outbound
// must dial the IP while keeping the original domain as the TLS SNI — otherwise
// the cert check would fail. This is the core of the false-kill-switch fix:
// sing-box never re-resolves the server domain mid-session.
func TestBuildProxyOutbound_PinnedResolvedIPKeepsDomainSNI(t *testing.T) {
	out := buildProxyOutbound(ProxyConfig{
		IP:         "k.example.com",
		ResolvedIP: "203.0.113.7",
		Port:       3443,
		Type:       "hysteria2",
		Password:   "pw",
	})
	if out.Server != "203.0.113.7" {
		t.Fatalf("expected outbound to dial the pinned IP, got %q", out.Server)
	}
	if out.TLS == nil || out.TLS.ServerName != "k.example.com" {
		t.Fatalf("expected SNI to stay the original domain, got %+v", out.TLS)
	}
}

// Without a pinned IP the outbound keeps the raw host (domain or literal),
// so the change is inert for manually-entered IP servers.
func TestBuildProxyOutbound_NoPinKeepsRawServer(t *testing.T) {
	out := buildProxyOutbound(ProxyConfig{
		IP:       "198.51.100.10",
		Port:     443,
		Type:     "hysteria2",
		Password: "pw",
	})
	if out.Server != "198.51.100.10" {
		t.Fatalf("expected raw server preserved, got %q", out.Server)
	}
}

func TestResolvePinnedServerIP_LiteralIPReturnsEmpty(t *testing.T) {
	if got := resolvePinnedServerIP("203.0.113.7"); got != "" {
		t.Fatalf("literal IP needs no pin, got %q", got)
	}
	if got := resolvePinnedServerIP(""); got != "" {
		t.Fatalf("empty host returns empty, got %q", got)
	}
}

// Subscription servers must never leak the provider backend address into logs.
func TestSingBoxLogWriter_RedactsSubscriptionServer(t *testing.T) {
	w := newSingBoxLogWriter(nil, ProxyConfig{
		IP:              "k.example.com",
		ResolvedIP:      "203.0.113.7",
		SubscriptionURL: "https://sub.example/abc",
	})
	in := "connection: open connection ... using outbound/hysteria2[proxy]: lookup k.example.com: context deadline exceeded (203.0.113.7)"
	got := w.redactServer(in)
	if strings.Contains(got, "k.example.com") || strings.Contains(got, "203.0.113.7") {
		t.Fatalf("server identifiers leaked after redaction: %q", got)
	}
	if !strings.Contains(got, "<сервер>") {
		t.Fatalf("expected placeholder in redacted message: %q", got)
	}
}

// Manual (non-subscription) servers keep full detail — the user owns them.
func TestSingBoxLogWriter_ManualServerNotRedacted(t *testing.T) {
	w := newSingBoxLogWriter(nil, ProxyConfig{IP: "198.51.100.10", Port: 443})
	in := "lookup 198.51.100.10 failed"
	if got := w.redactServer(in); got != in {
		t.Fatalf("manual server should not be redacted, got %q", got)
	}
}
