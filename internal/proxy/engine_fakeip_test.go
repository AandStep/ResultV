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
	"regexp"
	"slices"
	"strings"
	"testing"
)

const adaptiveSelfExe = `C:\Program Files\ResultV\ResultV.exe`

func adaptiveTunnelConfig() EngineConfig {
	return EngineConfig{
		Mode:               ProxyModeTunnel,
		RoutingMode:        ModeSmart,
		AdaptiveSmart:      true,
		SelfExecutablePath: adaptiveSelfExe,
		Proxy:              ProxyConfig{IP: "203.0.113.7", Port: 443, Type: "vless"},
		DataDir:            `C:\Users\test\AppData\Roaming\ResultV`,
	}
}

// The app's own lookups must never be answered with a fake address: with one,
// the prober, the updater and the subscription fetch all get a successful
// answer pointing at 198.18.x.x and fail silently. The rule must also come
// FIRST — DNS rules are ordered.
//
// This asserts the rule is emitted and positioned, not that it protects
// anything: on Windows it is bypassed, because getaddrinfo resolves through
// svchost and the engine matches that instead of us. The real defence is
// isFakeIPAddr / realIPv4s at the point of use — see the rule's own comment in
// buildDNS.
func TestSelfExecutableIsExemptFromFakeIPAndComesFirst(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, adaptiveTunnelConfig())
	if cfg.DNS == nil || len(cfg.DNS.Rules) == 0 {
		t.Fatal("no DNS rules built")
	}
	first := cfg.DNS.Rules[0]
	if len(first.ProcessPathRegex) == 0 {
		t.Fatalf("first DNS rule is not the self-exemption: %+v", first)
	}
	if first.Server == fakeIPTag {
		t.Fatal("the app's own lookups were pointed at fakeip")
	}
	// Assert on what the rule DOES, not on how it is spelled: the regex is
	// QuoteMeta-escaped, so looking for the literal "ResultV.exe" inside it
	// would fail against a perfectly correct rule.
	var matched bool
	for _, raw := range first.ProcessPathRegex {
		rx, err := regexp.Compile(raw)
		if err != nil {
			t.Fatalf("self-exemption regex does not compile: %q: %v", raw, err)
		}
		if rx.MatchString(adaptiveSelfExe) {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("self-exemption does not match the executable: %v", first.ProcessPathRegex)
	}
}

func TestFakeIPServerAndCatchAllRule(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, adaptiveTunnelConfig())

	var found bool
	for _, srv := range cfg.DNS.Servers {
		if srv.Tag != fakeIPTag {
			continue
		}
		found = true
		if srv.Type != "fakeip" {
			t.Errorf("fakeip server has type %q", srv.Type)
		}
		if srv.Inet4Range != fakeIPInet4Range {
			t.Errorf("inet4_range = %q, want %q", srv.Inet4Range, fakeIPInet4Range)
		}
		if srv.Inet6Range != "" {
			t.Errorf("IPv6 is off in this config, inet6_range must stay empty, got %q", srv.Inet6Range)
		}
	}
	if !found {
		t.Fatal("no fakeip server emitted")
	}

	last := cfg.DNS.Rules[len(cfg.DNS.Rules)-1]
	if last.Server != fakeIPTag {
		t.Fatalf("the catch-all fakeip rule must come last, got %+v", last)
	}
	if len(last.QueryType) != 2 || last.QueryType[0] != "A" || last.QueryType[1] != "AAAA" {
		t.Fatalf("fakeip rule must be scoped to A/AAAA, got %v", last.QueryType)
	}
	// dns/transport_manager.go:217 rejects a fakeip default server outright.
	if cfg.DNS.Final == fakeIPTag {
		t.Fatal("dns.final must not be the fakeip server")
	}
	if cfg.DNS.Final == "" {
		t.Fatal("an empty dns.final makes the first registered transport the default")
	}
}

// Without store_fakeip the mapping dies on every in-place reload while clients
// still hold the addresses it handed out, and route.go:426 turns each of those
// connections into a fatal "missing fakeip record".
func TestFakeIPRequiresPersistentCacheFile(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, adaptiveTunnelConfig())
	if cfg.Experimental == nil || cfg.Experimental.CacheFile == nil {
		t.Fatal("fakeip was enabled without a cache_file")
	}
	cf := cfg.Experimental.CacheFile
	if !cf.Enabled || !cf.StoreFakeIP {
		t.Fatalf("cache_file must be enabled with store_fakeip: %+v", cf)
	}
	if cf.Path == "" {
		t.Fatal("cache_file needs an explicit path so it survives restarts")
	}
}

