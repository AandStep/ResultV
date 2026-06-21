// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		m[s]--
	}
	for _, v := range m {
		if v != 0 {
			return false
		}
	}
	return true
}

func TestBuildRoute_AdBlock_AddsRejectAfterSniffAndDefinesRuleSets(t *testing.T) {
	cfg := EngineConfig{Mode: ProxyModeTunnel, AdBlock: true, DataDir: t.TempDir()}
	route := buildRoute(cfg)
	if route == nil {
		t.Fatal("expected non-nil route")
	}

	if len(route.RuleSet) != len(defaultAdBlockRuleSets) {
		t.Fatalf("expected %d rule_set defs, got %d (%+v)", len(defaultAdBlockRuleSets), len(route.RuleSet), route.RuleSet)
	}

	sniffIdx, rejectIdx := -1, -1
	for i, r := range route.Rules {
		if r.Action == "sniff" && sniffIdx == -1 {
			sniffIdx = i
		}
		if r.Action == "reject" && sameStringSet(r.RuleSet, adBlockRuleSetTags()) {
			rejectIdx = i
		}
	}
	if rejectIdx == -1 {
		t.Fatalf("expected ad-block reject rule referencing %v, rules=%+v", adBlockRuleSetTags(), route.Rules)
	}
	if sniffIdx == -1 || rejectIdx < sniffIdx {
		t.Fatalf("reject (idx=%d) must come after sniff (idx=%d) so the domain matcher is populated", rejectIdx, sniffIdx)
	}
}

func TestBuildRoute_AdBlockOff_NoRejectRules(t *testing.T) {
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel})
	for _, r := range route.Rules {
		if r.Action == "reject" {
			t.Fatalf("did not expect a reject rule when AdBlock is off, rules=%+v", route.Rules)
		}
	}
	if len(route.RuleSet) != 0 {
		t.Fatalf("did not expect rule_set defs when AdBlock is off, got %+v", route.RuleSet)
	}
}

func TestBuildDNS_AdBlock_AddsRejectRule(t *testing.T) {
	cfg := EngineConfig{Mode: ProxyModeTunnel, AdBlock: true, Proxy: ProxyConfig{Type: "vless"}}
	dns := buildDNS(cfg)
	if dns == nil {
		t.Fatal("expected non-nil dns")
	}
	found := false
	for _, r := range dns.Rules {
		if r.Action == "reject" && sameStringSet(r.RuleSet, adBlockRuleSetTags()) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a DNS reject rule referencing the ad rule_sets, rules=%+v", dns.Rules)
	}
}

func TestBuildDNS_AdBlock_BypassesConnectivityDomainsBeforeReject(t *testing.T) {
	cfg := EngineConfig{Mode: ProxyModeTunnel, AdBlock: true, Proxy: ProxyConfig{Type: "vless"}}
	dns := buildDNS(cfg)
	if dns == nil {
		t.Fatal("expected non-nil dns")
	}
	rejectIdx, bypassIdx := -1, -1
	for i, r := range dns.Rules {
		if r.Action == "reject" && sameStringSet(r.RuleSet, adBlockRuleSetTags()) {
			rejectIdx = i
		}
		if len(r.Domain) > 0 && r.Domain[0] == adBlockConnectivityBypassDomains[0] && r.Server != "" {
			bypassIdx = i
		}
	}
	if rejectIdx == -1 {
		t.Fatal("expected ad-block DNS reject rule")
	}
	if bypassIdx == -1 {
		t.Fatalf("expected connectivity bypass DNS rule before reject, rules=%+v", dns.Rules)
	}
	if bypassIdx >= rejectIdx {
		t.Fatalf("connectivity bypass (idx=%d) must precede reject (idx=%d)", bypassIdx, rejectIdx)
	}
}

func TestBuildRoute_AdBlock_BypassesConnectivityDomainsBeforeReject(t *testing.T) {
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel, AdBlock: true})
	rejectIdx, bypassIdx := -1, -1
	for i, r := range route.Rules {
		if r.Action == "reject" && sameStringSet(r.RuleSet, adBlockRuleSetTags()) {
			rejectIdx = i
		}
		if len(r.Domain) > 0 && r.Domain[0] == adBlockConnectivityBypassDomains[0] && r.Outbound == "direct" {
			bypassIdx = i
		}
	}
	if rejectIdx == -1 {
		t.Fatal("expected ad-block route reject rule")
	}
	if bypassIdx == -1 {
		t.Fatalf("expected connectivity bypass route rule, rules=%+v", route.Rules)
	}
	if bypassIdx >= rejectIdx {
		t.Fatalf("connectivity bypass (idx=%d) must precede reject (idx=%d)", bypassIdx, rejectIdx)
	}
}

func TestBuildAdBlockRuleSets_LocalWhenCachedElseRemoteViaDirect(t *testing.T) {
	dir := t.TempDir()

	// No cached files yet → every list is a remote rule_set fetched via direct.
	for _, rs := range buildAdBlockRuleSets(dir) {
		if rs.Type != "remote" {
			t.Fatalf("expected remote rule_set with no cache, got %q", rs.Type)
		}
		if rs.DownloadDetour != "direct" {
			t.Fatalf("remote rule_set must download via direct (pre-tunnel), got %q", rs.DownloadDetour)
		}
		if rs.Format != "binary" {
			t.Fatalf("expected binary SRS format, got %q", rs.Format)
		}
	}

	// A cached SRS of sufficient size flips that list to a local rule_set.
	sub := filepath.Join(dir, adBlockRuleSetsSubdir)
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	blob := make([]byte, minLocalSRSBytes+1)
	if err := os.WriteFile(filepath.Join(sub, defaultAdBlockRuleSets[0].fileName), blob, 0o600); err != nil {
		t.Fatal(err)
	}
	got := buildAdBlockRuleSets(dir)
	if got[0].Type != "local" || got[0].Path == "" {
		t.Fatalf("expected a local rule_set for the cached file, got %+v", got[0])
	}
}

func TestBuildAdBlockRuleSets_TruncatedCacheStaysRemote(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, adBlockRuleSetsSubdir)
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	// A short / half-written file must NOT be referenced as local — that would
	// fail sing-box startup and break the connection.
	if err := os.WriteFile(filepath.Join(sub, defaultAdBlockRuleSets[0].fileName), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := buildAdBlockRuleSets(dir); got[0].Type != "remote" {
		t.Fatalf("truncated cache must fall back to remote, got %+v", got[0])
	}
}

func TestBuildRoute_SmartModeEmptyList_KeepsGlobalFinal(t *testing.T) {
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel, SmartMode: true})
	if route.Final != "proxy" {
		t.Fatalf("empty smart list should fall back to Global final=proxy, got %q", route.Final)
	}
}

func TestBuildRoute_SmartModeWithList_UsesDirectFinal(t *testing.T) {
	route := buildRoute(EngineConfig{
		Mode:                ProxyModeTunnel,
		SmartMode:           true,
		SmartBlockedDomains: []string{"instagram.com"},
	})
	if route.Final != "direct" {
		t.Fatalf("smart with list should use final=direct, got %q", route.Final)
	}
}

func TestBuildRoute_YouTubeOff_NoYouTubeRules(t *testing.T) {
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel})
	for _, r := range route.Rules {
		for _, s := range r.DomainSuffix {
			if s == ".googlevideo.com" {
				t.Fatalf("did not expect a YouTube split rule when off, rules=%+v", route.Rules)
			}
		}
	}
}
