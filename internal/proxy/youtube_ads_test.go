// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

func TestBuildDNS_AdBlock_BypassesGooglevideoBeforeReject(t *testing.T) {
	cfg := EngineConfig{Mode: ProxyModeTunnel, AdBlock: true, Proxy: ProxyConfig{Type: "vless"}}
	dns := buildDNS(cfg)
	rejectIdx, gvBypassIdx := -1, -1
	for i, r := range dns.Rules {
		if r.Action == "reject" {
			rejectIdx = i
		}
		for _, sfx := range r.DomainSuffix {
			if sfx == ".googlevideo.com" && r.Server != "" {
				gvBypassIdx = i
			}
		}
	}
	if rejectIdx == -1 || gvBypassIdx == -1 || gvBypassIdx >= rejectIdx {
		t.Fatalf("expected googlevideo DNS bypass before reject, rules=%+v", dns.Rules)
	}
}

func TestBuildRoute_AdBlock_RejectsYouTubeAdDeliveryDomains(t *testing.T) {
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel, AdBlock: true})
	sniffIdx, bypassIdx, rejectIdx := -1, -1, -1
	for i, r := range route.Rules {
		if r.Action == "sniff" && sniffIdx == -1 {
			sniffIdx = i
		}
		if len(r.Domain) > 0 && r.Domain[0] == adBlockConnectivityBypassDomains[0] && r.Outbound == "direct" {
			bypassIdx = i
		}
		if r.Action == "reject" && len(r.RuleSet) == 0 {
			for _, s := range r.DomainSuffix {
				if s == ".doubleclick.net" {
					rejectIdx = i
				}
			}
		}
	}
	if rejectIdx == -1 {
		t.Fatalf("expected a plain reject rule (no RuleSet) covering .doubleclick.net, rules=%+v", route.Rules)
	}
	if sniffIdx == -1 || rejectIdx < sniffIdx {
		t.Fatalf("YouTube ad reject (idx=%d) must come after sniff (idx=%d)", rejectIdx, sniffIdx)
	}
	if bypassIdx == -1 || rejectIdx < bypassIdx {
		t.Fatalf("YouTube ad reject (idx=%d) must come after connectivity bypass (idx=%d)", rejectIdx, bypassIdx)
	}

	var has2mdn, hasDoubleclickCom, hasDoubleclickDe bool
	for _, s := range youTubeAdDeliverySuffixes {
		switch s {
		case ".2mdn.net":
			has2mdn = true
		case ".doubleclick.com":
			hasDoubleclickCom = true
		case ".doubleclick.de":
			hasDoubleclickDe = true
		}
	}
	if !has2mdn || !hasDoubleclickCom || !hasDoubleclickDe {
		t.Fatalf("expected curated list to include .2mdn.net, .doubleclick.com, .doubleclick.de, got %+v", youTubeAdDeliverySuffixes)
	}
}

func TestBuildDNS_AdBlock_RejectsYouTubeAdDeliveryDomains(t *testing.T) {
	cfg := EngineConfig{Mode: ProxyModeTunnel, AdBlock: true, Proxy: ProxyConfig{Type: "vless"}}
	dns := buildDNS(cfg)
	if dns == nil {
		t.Fatal("expected non-nil dns")
	}
	found := false
	for _, r := range dns.Rules {
		if r.Action == "reject" && len(r.RuleSet) == 0 {
			for _, s := range r.DomainSuffix {
				if s == ".doubleclick.net" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("expected a plain DNS reject rule (no RuleSet) covering .doubleclick.net, rules=%+v", dns.Rules)
	}
}