func TestFakeIPGetsIPv6RangeOnlyWhenTunCarriesIPv6(t *testing.T) {
	stubHostSupportsIPv6(t, true)
	cfg := adaptiveTunnelConfig()
	cfg.EnableIPv6 = true
	built := mustBuildTunnelModeConfig(t, cfg)
	for _, srv := range built.DNS.Servers {
		if srv.Tag == fakeIPTag && srv.Inet6Range != fakeIPInet6Range {
			t.Fatalf("inet6_range = %q, want %q when the TUN carries IPv6", srv.Inet6Range, fakeIPInet6Range)
		}
	}
}

// The whole rollback story: with the switch off the built config must be what
// it always was.
func TestSwitchOffEmitsNoFakeIP(t *testing.T) {
	cfg := adaptiveTunnelConfig()
	cfg.AdaptiveSmart = false
	built := mustBuildTunnelModeConfig(t, cfg)
	for _, srv := range built.DNS.Servers {
		if srv.Tag == fakeIPTag || srv.Type == "fakeip" {
			t.Fatal("fakeip leaked into a config with the switch off")
		}
	}
	for _, rule := range built.DNS.Rules {
		if rule.Server == fakeIPTag {
			t.Fatal("a fakeip DNS rule leaked into a config with the switch off")
		}
		if len(rule.ProcessPathRegex) > 0 {
			for _, raw := range rule.ProcessPathRegex {
				if rx, err := regexp.Compile(raw); err == nil && rx.MatchString(adaptiveSelfExe) {
					t.Fatal("the self-exemption rule leaked into a config with the switch off")
				}
			}
		}
	}
	if built.Experimental != nil && built.Experimental.CacheFile != nil && built.Experimental.CacheFile.StoreFakeIP {
		t.Fatal("store_fakeip leaked into a config with the switch off")
	}
}

// A connectivity probe exists to tell the truth about the network path, so it
// is the one kind of name a fake address must never be handed to.
//
// Windows asks for ipv6.msftconnecttest.com, a hostname that has AAAA records
// and no A record at all. FakeIP answers every A query regardless, so Windows
// opened a connection to 198.18.x.x, the router turned it back into the name,
// and the direct outbound resolved it for real under ipv4_only — producing
// "lookup ipv6.msftconnecttest.com: empty result" for a probe that had simply
// returned nothing before.
func TestConnectivityProbesAreExemptFromFakeIP(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, adaptiveTunnelConfig())

	probes := []string{
		"ipv6.msftconnecttest.com",
		"www.msftconnecttest.com",
		"dns.msftncsi.com",
	}
	for _, host := range probes {
		server, ok := dnsServerForHost(cfg.DNS, host)
		if !ok {
			t.Fatalf("%s matched no DNS rule at all", host)
		}
		if server == fakeIPTag {
			t.Errorf("%s was answered from the fake pool", host)
		}
	}

	// The exemption must be narrow: an ordinary name still has to reach fakeip,
	// or the feature has been switched off by accident.
	if server, _ := dnsServerForHost(cfg.DNS, "example.com"); server != fakeIPTag {
		t.Fatalf("an ordinary name went to %q instead of fakeip", server)
	}
}

// dnsServerForHost walks the built DNS rules the way sing-box does — first
// match wins — and reports which server would answer an A query for host from
// a process the config knows nothing about.
func dnsServerForHost(dns *SBDNS, host string) (string, bool) {
	for _, rule := range dns.Rules {
		// Rules keyed on the asking process do not apply to an arbitrary app.
		if len(rule.ProcessPathRegex) > 0 || len(rule.RuleSet) > 0 {
			continue
		}
		if len(rule.QueryType) > 0 && !slices.Contains(rule.QueryType, "A") {
			continue
		}
		if len(rule.Domain) == 0 && len(rule.DomainSuffix) == 0 {
			return rule.Server, true
		}
		if slices.Contains(rule.Domain, host) {
			return rule.Server, true
		}
		for _, suffix := range rule.DomainSuffix {
			if host == suffix || strings.HasSuffix(host, "."+strings.TrimPrefix(suffix, ".")) {
				return rule.Server, true
			}
		}
	}
	if dns.Final != "" {
		return dns.Final, true
	}
	return "", false
}

