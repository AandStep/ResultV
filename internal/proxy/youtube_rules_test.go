// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import "testing"

func TestBuildRoute_YouTubeUnblock_GeoSplitAndRejectsAds(t *testing.T) {
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel, YouTubeUnblock: true})
	var videoProxy, playerDirect, adReject bool
	var videoIdx, playerIdx, adIdx = -1, -1, -1
	for i, r := range route.Rules {
		for _, s := range r.DomainSuffix {
			if s == ".googlevideo.com" && r.Outbound == "proxy" {
				videoProxy = true
				videoIdx = i
			}
		}
		for _, d := range r.Domain {
			if d == "youtubei.googleapis.com" && r.Outbound == "direct" {
				playerDirect = true
				playerIdx = i
			}
		}
		if r.Action == "reject" {
			for _, s := range r.DomainSuffix {
				if s == ".doubleclick.net" {
					adReject = true
					adIdx = i
				}
			}
		}
	}
	if !videoProxy {
		t.Fatalf("expected *.googlevideo.com → proxy, rules=%+v", route.Rules)
	}
	if !playerDirect {
		t.Fatalf("expected youtubei.googleapis.com → direct, rules=%+v", route.Rules)
	}
	if !adReject {
		t.Fatalf("expected ad-delivery reject rule, rules=%+v", route.Rules)
	}
	if videoIdx >= playerIdx || playerIdx >= adIdx {
		t.Fatalf("expected order video-proxy (%d) < player-direct (%d) < ad-reject (%d)", videoIdx, playerIdx, adIdx)
	}
}

func TestBuildRoute_YouTubeUnblock_AutoEnablesSRSReject(t *testing.T) {
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel, YouTubeUnblock: true})
	var srsReject bool
	for _, r := range route.Rules {
		if r.Action == "reject" && sameStringSet(r.RuleSet, adBlockRuleSetTags()) {
			srsReject = true
		}
	}
	if !srsReject {
		t.Fatalf("expected SRS ad-block reject when YouTubeUnblock alone, rules=%+v", route.Rules)
	}
	if len(route.RuleSet) == 0 {
		t.Fatal("expected ad-block rule_sets when YouTubeUnblock alone")
	}
}

func TestBuildRoute_YouTubeUnblockWithAdBlock_RejectsAdsBeforeGlobalAdblock(t *testing.T) {
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel, YouTubeUnblock: true, AdBlock: true})
	rejectIdx, ytAdIdx := -1, -1
	for i, r := range route.Rules {
		if r.Action == "reject" && sameStringSet(r.RuleSet, adBlockRuleSetTags()) {
			rejectIdx = i
		}
		for _, s := range r.DomainSuffix {
			if s == ".doubleclick.net" && r.Action == "reject" && len(r.RuleSet) == 0 {
				ytAdIdx = i
			}
		}
	}
	if rejectIdx == -1 {
		t.Fatal("expected global ad-block reject rule")
	}
	if ytAdIdx == -1 {
		t.Fatalf("expected YouTube ad-delivery reject, rules=%+v", route.Rules)
	}
	if ytAdIdx >= rejectIdx {
		t.Fatalf("YouTube ad reject (idx=%d) must precede global reject (idx=%d)", ytAdIdx, rejectIdx)
	}
}

func TestBuildDNS_YouTubeUnblock_AutoEnablesSRSReject(t *testing.T) {
	dns := buildDNS(EngineConfig{Mode: ProxyModeTunnel, YouTubeUnblock: true, Proxy: ProxyConfig{Type: "vless"}})
	found := false
	for _, r := range dns.Rules {
		if r.Action == "reject" && sameStringSet(r.RuleSet, adBlockRuleSetTags()) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected SRS DNS reject when YouTubeUnblock alone, rules=%+v", dns.Rules)
	}
}

func TestBuildDNS_YouTubeUnblockWithAdBlock_BypassesCoreBeforeReject(t *testing.T) {
	cfg := EngineConfig{Mode: ProxyModeTunnel, YouTubeUnblock: true, AdBlock: true, Proxy: ProxyConfig{Type: "vless"}}
	dns := buildDNS(cfg)
	if dns == nil {
		t.Fatal("expected non-nil dns")
	}
	rejectIdx, ytBypassIdx := -1, -1
	for i, r := range dns.Rules {
		if r.Action == "reject" && sameStringSet(r.RuleSet, adBlockRuleSetTags()) {
			rejectIdx = i
		}
		for _, sfx := range r.DomainSuffix {
			if sfx == ".googlevideo.com" && r.Server != "" {
				ytBypassIdx = i
			}
		}
	}
	if rejectIdx == -1 {
		t.Fatal("expected ad-block DNS reject rule")
	}
	if ytBypassIdx == -1 {
		t.Fatalf("expected YouTube core DNS bypass before reject, rules=%+v", dns.Rules)
	}
	if ytBypassIdx >= rejectIdx {
		t.Fatalf("YouTube DNS bypass (idx=%d) must precede reject (idx=%d)", ytBypassIdx, rejectIdx)
	}
}

func TestBuildDNS_AdBlock_BypassesGooglevideoWithoutYouTubeUnblock(t *testing.T) {
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
