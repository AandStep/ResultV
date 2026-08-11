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
	"testing"
)

// dnsForSmart builds a tunnel-mode DNS block for a Smart config backed by a
// real compiled SRS in a temp dir — localSmartSRSUsable parses the file, so a
// stub path would just disable the whole branch under test.
func dnsForSmart(t *testing.T, proxyType string) *SBDNS {
	t.Helper()
	dir := t.TempDir()
	if err := CompileSmartSRS([]string{"x.com", "instagram.com"}, SmartSRSPath(dir)); err != nil {
		t.Fatalf("CompileSmartSRS: %v", err)
	}
	return buildDNS(EngineConfig{
		Mode:                ProxyModeTunnel,
		Proxy:               ProxyConfig{IP: "203.0.113.9", Port: 443, Type: proxyType},
		DataDir:             dir,
		IsAndroid:           true,
		SmartMode:           true,
		SmartBlockedDomains: []string{"x.com", "instagram.com"},
	})
}

func TestSmartDNSFinalIsLocal(t *testing.T) {
	dns := dnsForSmart(t, "VLESS")
	if dns.Final != "local" {
		t.Fatalf("Smart mode must default DNS to the system resolver, got final=%q", dns.Final)
	}
}

func TestSmartDNSSendsBlockedDomainsThroughTheTunnel(t *testing.T) {
	dns := dnsForSmart(t, "VLESS")
	var found bool
	for _, r := range dns.Rules {
		for _, tag := range r.RuleSet {
			if tag != smartRuleSetTag {
				continue
			}
			found = true
			if r.Server == "" || r.Server == "local" {
				t.Fatalf("blocked domains must resolve through the tunnel, got server=%q", r.Server)
			}
		}
	}
	if !found {
		t.Fatal("no DNS rule referencing the Smart rule-set")
	}
}

// A DNS rule pointing at a rule_set the route never registered fails the whole
// start, so the two sides must agree on exactly one condition.
func TestSmartDNSRuleSetTagIsRegisteredByRoute(t *testing.T) {
	dir := t.TempDir()
	if err := CompileSmartSRS([]string{"x.com"}, SmartSRSPath(dir)); err != nil {
		t.Fatalf("CompileSmartSRS: %v", err)
	}
	cfg := EngineConfig{
		Mode:                ProxyModeTunnel,
		Proxy:               ProxyConfig{IP: "203.0.113.9", Port: 443, Type: "VLESS"},
		DataDir:             dir,
		IsAndroid:           true,
		SmartMode:           true,
		SmartBlockedDomains: []string{"x.com"},
	}
	route := buildRoute(cfg)
	registered := map[string]bool{}
	for _, rs := range route.RuleSet {
		registered[rs.Tag] = true
	}
	for _, r := range buildDNS(cfg).Rules {
		for _, tag := range r.RuleSet {
			if !registered[tag] {
				t.Fatalf("DNS references unregistered rule_set %q", tag)
			}
		}
	}
}

// Global mode has no split to mirror: every lookup must keep going through the
// tunnel, or a censored domain gets a poisoned local answer.
func TestGlobalDNSStaysOnTheTunnel(t *testing.T) {
	dns := buildDNS(EngineConfig{
		Mode:      ProxyModeTunnel,
		Proxy:     ProxyConfig{IP: "203.0.113.9", Port: 443, Type: "VLESS"},
		DataDir:   t.TempDir(),
		IsAndroid: true,
	})
	if dns.Final != "" {
		t.Fatalf("Global mode must not redirect DNS to the system resolver, got final=%q", dns.Final)
	}
}

// WireGuard/AmneziaWG endpoints run with detour="" — their lookups are already
// routed like ordinary traffic, so the split must not be applied twice.
func TestSmartDNSLeavesWireGuardEndpointsAlone(t *testing.T) {
	dns := dnsForSmart(t, "AMNEZIAWG")
	if dns.Final != "" {
		t.Fatalf("endpoint transports must keep their existing DNS behaviour, got final=%q", dns.Final)
	}
}

func TestSmartDNSInactiveWithoutCompiledRuleSet(t *testing.T) {
	dns := buildDNS(EngineConfig{
		Mode:                ProxyModeTunnel,
		Proxy:               ProxyConfig{IP: "203.0.113.9", Port: 443, Type: "VLESS"},
		DataDir:             t.TempDir(), // no SRS compiled
		IsAndroid:           true,
		SmartMode:           true,
		SmartBlockedDomains: []string{"x.com"},
	})
	if dns.Final != "" {
		t.Fatalf("without a usable SRS there is no tag to reference; got final=%q", dns.Final)
	}
}
