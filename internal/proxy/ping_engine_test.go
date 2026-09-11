package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
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

func TestBuildPingProbeConfigRejectsWireGuard(t *testing.T) {
	node := vlessProbeNode()
	node.Type = "WIREGUARD"
	_, err := BuildPingProbeConfig(node, 14999, "")
	if !errors.Is(err, errPingProbeUnsupported) {
		t.Fatalf("WireGuard carries no arbitrary TCP; want errPingProbeUnsupported, got %v", err)
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
