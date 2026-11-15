package proxy

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"resultproxy-wails/internal/config"
)

func listedExit(sites, apps []string) *config.Socks5ExitConfig {
	return &config.Socks5ExitConfig{Enabled: true, Host: "192.0.2.2", Port: 1080, Sites: sites, Apps: apps}
}

func TestSocks5ExitListGlobal(t *testing.T) {
	cfg := EngineConfig{
		Mode: ProxyModeTunnel, RoutingMode: ModeGlobal, LocalPort: 14081,
		Proxy:      ProxyConfig{Type: "VLESS", IP: "192.0.2.1", Port: 443},
		Whitelist:  []string{"direct.example.test"},
		Socks5Exit: listedExit([]string{"example.test", "direct.example.test", "198.51.100.0/24"}, nil),
	}
	out := mustBuildTunnelModeConfig(t, cfg)
	if out.Route.Final != "proxy" {
		t.Fatalf("final = %s, want proxy", out.Route.Final)
	}
	rules := out.Route.Rules
	listed := indexOfRule(rules, func(r SBRouteRule) bool {
		return r.Outbound == socks5ExitSelector && slices.Contains(r.DomainSuffix, "example.test")
	})
	cidr := indexOfRule(rules, func(r SBRouteRule) bool {
		return r.Outbound == socks5ExitSelector && slices.Contains(r.IPCidr, "198.51.100.0/24")
	})
	if listed < 0 || cidr < 0 {
		t.Fatal("listed site or range does not use the SOCKS5 selector")
	}
	viaExit := indexOfRule(rules, func(r SBRouteRule) bool {
		return r.Outbound == socks5ExitSelector && slices.Contains(r.DomainSuffix, "direct.example.test")
	})
	direct := indexOfRule(rules, func(r SBRouteRule) bool {
		return r.Outbound == "direct" && slices.Contains(r.DomainSuffix, "direct.example.test")
	})
	if viaExit < 0 || direct < 0 || viaExit > direct {
		t.Fatalf("SOCKS5 list must outrank exclusions: socks=%d direct=%d", viaExit, direct)
	}
	for _, r := range rules {
		if r.Outbound == socks5ExitSelector && len(r.DomainSuffix) == 0 && len(r.IPCidr) == 0 && len(r.PackageName) == 0 {
			t.Fatalf("a VPN rule outside the list was moved to the exit: %+v", r)
		}
	}
	last := rules[len(rules)-1]
	if last.Action != "reject" || !slices.Equal(last.Network, []string{"udp"}) || !slices.Equal(last.Port, []int{443}) ||
		len(last.DomainSuffix) != 0 || len(last.Inbound) != 0 {
		t.Fatalf("HTTP/3 with an unknown name can reach the VPN exit: last rule %+v", last)
	}
	assertCoreAcceptsConfig(t, out)
}

func TestSocks5ExitListSmart(t *testing.T) {
	cfg := EngineConfig{
		Mode: ProxyModeTunnel, RoutingMode: ModeSmart, LocalPort: 14081,
		Proxy:          ProxyConfig{Type: "HYSTERIA2", IP: "192.0.2.1", Port: 443},
		BlockedDomains: []string{"blocked.example.test"},
		Socks5Exit:     listedExit([]string{"example.test"}, nil),
	}
	out := mustBuildTunnelModeConfig(t, cfg)
	if out.Route.Final != "direct" {
		t.Fatal("Smart lost the direct default")
	}
	if indexOfRule(out.Route.Rules, func(r SBRouteRule) bool {
		return r.Outbound == socks5ExitSelector && slices.Contains(r.DomainSuffix, "example.test")
	}) < 0 {
		t.Fatal("listed site does not use the SOCKS5 selector")
	}
	if indexOfRule(out.Route.Rules, func(r SBRouteRule) bool {
		return r.Outbound == "proxy" && slices.Contains(r.DomainSuffix, "blocked.example.test")
	}) < 0 {
		t.Fatal("Smart VPN route moved to the exit although a list is set")
	}
	tunnel := firstDetourServerTag(out.DNS.Servers, "proxy")
	if tunnel == "" || !slices.ContainsFunc(out.DNS.Rules, func(r SBDNSRule) bool {
		return r.Server == tunnel && slices.Contains(r.DomainSuffix, "example.test")
	}) {
		t.Fatal("a listed site resolves outside the tunnel")
	}
	cfg.AdaptiveSmart = true
	adaptive := mustBuildTunnelModeConfig(t, cfg)
	if smart := mustOutbound(t, adaptive, smartOutboundTag); !reflect.DeepEqual(smart.Outbounds, []string{"direct", "proxy"}) {
		t.Fatalf("adaptive legs = %v", smart.Outbounds)
	}
	assertCoreAcceptsConfig(t, adaptive)
}

