// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

// Hosts caught live serving ads while absent from every connected public
// list (AdGuard, EasyList, RU AdList, both reject SRS sets) — each must be
// covered by the built-in supplement in the plain (non-RuleSet) reject rules.
// pubserv.pro: video pre-rolls on RU streaming sites; the rest were caught
// 2026-07-16 on hot.noodlemagazine.com via the engine connection log
// (foxstreetcore native banners, ofjvnvjf.win popup slider iframe,
// betamountwo.com ad infra, adultmasters.pro banner network).
var supplementTestDomains = []string{
	"pubserv.pro",
	"foxstreetcore.com",
	"ofjvnvjf.win",
	"betamountwo.com",
	"adultmasters.pro",
	"nmsrv.run",
	"kintg.site",
}

// rejectRuleCovers reports whether any plain (non-RuleSet) reject rule in the
// list carries both the exact domain and its dot-suffix.
func rejectRuleCovers(rules []SBRouteRule, domain string) bool {
	for _, r := range rules {
		if r.Action != "reject" || len(r.RuleSet) > 0 {
			continue
		}
		hasSuffix, hasExact := false, false
		for _, s := range r.DomainSuffix {
			if s == "."+domain {
				hasSuffix = true
			}
		}
		for _, d := range r.Domain {
			if d == domain {
				hasExact = true
			}
		}
		if hasSuffix && hasExact {
			return true
		}
	}
	return false
}

func TestBuildRoute_AdBlock_RejectsExtraAdDomains(t *testing.T) {
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel, AdBlock: true})
	for _, domain := range supplementTestDomains {
		if !rejectRuleCovers(route.Rules, domain) {
			t.Errorf("expected a plain reject rule covering %s (exact + suffix)", domain)
		}
	}
}

func TestBuildDNS_AdBlock_RejectsExtraAdDomains(t *testing.T) {
	cfg := EngineConfig{Mode: ProxyModeTunnel, AdBlock: true, Proxy: ProxyConfig{Type: "vless"}}
	dns := buildDNS(cfg)
	if dns == nil {
		t.Fatal("expected non-nil dns")
	}
	// SBDNSRule and SBRouteRule are distinct structs; project the fields the
	// checker needs so both tests share it.
	asRouteRules := make([]SBRouteRule, len(dns.Rules))
	for i, r := range dns.Rules {
		asRouteRules[i] = SBRouteRule{
			Action:       r.Action,
			RuleSet:      r.RuleSet,
			Domain:       r.Domain,
			DomainSuffix: r.DomainSuffix,
		}
	}
	for _, domain := range supplementTestDomains {
		if !rejectRuleCovers(asRouteRules, domain) {
			t.Errorf("expected a plain DNS reject rule covering %s (exact + suffix)", domain)
		}
	}
}

// The extra reject must not regress the curated YouTube suffixes it is merged
// with — both sets ride the same rule.
func TestBuildRoute_AdBlock_ExtraDoesNotDropYouTubeSuffixes(t *testing.T) {
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel, AdBlock: true})
	for _, r := range route.Rules {
		if r.Action != "reject" || len(r.RuleSet) > 0 {
			continue
		}
		hasYT, hasExtra := false, false
		for _, s := range r.DomainSuffix {
			if s == ".doubleclick.net" {
				hasYT = true
			}
			if s == ".pubserv.pro" {
				hasExtra = true
			}
		}
		if hasExtra && !hasYT {
			t.Fatalf("extra ad suffixes must be merged with the YouTube set, not replace it: %+v", r.DomainSuffix)
		}
	}
}
