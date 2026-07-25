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
			// The DoT (port 853) reject is DNS hardening, present regardless of
			// the AdBlock toggle — only ad-block (rule_set) rejects must be
			// absent here.
			if len(r.Port) == 1 && r.Port[0] == 853 {
				continue
			}
			t.Fatalf("did not expect a reject rule when AdBlock is off, rules=%+v", route.Rules)
		}
	}
	if len(route.RuleSet) != 0 {
		t.Fatalf("did not expect rule_set defs when AdBlock is off, got %+v", route.RuleSet)
	}
}

// TestBuildRoute_TunnelRejectsDoTPort853 pins Bug D: Android "Private DNS"
// (DoT, port 853) must be reject-routed so it can't stall on the direct
// outbound in Smart mode — Android then falls back to plaintext DNS, which is
// hijacked. The reject must sit after hijack-dns.
func TestBuildRoute_TunnelRejectsDoTPort853(t *testing.T) {
	route := buildRoute(EngineConfig{Mode: ProxyModeTunnel})
	hijackIdx, dotIdx := -1, -1
	for i, r := range route.Rules {
		if r.Action == "hijack-dns" {
			hijackIdx = i
		}
		if r.Action == "reject" && len(r.Port) == 1 && r.Port[0] == 853 {
			dotIdx = i
		}
	}
	if dotIdx == -1 {
		t.Fatalf("expected a reject rule for DoT port 853, rules=%+v", route.Rules)
	}
	if hijackIdx == -1 || dotIdx < hijackIdx {
		t.Fatalf("DoT reject (idx=%d) must come after hijack-dns (idx=%d)", dotIdx, hijackIdx)
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

func TestBuildAdBlockRuleSets_LocalWhenCachedElseOmitted(t *testing.T) {
	dir := t.TempDir()

	// No cached files yet → nothing is emitted (a remote rule_set would be
	// fatal on a cold start; the SRS is warmed out-of-band and applied on
	// the next reload).
	if got := buildAdBlockRuleSets(dir); len(got) != 0 {
		t.Fatalf("expected no rule_sets with an empty cache, got %+v", got)
	}

	// A valid cached SRS flips that list to a local rule_set.
	sub := filepath.Join(dir, adBlockRuleSetsSubdir)
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, defaultAdBlockRuleSets[0].fileName), validSRSBytes(t), 0o600); err != nil {
		t.Fatal(err)
	}
	got := buildAdBlockRuleSets(dir)
	if len(got) != 1 {
		t.Fatalf("expected exactly the one cached rule_set, got %+v", got)
	}
	if got[0].Type != "local" || got[0].Path == "" {
		t.Fatalf("expected a local rule_set for the cached file, got %+v", got[0])
	}
}

func TestBuildAdBlockRuleSets_TruncatedCacheOmitted(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, adBlockRuleSetsSubdir)
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	// A short / half-written file must NOT be referenced as local (that would
	// fail sing-box startup) and must NOT emit a remote fallback either — it is
	// simply skipped this session.
	if err := os.WriteFile(filepath.Join(sub, defaultAdBlockRuleSets[0].fileName), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := buildAdBlockRuleSets(dir); len(got) != 0 {
		t.Fatalf("truncated cache must be omitted, got %+v", got)
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
			if s == ".doubleclick.net" {
				t.Fatalf("did not expect a YouTube ad-delivery reject rule when AdBlock is off, rules=%+v", route.Rules)
			}
		}
	}
}

// Regression: ad/tracker SRS lists include Google's push endpoints, so with
// ad-block on the reject rule_set killed FCM/GCM — observed on device as
//   found package name: com.google.android.gms
//   sniffed: tls, domain: mtalk.google.com
//   outbound/block[block]: blocked connection to 142.251.127.188:5228
// which silently breaks notifications in EVERY app. These hosts must bypass
// the reject the same way the captive-portal / DoT hosts already do.
func TestAdBlockBypassesGooglePush(t *testing.T) {
	want := []string{
		"mtalk.google.com",
		"mtalk4.google.com",
		"alt1-mtalk.google.com",
		"alt8-mtalk.google.com",
		"android.googleapis.com",
		"fcm.googleapis.com",
	}
	for _, w := range want {
		found := false
		for _, d := range adBlockConnectivityBypassDomains {
			if d == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("push host %q missing from the ad-block bypass list", w)
		}
	}
}
