// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"testing"

	"github.com/sagernet/sing/common/domain"
)

// smartProxyRule extracts the Smart-mode "listed domains → proxy" rule from a
// built route. Fails the test if it is absent.
func smartProxyRule(t *testing.T, cfg EngineConfig) SBRouteRule {
	t.Helper()
	route := buildRoute(cfg)
	for _, r := range route.Rules {
		if r.Outbound == "proxy" && (len(r.Domain) > 0 || len(r.DomainSuffix) > 0) {
			return r
		}
	}
	t.Fatalf("no smart proxy rule found, rules=%+v", route.Rules)
	return SBRouteRule{}
}

// TestBuildRoute_SmartMode_MatchesBareDomain pins the real sing-box matcher
// semantics: a domain_suffix entry with a leading dot (".x.com") matches ONLY
// subdomains, never the bare host. Smart-mode users type "x.com" /
// "instagram.com" with a bare SNI, so the generated rule MUST match the bare
// domain itself — otherwise those hosts fall through to final=direct and die
// behind RKN blocks (the original bug).
func TestBuildRoute_SmartMode_MatchesBareDomain(t *testing.T) {
	rule := smartProxyRule(t, EngineConfig{
		Mode:                ProxyModeTunnel,
		SmartMode:           true,
		SmartBlockedDomains: []string{"x.com", "instagram.com", "youtube.com"},
	})
	m := domain.NewMatcher(rule.Domain, rule.DomainSuffix, false)

	for _, host := range []string{
		"x.com", "instagram.com", "youtube.com", // bare SNI (the bug)
		"www.instagram.com", "i.instagram.com", "www.youtube.com", // subdomains
	} {
		if !m.Match(host) {
			t.Errorf("smart rule must match %q, rule=%+v", host, rule)
		}
	}
	// Label-boundary safety: unrelated hosts sharing a string suffix must NOT
	// be pulled into the proxy.
	for _, host := range []string{"fakeinstagram.com", "notx.com"} {
		if m.Match(host) {
			t.Errorf("smart rule must NOT match %q, rule=%+v", host, rule)
		}
	}
}

// TestSplitSmartDomains_IPsStayExact keeps the pure-IP handling: IP literals
// from RU lists go to the exact bucket verbatim.
func TestSplitSmartDomains_IPsStayExact(t *testing.T) {
	exact, suffix := splitSmartDomains([]string{"1.2.3.4", "instagram.com", "instagram.com", ""})
	if len(exact) != 1 || exact[0] != "1.2.3.4" {
		t.Fatalf("expected exact=[1.2.3.4], got %v", exact)
	}
	if len(suffix) != 1 {
		t.Fatalf("expected one suffix entry (deduped), got %v", suffix)
	}
}