func TestSocks5ExitListApps(t *testing.T) {
	cfg := EngineConfig{
		Mode: ProxyModeTunnel, RoutingMode: ModeGlobal, LocalPort: 14081,
		Proxy:        ProxyConfig{Type: "VLESS", IP: "192.0.2.1", Port: 443},
		AppWhitelist: []string{"other.exe"},
		Socks5Exit:   listedExit(nil, []string{"tool.exe"}),
	}
	route := buildRoute(cfg)
	socks := indexOfRule(route.Rules, func(r SBRouteRule) bool {
		return r.Outbound == socks5ExitSelector && slices.Contains(r.PackageName, familyTagSocks)
	})
	direct := indexOfRule(route.Rules, func(r SBRouteRule) bool { return slices.Contains(r.PackageName, familyTagDirect) })
	if socks < 0 || direct < 0 || socks > direct || !route.FindProcess {
		t.Fatalf("app family rule: socks=%d direct=%d find_process=%t", socks, direct, route.FindProcess)
	}
	cfg.Mode = ProxyModeProxy
	if indexOfRule(buildRoute(cfg).Rules, func(r SBRouteRule) bool { return slices.Contains(r.PackageName, familyTagSocks) }) >= 0 {
		t.Fatal("app list applied in proxy mode")
	}
}

func TestSocks5ExitListBlocksWin(t *testing.T) {
	file := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(file, []byte(`{"version":3,"rules":[{"domain_suffix":["example"]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := EngineConfig{
		Mode: ProxyModeProxy, RoutingMode: ModeGlobal, FirstHopProbePort: 14082,
		Proxy:        ProxyConfig{Type: "ss", IP: "192.0.2.1", Port: 443, Password: "test"},
		RoutingLists: []RoutingListSpec{{Tag: "profile-block", Path: file, Action: "block"}},
		RoutingOrder: []string{"block", "direct", "proxy"},
		Socks5Exit:   listedExit([]string{"via.example"}, nil),
	}
	rules := buildRoute(cfg).Rules
	block := indexOfRule(rules, func(r SBRouteRule) bool {
		return r.Action == "reject" && len(r.Rules) == 2 && slices.Contains(r.Rules[0].DomainSuffix, "via.example")
	})
	socks := indexOfRule(rules, func(r SBRouteRule) bool {
		return r.Outbound == socks5ExitSelector && slices.Contains(r.DomainSuffix, "via.example")
	})
	if block < 0 || socks < 0 || block > socks {
		t.Fatalf("profile block must precede the SOCKS5 list: block=%d socks=%d", block, socks)
	}
	assertCoreBuildsConfig(t, mustBuildProxyModeConfig(t, cfg))
}

func TestAppListsSocksWins(t *testing.T) {
	lists := newAppLists([]string{"tool.exe"}, []string{"tool.exe"}).withSocks([]string{"tool.exe"})
	if got := lists.own(`C:\Tools\tool.exe`); got != familySocks {
		t.Fatalf("family = %v, want socks", got)
	}
	if familySocks.tag() != "resultv.socks" || lists.empty() {
		t.Fatal("socks family tag or emptiness")
	}
}
