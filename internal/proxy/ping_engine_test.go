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
	"net/http"
	"testing"
	"time"
)

func vlessProbeNode() ProxyConfig {
	return ProxyConfig{
		ID:    "n1",
		IP:    "node.example.com",
		Port:  443,
		Type:  "VLESS",
		Extra: json.RawMessage(`{"uuid":"11111111-1111-1111-1111-111111111111","security":"tls","sni":"node.example.com"}`),
		// ResolvedIPs is what connect time learned; the probe engine reuses it
		// as a static hosts record instead of asking the OS resolver.
		ResolvedIPs: []string{"203.0.113.10"},
	}
}

func TestBuildPingProbeConfigShape(t *testing.T) {
	cfg, err := BuildPingProbeConfig(vlessProbeNode(), 14999, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Inbounds) != 1 {
		t.Fatalf("want exactly one inbound, got %d", len(cfg.Inbounds))
	}
	in := cfg.Inbounds[0]
	if in.Type != "mixed" || in.Listen != "127.0.0.1" || in.ListenPort != 14999 {
		t.Fatalf("probe inbound must be a loopback mixed listener, got %+v", in)
	}
	if in.AutoRoute || in.StrictRoute {
		t.Fatal("probe inbound must never touch system routing")
	}
	if cfg.Route == nil || cfg.Route.Final != "proxy" {
		t.Fatalf("probe traffic must default to the node outbound, got %+v", cfg.Route)
	}
	if cfg.Experimental != nil {
		t.Fatal("probe engine must not open the shared sing-box cache database")
	}
}

