package proxy

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestPersonalRoutingKeepsModesIsolated(t *testing.T) {
	for _, tc := range []struct {
		mode                                   RoutingMode
		vpnSite, directSite, vpnApp, directApp string
	}{
		{ModeSmart, "first.example", "second.example", "first.exe", "second.exe"},
		{ModeGlobal, "second.example", "first.example", "second.exe", "first.exe"},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			cfg := EngineConfig{
				Mode: ProxyModeTunnel, RoutingMode: tc.mode,
				Whitelist: []string{"first.example"}, AppWhitelist: []string{"first.exe"},
				AppForceVPN: []string{"first.exe"}, BlockedDomains: []string{"second.example", "builtin.example"},
				PersonalRouting: PersonalRouting{
					SmartVPNSites: []string{"first.example"}, SmartDirectSites: []string{"second.example"},
					SmartDirectApps: []string{"second.exe"}, GlobalVPNSites: []string{"second.example"}, GlobalVPNApps: []string{"second.exe"},
				},
			}
			route := buildRoute(cfg)
			for domain, want := range map[string]string{tc.vpnSite: "proxy", tc.directSite: "direct"} {
				idx := indexOfRule(route.Rules, func(rule SBRouteRule) bool {
					return rule.Action == "route" && slices.Contains(rule.DomainSuffix, domain)
				})
				if idx < 0 || route.Rules[idx].Outbound != want {
					t.Errorf("%s should explicitly route to %s, rule index %d", domain, want, idx)
				}
			}
			dns := buildDNS(cfg)
			for domain, want := range map[string]string{tc.vpnSite: firstDetourServerTag(dns.Servers, "proxy"), tc.directSite: "local"} {
				if !slices.ContainsFunc(dns.Rules, func(r SBDNSRule) bool { return slices.Contains(r.DomainSuffix, domain) && r.Server == want }) {
					t.Errorf("missing DNS route for %s via %s", domain, want)
				}
			}
			if got := appFamilyVPNList(cfg); !slices.Equal(got, []string{tc.vpnApp}) {
				t.Errorf("VPN apps = %v, want %s", got, tc.vpnApp)
			}
			if indexOfRule(route.Rules, func(r SBRouteRule) bool {
				return slices.Contains(r.PackageName, familyTagDirect)
			}) < 0 {
				t.Errorf("missing direct family route for %s", tc.directApp)
			}
			if got := cfg.forRoutingMode().directApps(); !slices.Equal(got, []string{tc.directApp}) {
				t.Errorf("direct apps = %v, want %s", got, tc.directApp)
			}
		})
	}
}

func TestGlobalPersonalVPNOverridesProfileDirection(t *testing.T) {
	file := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(file, []byte(`{"version":3,"rules":[{"domain_suffix":["example"]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := EngineConfig{
		Mode: ProxyModeProxy, RoutingMode: ModeGlobal,
		Proxy: ProxyConfig{Type: "ss", IP: "192.0.2.1", Port: 443, Password: "test"},
		RoutingLists: []RoutingListSpec{
			{Tag: "profile-direct", Path: file, Action: "direct"},
			{Tag: "profile-block", Path: file, Action: "block"},
		},
		RoutingOrder:    []string{"direct", "block", "proxy"},
		PersonalRouting: PersonalRouting{GlobalVPNSites: []string{"via.example", "203.0.113.7"}},
	}
	rules := buildRoute(cfg).Rules
	personal := indexOfRule(rules, func(r SBRouteRule) bool {
		return r.Outbound == "proxy" && slices.Contains(r.DomainSuffix, "via.example")
	})
	ip := indexOfRule(rules, func(r SBRouteRule) bool { return r.Outbound == "proxy" && slices.Contains(r.IPCidr, "203.0.113.7/32") })
	direct := indexOfRule(rules, func(r SBRouteRule) bool { return slices.Contains(r.RuleSet, "profile-direct") })
	if personal < 0 || ip < 0 || direct < personal || direct < ip {
		t.Fatalf("personal VPN rules must precede profile direct: host=%d ip=%d profile=%d", personal, ip, direct)
	}
	block := indexOfRule(rules, func(r SBRouteRule) bool {
		return r.Action == "reject" && len(r.Rules) == 2 && slices.Contains(r.Rules[1].RuleSet, "profile-block")
	})
	if block < 0 || block > personal || block > ip {
		t.Fatal("profile blocks must be checked before personal VPN routes")
	}
	assertCoreBuildsConfig(t, mustBuildProxyModeConfig(t, cfg))
}

func TestPersonalRoutingSurvivesLiveModeChanges(t *testing.T) {
	h := newReloadHarness(t)
	engine := &stubEngine{running: true}
	h.m.engine = engine
	profile := RoutingListSpec{Tag: "global-profile", GlobalOnly: true}
	standalone := RoutingListSpec{Tag: "standalone"}
	h.m.SetRoutingLists([]RoutingListSpec{profile, standalone})
	h.m.SetRoutingOrder([]string{"direct", "block", "proxy"})
	personal := PersonalRouting{SmartDirectSites: []string{"smart.example"}, GlobalVPNSites: []string{"global.example"}}
	for i, mode := range []RoutingMode{ModeSmart, ModeGlobal, ModeSmart} {
		var update []PersonalRouting
		if i == 0 {
			update = []PersonalRouting{personal}
		}
		res := h.m.ReconnectWithRoutingRules(context.Background(), mode, nil, nil, nil, update...)
		if !res.Success {
			t.Fatalf("%s reload failed: %+v", mode, res)
		}
		cfg := engine.startCalls[len(engine.startCalls)-1]
		active := cfg.PersonalRouting
		if mode == ModeSmart {
			if !slices.Equal(active.SmartDirectSites, []string{"smart.example"}) || len(active.GlobalVPNSites) != 0 {
				t.Fatalf("incorrect Smart rules after reload: %+v", active)
			}
			if !slices.Equal(cfg.RoutingLists, []RoutingListSpec{standalone}) || len(cfg.RoutingOrder) != 0 {
				t.Fatal("global profile or its order reached the Smart engine")
			}
		} else if !slices.Equal(active.GlobalVPNSites, []string{"global.example"}) || len(active.SmartDirectSites) != 0 {
			t.Fatalf("incorrect Global rules after reload: %+v", active)
		} else if !slices.Equal(cfg.RoutingLists, []RoutingListSpec{profile, standalone}) || len(cfg.RoutingOrder) != 3 {
			t.Fatal("global profile or its order was lost after switching modes")
		}
	}
}
