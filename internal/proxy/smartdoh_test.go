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
	"slices"
	"strings"
	"testing"
)

// dohRejectIndex returns the position of the DoH reject rule, or -1. It is the
// only reject rule that selects on domains alone, with no network or port of
// its own — the QUIC rejects all carry udp/443.
func dohRejectIndex(rules []SBRouteRule) int {
	for i, rule := range rules {
		if rule.Action == "reject" && len(rule.DomainSuffix) > 0 &&
			len(rule.Network) == 0 && len(rule.Port) == 0 {
			return i
		}
	}
	return -1
}

func dohTunnelConfig() EngineConfig {
	cfg := adaptiveTunnelConfig()
	cfg.AdaptiveSmartBlockBrowserDoH = true
	return cfg
}

// A browser with Secure DNS on never asks the system resolver, so FakeIP never
// sees the name and the connection reaches the router as a bare address. The
// engine can still race it, but it then learns by address — and a CDN rotates
// addresses, so that knowledge is weaker and does not generalise to the domain.
func TestBrowserDoHIsRejectedUnderTheSubToggle(t *testing.T) {
	built := mustBuildTunnelModeConfig(t, dohTunnelConfig())
	idx := dohRejectIndex(built.Route.Rules)
	if idx < 0 {
		t.Fatal("the sub-toggle is on and nothing rejects DoH")
	}
	for _, host := range []string{"dns.google", "cloudflare-dns.com", "dns.quad9.net"} {
		if !slices.Contains(built.Route.Rules[idx].DomainSuffix, host) {
			t.Errorf("%s is not in the DoH reject rule: %v", host, built.Route.Rules[idx].DomainSuffix)
		}
	}
}

// The toggle is intrusive — it breaks users who turned Secure DNS on
// deliberately — so nothing may happen without it.
func TestNoDoHRejectWithoutTheSubToggle(t *testing.T) {
	built := mustBuildTunnelModeConfig(t, adaptiveTunnelConfig())
	if idx := dohRejectIndex(built.Route.Rules); idx >= 0 {
		t.Fatalf("DoH was rejected with the sub-toggle off: %+v", built.Route.Rules[idx])
	}
}

// The whole point is to push the browser back onto a resolver FakeIP can see.
// Without the adaptive engine there is no FakeIP, so the rule would cost the
// user their DoH and buy nothing.
func TestDoHRejectNeedsTheAdaptiveEngine(t *testing.T) {
	cfg := dohTunnelConfig()
	cfg.AdaptiveSmart = false
	built := mustBuildTunnelModeConfig(t, cfg)
	if idx := dohRejectIndex(built.Route.Rules); idx >= 0 {
		t.Fatalf("DoH was rejected with the adaptive engine off: %+v", built.Route.Rules[idx])
	}
}

// Ordering is the difference between working and not: the block-list rule
// routes its domains to the node, and a DoH endpoint that happens to be on the
// list would then resolve happily through the tunnel — the browser would keep
// bypassing the system resolver and FakeIP would still see nothing.
func TestDoHRejectComesBeforeTheBlockList(t *testing.T) {
	cfg := dohTunnelConfig()
	cfg.BlockedDomains = []string{"dns.google", "example.com"}
	built := mustBuildTunnelModeConfig(t, cfg)

	doh := dohRejectIndex(built.Route.Rules)
	if doh < 0 {
		t.Fatal("no DoH reject rule")
	}
	for i, rule := range built.Route.Rules {
		if rule.Action == "route" && rule.Outbound == "proxy" &&
			slices.Contains(rule.DomainSuffix, "dns.google") && i < doh {
			t.Fatalf("the block-list routes dns.google to the node at rule %d, before the reject at %d", i, doh)
		}
	}
}

// An excluded app is the user saying "leave this alone", and it must keep
// winning over a blanket rule of ours.
func TestExcludedAppKeepsItsDoH(t *testing.T) {
	cfg := dohTunnelConfig()
	cfg.AppWhitelist = []string{`C:\Program Files\Mozilla Firefox\firefox.exe`}
	built := mustBuildTunnelModeConfig(t, cfg)

	doh := dohRejectIndex(built.Route.Rules)
	if doh < 0 {
		t.Fatal("no DoH reject rule")
	}
	var whitelist = -1
	for i, rule := range built.Route.Rules {
		if rule.Action == "route" && rule.Outbound == "direct" && len(rule.ProcessPathRegex) > 0 {
			whitelist = i
			break
		}
	}
	if whitelist < 0 {
		t.Fatal("the app-exclusion rule disappeared")
	}
	if whitelist > doh {
		t.Fatalf("the DoH reject at %d wins over the user's app exclusion at %d", doh, whitelist)
	}
}

// Emitted as domain_suffix, so every entry matches its sub-domains too. An
// entry that is also a suffix of an ordinary site would knock that site off the
// network entirely — this rule rejects, it does not reroute.
func TestDoHEntriesDoNotSwallowOrdinarySites(t *testing.T) {
	ordinary := []string{
		"www.google.com", "google.com", "mail.google.com",
		"www.cloudflare.com", "cloudflare.com",
		"www.quad9.net", "quad9.net",
		"www.opendns.com", "nextdns.io", "adguard.com",
	}
	for _, entry := range browserDoHDomains() {
		if entry != normalizeRule(entry) {
			t.Errorf("DoH entry %q is not in normalized form (%q)", entry, normalizeRule(entry))
		}
		for _, host := range ordinary {
			if host == entry || strings.HasSuffix(host, "."+entry) {
				t.Errorf("DoH entry %q swallows the ordinary host %q", entry, host)
			}
		}
	}
}
