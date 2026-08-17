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

// hasSmartRule reports whether dns carries the block-list DNS rule that
// buildDNS's Smart-mode branch adds. With dns.Final no longer set for either
// the active or inactive case (Fix 1), this rule's presence — not Final — is
// what actually distinguishes "the split applied" from "it did not".
func hasSmartRule(dns *SBDNS) bool {
	for _, r := range dns.Rules {
		for _, tag := range r.RuleSet {
			if tag == smartRuleSetTag {
				return true
			}
		}
	}
	return false
}

// TestSmartDNSDoesNotSetFinal pins the deliberate retreat in buildDNS: Smart
// mode used to default DNS to "local" so an unblocked lookup resolved from
// wherever the traffic actually left (the GeoDNS fix). That needs
// PlatformInterface.LocalDNSTransport, which this app does not implement —
// without it "local" falls back to sing-box's built-in transport, which
// targets 127.0.0.1:53 when /etc/resolv.conf is absent, breaking every
// lookup outside the block-list. So Final must stay unset, while the
// block-list rule (asserted in full by
// TestSmartDNSSendsBlockedDomainsThroughTheTunnel) must survive the removal.
func TestSmartDNSDoesNotSetFinal(t *testing.T) {
	dns := dnsForSmart(t, "VLESS")
	if dns.Final != "" {
		t.Fatalf("Smart mode must not redirect default DNS to the system resolver until LocalDNSTransport exists, got final=%q", dns.Final)
	}
	if !hasSmartRule(dns) {
		t.Fatal("the block-list DNS rule must survive the Final=local removal")
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

// WireGuard/AmneziaWG endpoints resolve through the tunnel like every other
// transport (see TestEndpointDNSGoesThroughTheTunnel), so the Smart split
// applies to them too: a blocked domain must not be answered by the local
// resolver just because the profile happens to be an endpoint one.
func TestSmartDNSSplitAppliesToWireGuardEndpoints(t *testing.T) {
	dns := dnsForSmart(t, "AMNEZIAWG")
	if !hasSmartRule(dns) {
		t.Fatal("endpoint transports must get the Smart DNS split rule; a censored domain resolved locally is a poisoned answer")
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
	if hasSmartRule(dns) {
		t.Fatal("without a usable SRS there is no tag to reference; the split rule must not be added")
	}
}