// The core is the final judge of whether this config is legal at all.
func TestCoreAcceptsAdaptiveConfig(t *testing.T) {
	assertCoreAcceptsConfig(t, mustBuildTunnelModeConfig(t, adaptiveTunnelConfig()))
}

// A WireGuard node has no "proxy" outbound to compare against — buildOutbounds
// emits direct+block for it, which is why adaptiveSmartActive skips it — so
// nothing ever asks the store what it learned. FakeIP there is all cost and no
// benefit: every name goes through the fake pool, game launchers see 198.18.x.x
// and names with no A record turn into dead connections, while the verdict the
// pool exists to feed is never consulted.
func TestWireGuardNodeGetsNoFakeIPEvenWithTheSwitchOn(t *testing.T) {
	for _, proto := range []string{"wireguard", "amneziawg"} {
		t.Run(proto, func(t *testing.T) {
			cfg := adaptiveTunnelConfig()
			cfg.Proxy = ProxyConfig{Type: proto, IP: "203.0.113.7", Port: 51820}
			built := mustBuildTunnelModeConfig(t, cfg)

			for _, srv := range built.DNS.Servers {
				if srv.Tag == fakeIPTag || srv.Type == "fakeip" {
					t.Fatal("fakeip was emitted for a node with no proxy outbound to compare against")
				}
			}
			for _, rule := range built.DNS.Rules {
				if rule.Server == fakeIPTag {
					t.Fatal("a fakeip DNS rule was emitted for a WireGuard node")
				}
			}
			if built.Experimental != nil && built.Experimental.CacheFile != nil &&
				built.Experimental.CacheFile.StoreFakeIP {
				t.Fatal("store_fakeip was turned on for a config with no fakeip server")
			}
		})
	}
}

// FakeIP hands the router a NAME where there used to be an address, so the
// direct outbound has to resolve it at dial time. That lookup walks the DNS
// rules, where the fakeip catch-all is waiting for it — and a fakeip transport
// cannot answer an internal lookup, so the dial sat for the full deadline and
// died. Measured on a live engine 2026-09-22:
//
//	router: found fakeip domain: example.com
//	outbound/direct[direct]: outbound connection to example.com:80
//	dns: match[3] query_type=[A AAAA] => route(fakeip)
//	dns: lookup failed for example.com: context deadline exceeded   (10.0s)
//
// Naming a resolver on the outbound takes dial-time resolution off the rule
// walk entirely, which is the same move serverDomainResolverTag already makes
// for the node's own address.
func TestDirectOutboundResolvesOutsideTheRulesUnderFakeIP(t *testing.T) {
	cfg := mustBuildTunnelModeConfig(t, adaptiveTunnelConfig())
	direct := mustOutbound(t, cfg, "direct")
	if direct.DomainResolver == "" {
		t.Fatal("у direct нет domain_resolver: резолв уйдёт в обход правил и попадёт в fakeip-ловушку")
	}
	if direct.DomainResolver == fakeIPTag {
		t.Fatalf("direct резолвит через fakeip: %s", direct.DomainResolver)
	}
	if !dnsServerExists(cfg.DNS, direct.DomainResolver) {
		t.Fatalf("direct ссылается на несуществующий DNS-сервер %q", direct.DomainResolver)
	}
}

// Without FakeIP a destination reaches direct as an address, so there is
// nothing to resolve and nothing to change.
func TestDirectOutboundKeepsItsOldShapeWithoutFakeIP(t *testing.T) {
	plain := adaptiveTunnelConfig()
	plain.AdaptiveSmart = false
	cfg := mustBuildTunnelModeConfig(t, plain)
	if direct := mustOutbound(t, cfg, "direct"); direct.DomainResolver != "" {
		t.Fatalf("direct получил domain_resolver без FakeIP: %s", direct.DomainResolver)
	}
}

func mustOutbound(t *testing.T, cfg SingBoxConfig, tag string) SBOutbound {
	t.Helper()
	out, ok := outboundByTag(cfg, tag)
	if !ok {
		t.Fatalf("аутбаунд %q не найден", tag)
	}
	return out
}
