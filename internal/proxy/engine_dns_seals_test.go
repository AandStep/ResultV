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
	"strings"
	"testing"
)

// sealConfigs are the configs the seals below are checked against: every shape
// this client emits, so a seal cannot be satisfied by the one mode that happens
// not to build the thing it guards.
func sealConfigs(t *testing.T) map[string]SingBoxConfig {
	t.Helper()
	node := ProxyConfig{IP: "203.0.113.7", Port: 443, Type: "vless"}
	return map[string]SingBoxConfig{
		"tunnel+global": mustBuildTunnelModeConfig(t, EngineConfig{
			Mode: ProxyModeTunnel, RoutingMode: ModeGlobal, Proxy: node, DataDir: t.TempDir(),
		}),
		"tunnel+smart": mustBuildTunnelModeConfig(t, EngineConfig{
			Mode: ProxyModeTunnel, RoutingMode: ModeSmart, Proxy: node, DataDir: t.TempDir(),
			BlockedDomains: []string{"example.org"},
		}),
		"tunnel+adaptive": mustBuildTunnelModeConfig(t, adaptiveTunnelConfig()),
		"proxy": mustBuildProxyModeConfig(t, EngineConfig{
			Mode: ProxyModeProxy, LocalPort: 24098, Proxy: node, DataDir: t.TempDir(),
		}),
	}
}

// route.default_domain_resolver stays unset, and that is a decision rather than
// an omission — which is why it is sealed.
//
// sing-box 1.14 deprecated dialing a domain-addressed server with no resolver
// named, and the migration note says to name one. Naming one here would send
// EVERY internal resolve straight to that transport: dns/router.go takes the
// options.Transport branch and never walks dns.rules at all. The direct
// outbound is the dial that makes this expensive — protocol/direct/outbound.go
// sets RemoteIsDomain unconditionally and carries no detour, so in Smart mode
// the rule walk is how a blocked domain still gets resolved through the tunnel
// instead of through the censored local resolver. One tag cannot express that.
//
// A wrapper server does not rescue it either: a "hosts" miss answers NXDOMAIN,
// and the fallback strategy counts NXDOMAIN as success (dns/transport/fallback/
// strategy.go), so a [server-pin, local] chain would stop at the first leg
// forever.
//
// What IS named is the node's own dial field — see serverDomainResolverTag.
// When the fork moves to a core where the fallback is an error rather than a
// warning, the answer is a patch or a different resolution path for direct, not
// a tag here.
func TestRouteNeverNamesADefaultDomainResolver(t *testing.T) {
	for name, cfg := range sealConfigs(t) {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(cfg.Route)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "default_domain_resolver") {
				t.Fatalf("route names a default domain resolver, which sends every internal resolve past dns.rules: %s", raw)
			}
		})
	}
}

// query_type is load-bearing in a way its one use does not show.
//
// On sing-box 1.14 an internal resolve is no longer exempt from DNS rules: a
// dial for a domain-addressed server reaches dns/router.go lookupWithRules,
// which builds real A and AAAA questions, so query_type matches there too. The
// fakeip catch-all survives that only because an internal lookup passes
// allowFakeIP=false and resolveDNSRoute skips a fakeip transport outright — the
// protection is the server's TYPE, not the rule's rarity.
//
// So any query_type rule pointing somewhere other than fakeip would silently
// start claiming internal dials as well. Worse, lookupWithRules fans A and AAAA
// out as two independent rule walks, so a rule scoped to one type would split a
// single dial's resolution across two different servers.
func TestQueryTypeIsOnlyEverUsedForTheFakeIPRule(t *testing.T) {
	for name, cfg := range sealConfigs(t) {
		t.Run(name, func(t *testing.T) {
			if cfg.DNS == nil {
				return
			}
			for i, rule := range cfg.DNS.Rules {
				if len(rule.QueryType) == 0 {
					continue
				}
				if rule.Server != fakeIPTag {
					t.Fatalf("dns rule[%d] scopes query_type %v to server %q; internal dials match query_type too, so this now claims them as well",
						i, rule.QueryType, rule.Server)
				}
			}
		})
	}
}

// The premise behind the seal below, checked against the real core rather than
// assumed: a dial field naming a server that does not exist is not ignored, it
// is a start failure. This also proves the field is live at all — a field the
// core merely tolerated would make the whole change a no-op.
func TestCoreRefusesADomainResolverNamingAnUnregisteredServer(t *testing.T) {
	cfg := mustBuildProxyModeConfig(t, EngineConfig{
		Mode:      ProxyModeProxy,
		LocalPort: 24098,
		Proxy:     ProxyConfig{IP: "node.example.com", Port: 443, Type: "vless"},
		DataDir:   t.TempDir(),
	})
	for i := range cfg.Outbounds {
		if cfg.Outbounds[i].Tag == "proxy" {
			cfg.Outbounds[i].DomainResolver = "no-such-server"
		}
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := coreBuildError(t, raw); err == nil {
		t.Fatal("the core accepted a domain_resolver naming an unregistered server — the field is not doing what the seal assumes")
	} else if !strings.Contains(err.Error(), "no-such-server") {
		t.Fatalf("the core failed for some other reason than the missing server: %v", err)
	}
}

// Every server a dial field names has to be a registered transport: sing-box
// resolves the tag while building the dialer and fails the whole start with
// "domain resolver not found" when it is missing. The tag is chosen in one
// place and the servers are built in another, so the two are checked against
// each other rather than trusted to agree.
func TestEveryNamedDomainResolverIsARegisteredServer(t *testing.T) {
	configs := sealConfigs(t)
	domainNode := ProxyConfig{IP: "node.example.com", Port: 443, Type: "vless", ResolvedIPs: []string{"203.0.113.7"}}
	configs["tunnel+domain node"] = mustBuildTunnelModeConfig(t, EngineConfig{
		Mode: ProxyModeTunnel, RoutingMode: ModeSmart, Proxy: domainNode, DataDir: t.TempDir(),
	})
	configs["proxy+domain node"] = mustBuildProxyModeConfig(t, EngineConfig{
		Mode: ProxyModeProxy, LocalPort: 24098, Proxy: domainNode, DataDir: t.TempDir(),
		DNSServers: []string{"dns.example.com"},
	})

	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			check := func(what, tag string) {
				if tag == "" {
					return
				}
				if !dnsServerExists(cfg.DNS, tag) {
					t.Fatalf("%s names domain_resolver %q, which no DNS server provides", what, tag)
				}
			}
			for _, out := range cfg.Outbounds {
				check("outbound "+out.Tag, out.DomainResolver)
			}
			for _, ep := range cfg.Endpoints {
				check("endpoint "+ep.Tag, ep.DomainResolver)
			}
			if cfg.DNS != nil {
				for _, srv := range cfg.DNS.Servers {
					check("dns server "+srv.Tag, srv.DomainResolver)
				}
			}
		})
	}
}
