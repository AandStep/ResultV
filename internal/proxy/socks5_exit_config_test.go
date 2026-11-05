package proxy

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"resultproxy-wails/internal/config"
)

func TestBuildTunnelSocks5Exit(t *testing.T) {
	cfg := EngineConfig{
		Mode: ProxyModeTunnel, RoutingMode: ModeGlobal, LocalPort: 14081,
		Proxy:     ProxyConfig{Type: "VLESS", IP: "192.0.2.1", Port: 443},
		Whitelist: []string{"direct.example.test"},
		Socks5Exit: &config.Socks5ExitConfig{
			Host: "exit.socks.test", Port: 8000, Username: "test-user", Password: "test-password",
			Domains: []string{"example.test"},
		},
	}
	for _, enabled := range []bool{false, true} {
		cfg.Socks5ExitEnabled = enabled
		out := mustBuildTunnelModeConfig(t, cfg)
		exit := mustOutbound(t, out, "socks5-exit")
		if exit.Type != socks5ExitOutboundType || exit.Version != "5" || exit.Network != "tcp" || exit.Detour != "proxy" {
			t.Fatalf("invalid SOCKS5 exit graph: %s/%s/%s/%s", exit.Type, exit.Version, exit.Network, exit.Detour)
		}
		if want := firstDetourServerTag(out.DNS.Servers, "proxy"); want == "" || exit.DomainResolver != want {
			t.Fatal("SOCKS5 hostname resolver does not use the first hop")
		}
		selector := mustOutbound(t, out, "socks5-exit-switch")
		want := "proxy"
		if enabled {
			want = "socks5-exit"
		}
		if selector.Type != socks5ExitSelectorType || selector.Default != want || !selector.InterruptExistConnections ||
			!reflect.DeepEqual(selector.Outbounds, []string{"proxy", "socks5-exit"}) {
			t.Fatalf("invalid selector for enabled=%t", enabled)
		}
		rules := out.Route.Rules
		last := rules[len(rules)-1]
		if last.Outbound != "socks5-exit-switch" || !reflect.DeepEqual(last.DomainSuffix, []string{"example.test"}) ||
			!reflect.DeepEqual(last.Inbound, []string{"tun-in"}) || out.Route.Final != "proxy" {
			t.Fatal("SOCKS5 route is not a TUN-only rule before the ordinary VPN default")
		}
		if !slices.ContainsFunc(rules[:len(rules)-1], func(rule SBRouteRule) bool {
			return rule.Outbound == "direct" && slices.Contains(rule.DomainSuffix, "direct.example.test")
		}) {
			t.Fatal("explicit site exclusion lost priority over SOCKS5")
		}
		assertCoreAcceptsConfig(t, out)
	}
	cfg.RoutingMode = ModeSmart
	if _, err := BuildTunnelModeConfig(cfg); err == nil {
		t.Fatal("enabled SOCKS5 accepted an unsupported session")
	}
	cfg.Socks5ExitEnabled = false
	if got := mustBuildTunnelModeConfig(t, cfg); slices.ContainsFunc(got.Outbounds, func(out SBOutbound) bool {
		return out.Tag == "socks5-exit" || out.Tag == "socks5-exit-switch"
	}) {
		t.Fatal("unsupported mode constructed the SOCKS5 route")
	}
}

func TestSocks5ExitDiagnosticsRedact(t *testing.T) {
	cfg := SingBoxConfig{
		Outbounds: []SBOutbound{
			{Type: "socks", Tag: "socks5-exit", Username: "diag-test-user", Password: "diag-test-password"},
			{Type: "vless", Tag: "proxy"},
		},
	}
	diagnostic, err := marshalDiagnosticConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	full, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range [][]byte{[]byte("diag-test-user"), []byte("diag-test-password")} {
		if bytes.Contains(diagnostic, secret) || !bytes.Contains(full, secret) {
			t.Fatal("diagnostic redaction exposed the secret or altered the live config")
		}
	}
}