func TestBuildPingProbeConfigKeepsServerDomainForSNI(t *testing.T) {
	cfg, err := BuildPingProbeConfig(vlessProbeNode(), 14999, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var proxyOut *SBOutbound
	for i := range cfg.Outbounds {
		if cfg.Outbounds[i].Tag == "proxy" {
			proxyOut = &cfg.Outbounds[i]
		}
	}
	if proxyOut == nil {
		t.Fatal("no proxy outbound")
	}
	// Rewriting Server to a literal would drag SNI down with it
	// (outbound.go: ServerName falls back to proxy.IP), breaking TLS for
	// every domain-addressed node.
	if proxyOut.Server != "node.example.com" {
		t.Fatalf("server host must stay the domain, got %q", proxyOut.Server)
	}
	if cfg.DNS == nil {
		t.Fatal("a domain-addressed node needs the static hosts record")
	}
	pin := cfg.DNS.Servers[0]
	if pin.Type != "hosts" || len(pin.Predefined["node.example.com"]) == 0 {
		t.Fatalf("server domain must be pinned locally, got %+v", pin)
	}
}

func TestBuildPingProbeConfigOmitsDNSForLiteralServer(t *testing.T) {
	node := vlessProbeNode()
	node.IP = "203.0.113.10"
	node.ResolvedIPs = nil
	cfg, err := BuildPingProbeConfig(node, 14999, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DNS != nil {
		t.Fatal("a literal server address needs no resolver at all")
	}
}

func TestBuildPingProbeConfigBindsOnlyWhenAsked(t *testing.T) {
	cfg, err := BuildPingProbeConfig(vlessProbeNode(), 14999, "192.168.1.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, out := range cfg.Outbounds {
		if out.Tag == "proxy" && out.Inet4BindAddress != "192.168.1.5" {
			t.Fatalf("probe must leave via the physical adapter, got %q", out.Inet4BindAddress)
		}
	}

	unbound, err := BuildPingProbeConfig(vlessProbeNode(), 14999, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, out := range unbound.Outbounds {
		if out.Inet4BindAddress != "" {
			t.Fatalf("no bind was asked for, got %q", out.Inet4BindAddress)
		}
	}
}

func TestBuildPingProbeConfigWireGuardStaysOffLiveSession(t *testing.T) {
	node := ProxyConfig{
		ID: "wg1", IP: "203.0.113.20", Port: 51820, Type: "AMNEZIAWG",
		Extra: json.RawMessage(`{"private_key":"k","public_key":"p","system":true,"name":"wg0","listen_port":51820,"persistent_keepalive_interval":25}`),
	}
	cfg, err := BuildPingProbeConfig(node, 14999, "192.168.1.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Route == nil || cfg.Route.Final != wireguardEndpointTag || len(cfg.Endpoints) != 1 {
		t.Fatalf("route %+v endpoints %d", cfg.Route, len(cfg.Endpoints))
	}
	ep := cfg.Endpoints[0]
	if ep.System || ep.Name != "" || ep.ListenPort != 0 || ep.Peers[0].PersistentKeepaliveInterval != 0 {
		t.Fatalf("probe endpoint would collide with the live session: %+v", ep)
	}
	for _, out := range cfg.Outbounds {
		if out.Tag == "direct" && out.Inet4BindAddress != "192.168.1.5" {
			t.Fatalf("endpoint must reach the server via the physical adapter, got %q", out.Inet4BindAddress)
		}
	}
}

func TestClassifyPingFetchAcceptsAnyStatus(t *testing.T) {
	// Over HTTPS the certificate is verified in this process, so a response
	// arriving at all already proves the bytes reached the real host. Status
	// is deliberately not a criterion: a user-chosen URL may answer 200, 204,
	// 301 or 404 and all of them mean "the node carried the request".
	for _, status := range []int{200, 204, 301, 404, 500} {
		ok, reason := classifyPingFetch(&http.Response{StatusCode: status}, nil)
		if !ok {
			t.Fatalf("status %d: want reachable, got reason %q", status, reason)
		}
	}
}

func TestClassifyPingFetchRejectsProxyAuth(t *testing.T) {
	ok, reason := classifyPingFetch(&http.Response{StatusCode: http.StatusProxyAuthRequired}, nil)
	if ok || reason != "proxy_auth_required" {
		t.Fatalf("got ok=%v reason=%q", ok, reason)
	}
}

func TestClassifyPingFetchReportsTransportError(t *testing.T) {
	ok, reason := classifyPingFetch(nil, context.DeadlineExceeded)
	if ok {
		t.Fatal("a transport error is never a successful measurement")
	}
	if reason == "" {
		t.Fatal("a failure must carry a reason the UI can show")
	}
}

// Compile-time seal: the probe engine must have no way to reach the user's log.
//
// It briefly did. closeInstanceBounded writes one line per teardown, the probe
// path handed it the Manager's logger, and a sweep that stands up a throwaway
// engine per node turned that into a stream of "Закрываем N соединений перед
// остановкой" in the user's own log — several lines a second, drowning the
// session's real events. The fix is the absent parameter, not a quieter call
// site: with no logger in the signature there is nothing to pass by mistake.
var _ func(context.Context, ProxyConfig, string, string, string) (int64, bool, string) = pingThroughNode

// Свежее ядро рвёт первое hy2-соединение стартовым ResetNetwork («network
// changed» через пару миллисекунд), и без повтора hy2-узел в пинге всегда
// выглядел мёртвым.
func TestFetchPingRetriesInstantFailureOnce(t *testing.T) {
	calls := 0
	ms, ok, reason := fetchPing(context.Background(), func() (time.Duration, bool, string) {
		calls++
		if calls == 1 {
			return 2 * time.Millisecond, false, "connection_closed"
		}
		return 180 * time.Millisecond, true, ""
	})
	if !ok || calls != 2 || ms != 180 {
		t.Fatalf("want retry and second latency: ok=%v calls=%d ms=%d reason=%q", ok, calls, ms, reason)
	}
}

func TestFetchPingGivesUpAfterBudget(t *testing.T) {
	calls := 0
	_, ok, reason := fetchPing(context.Background(), func() (time.Duration, bool, string) {
		calls++
		return 3 * time.Second, false, "timeout"
	})
	if ok || calls != pingAttempts || reason != "timeout" {
		t.Fatalf("dead node: ok=%v calls=%d reason=%q", ok, calls, reason)
	}
}

func TestFetchPingStopsWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, ok, _ := fetchPing(ctx, func() (time.Duration, bool, string) {
		calls++
		cancel()
		return time.Second, false, "timeout"
	})
	if ok || calls != 1 {
		t.Fatalf("no retry past the ping timeout: ok=%v calls=%d", ok, calls)
	}
}

func TestBuildPingProbeConfigShortensHysteria2Handshake(t *testing.T) {
	extra, _ := json.Marshal(map[string]interface{}{"password": "x"})
	cfg, err := BuildPingProbeConfig(ProxyConfig{IP: "1.2.3.4", Port: 443, Type: "hysteria2", Extra: extra}, 14999, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range cfg.Outbounds {
		if o.Tag == "proxy" && (o.TLS == nil || o.TLS.HandshakeTimeout != pingHysteria2HandshakeTimeout) {
			t.Fatalf("ping engine hy2 handshake_timeout = %+v", o.TLS)
		}
	}
}
