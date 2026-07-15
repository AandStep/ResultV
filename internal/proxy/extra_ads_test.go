// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

// pubserv.pro (psb-dsp.pubserv.pro) was caught live serving video pre-roll
// ads and is absent from every connected public list (AdGuard, EasyList,
// RU AdList, both reject SRS sets) — so it must be covered by the built-in
// supplement in the plain (non-RuleSet) reject rules.

func TestBuildRoute_AdBlock_RejectsExtraAdDomains(t *testing.T) {
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel, AdBlock: true})
	found := false
	for _, r := range route.Rules {
		if r.Action != "reject" || len(r.RuleSet) > 0 {
			continue
		}
		hasSuffix, hasExact := false, false
		for _, s := range r.DomainSuffix {
			if s == ".pubserv.pro" {
				hasSuffix = true
			}
		}
		for _, d := range r.Domain {
			if d == "pubserv.pro" {
				hasExact = true
			}
		}
		if hasSuffix && hasExact {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a plain reject rule covering pubserv.pro (exact + suffix), rules=%+v", route.Rules)
	}
}

func TestBuildDNS_AdBlock_RejectsExtraAdDomains(t *testing.T) {
	cfg := EngineConfig{Mode: ProxyModeTunnel, AdBlock: true, Proxy: ProxyConfig{Type: "vless"}}
	dns := buildDNS(cfg)
	if dns == nil {
		t.Fatal("expected non-nil dns")
	}
	found := false
	for _, r := range dns.Rules {
		if r.Action != "reject" || len(r.RuleSet) > 0 {
			continue
		}
		hasSuffix, hasExact := false, false
		for _, s := range r.DomainSuffix {
			if s == ".pubserv.pro" {
				hasSuffix = true
			}
		}
		for _, d := range r.Domain {
			if d == "pubserv.pro" {
				hasExact = true
			}
		}
		if hasSuffix && hasExact {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a plain DNS reject rule covering pubserv.pro (exact + suffix), rules=%+v", dns.Rules)
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
