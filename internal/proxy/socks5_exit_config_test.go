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
		},
	}
	{
		cfg.Socks5Exit.Enabled = true
		out := mustBuildTunnelModeConfig(t, cfg)
		exit := mustOutbound(t, out, "socks5-exit")
		if exit.Type != socks5ExitOutboundType || exit.Version != "5" || exit.Network != "tcp" || exit.Detour != "proxy" {
			t.Fatalf("invalid SOCKS5 exit graph: %s/%s/%s/%s", exit.Type, exit.Version, exit.Network, exit.Detour)
		}
		if want := firstDetourServerTag(out.DNS.Servers, "proxy"); want == "" || exit.DomainResolver != want {
			t.Fatal("SOCKS5 hostname resolver does not use the first hop")
		}
		selector := mustOutbound(t, out, "socks5-exit-switch")
		if selector.Type != socks5ExitSelectorType || selector.Default != "socks5-exit" || !selector.InterruptExistConnections ||
			!reflect.DeepEqual(selector.Outbounds, []string{"proxy", "socks5-exit", "socks5-exit-next"}) {
			t.Fatal("invalid selector for an enabled exit")
		}
		rules := out.Route.Rules
		if rules[0].Outbound != "proxy" || !slices.Contains(rules[0].Inbound, probeInboundTag) {
			t.Fatal("an ordinary domain rule can intercept a first-hop health probe")
		}
		if out.Route.Final != socks5ExitSelector {
			t.Fatal("global traffic does not use the SOCKS5 selector")
		}
		if !slices.ContainsFunc(rules, func(rule SBRouteRule) bool {
			return rule.Outbound == "direct" && slices.Contains(rule.DomainSuffix, "direct.example.test")
		}) {
			t.Fatal("explicit site exclusion lost priority over SOCKS5")
		}
		assertCoreAcceptsConfig(t, out)
	}
	for _, profile := range []*config.Socks5ExitConfig{nil, {}, {Enabled: true}, {Host: "exit.socks.test", Port: 8000}} {
		cfg.Socks5Exit = profile
		got := mustBuildTunnelModeConfig(t, cfg)
		if slices.ContainsFunc(got.Outbounds, func(out SBOutbound) bool {
			return out.Tag == socks5ExitTag || out.Tag == socks5ExitSelector
		}) || got.Route.Final != "proxy" {
			t.Fatalf("profile %+v without an address changed the VPN route", profile)
		}
	}
}

func TestSocks5ExitWithoutAddressKeepsProxyModeUnchanged(t *testing.T) {
	cfg := EngineConfig{
		Mode: ProxyModeProxy, RoutingMode: ModeGlobal, LocalPort: 14081,
		Proxy:      ProxyConfig{Type: "HTTP", IP: "192.0.2.1", Port: 8000},
		Socks5Exit: &config.Socks5ExitConfig{Enabled: true},
	}
	out := mustBuildProxyModeConfig(t, cfg)
	if out.Route.Final != "proxy" || slices.ContainsFunc(out.Outbounds, func(out SBOutbound) bool {
		return out.Tag == socks5ExitSelector
	}) {
		t.Fatal("proxy mode built the SOCKS5 route without an address")
	}
}

func TestSocks5ExitFollowsVPNInSmartMode(t *testing.T) {
	cfg := EngineConfig{
		Mode: ProxyModeTunnel, RoutingMode: ModeSmart, LocalPort: 14081,
		Proxy:          ProxyConfig{Type: "HYSTERIA2", IP: "192.0.2.1", Port: 443},
		BlockedDomains: []string{"example.test"},
		Socks5Exit:     &config.Socks5ExitConfig{Enabled: true, Host: "192.0.2.2", Port: 1080},
	}
	out := mustBuildTunnelModeConfig(t, cfg)
	if out.Route.Final != "direct" {
		t.Fatal("Smart lost the direct default")
	}
	if !slices.ContainsFunc(out.Route.Rules, func(rule SBRouteRule) bool {
		return rule.Outbound == socks5ExitSelector && slices.Contains(rule.DomainSuffix, "example.test")
	}) {
		t.Fatal("Smart VPN route did not use the SOCKS5 selector")
	}
	cfg.AdaptiveSmart = true
	adaptive := mustBuildTunnelModeConfig(t, cfg)
	if adaptive.Route.Final != smartOutboundTag {
		t.Fatal("adaptive Smart lost its own default")
	}
	smart := mustOutbound(t, adaptive, smartOutboundTag)
	if !reflect.DeepEqual(smart.Outbounds, []string{"direct", socks5ExitSelector}) {
		t.Fatal("adaptive Smart VPN leg does not use the selector")
	}
	assertCoreAcceptsConfig(t, adaptive)
}

func TestSocks5ExitProxyModeKeepsFirstHopProbes(t *testing.T) {
	cfg := EngineConfig{
		Mode: ProxyModeProxy, RoutingMode: ModeGlobal, LocalPort: 14081, FirstHopProbePort: 14082,
		Proxy:      ProxyConfig{Type: "HTTP", IP: "192.0.2.1", Port: 8000},
		Socks5Exit: &config.Socks5ExitConfig{Enabled: true, Host: "192.0.2.2", Port: 1080},
	}
	out := mustBuildProxyModeConfig(t, cfg)
	if out.Route.Final != socks5ExitSelector ||
		out.Route.Rules[0].Outbound != "proxy" || !slices.Contains(out.Route.Rules[0].Inbound, probeInboundTag) {
		t.Fatal("proxy mode does not separate user traffic and the first-hop health check")
	}
	assertCoreAcceptsConfig(t, out)
}

func TestSocks5ExitUsesWireGuardEndpointAsFirstHop(t *testing.T) {
	cfg := adaptiveWireGuardConfig(t)
	cfg.Socks5Exit = &config.Socks5ExitConfig{Enabled: true, Host: "socks.example.test", Port: 1080}
	out := mustBuildTunnelModeConfig(t, cfg)
	exit := mustOutbound(t, out, socks5ExitTag)
	if exit.Detour != "proxy" || len(out.Endpoints) != 1 || out.Endpoints[0].Tag != exit.Detour {
		t.Fatal("the SOCKS5 dialer does not point to the WireGuard endpoint")
	}
	assertCoreBuildsConfig(t, out)
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
