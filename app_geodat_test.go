// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

// plainProfile references no geo database, so every test below compiles
// without reaching the network.
func plainProfile() config.RoutingProfile {
	return config.RoutingProfile{
		Name:        "Мой профиль",
		DirectSites: []string{"gosuslugi.ru", "domain:nalog.ru"},
		DirectIPs:   []string{"10.0.0.0/8"},
		BlockSites:  []string{"ads.example"},
		Source:      "manual",
	}
}

func TestCompileRoutingProfileWritesRuleSets(t *testing.T) {
	dir := t.TempDir()
	a := newTestApp(t, dir)

	saved, err := a.SaveRoutingProfile(plainProfile())
	if err != nil {
		t.Fatalf("SaveRoutingProfile: %v", err)
	}
	// SaveRoutingProfile compiles on the way through; verify the files landed.
	for _, action := range []string{"direct", "block"} {
		path := proxy.RoutingListCachePath(dir, routingProfileRuleSetID(saved.ID, action))
		st, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s rule-set not written: %v", action, err)
		}
		if st.Size() == 0 {
			t.Fatalf("%s rule-set is empty", action)
		}
	}
	// An action the profile never uses must leave no file behind.
	if _, err := os.Stat(proxy.RoutingListCachePath(dir, routingProfileRuleSetID(saved.ID, "proxy"))); err == nil {
		t.Error("a rule-set was written for an action with no rules")
	}
}

func TestCompileRoutingProfileReportsCounts(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	saved, err := a.SaveRoutingProfile(plainProfile())
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.CompileRoutingProfile(saved.ID, false)
	if err != nil {
		t.Fatalf("CompileRoutingProfile: %v", err)
	}
	counts, _ := res["counts"].(map[string]any)
	if counts["direct"] != 3 { // two domains + one CIDR
		t.Errorf("direct = %v, want 3", counts["direct"])
	}
	if counts["block"] != 1 {
		t.Errorf("block = %v, want 1", counts["block"])
	}
	if counts["proxy"] != 0 {
		t.Errorf("proxy = %v, want 0", counts["proxy"])
	}
}

func TestActiveProfileSpecsAreEmptyInSmartMode(t *testing.T) {
	dir := t.TempDir()
	a := newTestApp(t, dir)

	saved, err := a.SaveRoutingProfile(plainProfile())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetActiveRoutingProfile(saved.ID); err != nil {
		t.Fatal(err)
	}

	rr := a.config.GetConfig().RoutingRules
	rr.Mode = "global"
	if err := a.config.UpdateRoutingRules(rr); err != nil {
		t.Fatal(err)
	}
	if got := len(a.buildRoutingProfileSpecs()); got != 2 {
		t.Fatalf("global mode: %d specs, want 2 (direct + block)", got)
	}

	// Smart works its own routing out; a profile there would pull against it.
	// The engine must enforce that, not just the UI hiding the button.
	rr = a.config.GetConfig().RoutingRules
	rr.Mode = "smart"
	if err := a.config.UpdateRoutingRules(rr); err != nil {
		t.Fatal(err)
	}
	if got := a.buildRoutingProfileSpecs(); len(got) != 0 {
		t.Fatalf("smart mode: %d specs, want none", len(got))
	}
	if got := a.activeRoutingOrder(); got != nil {
		t.Fatalf("smart mode: order = %v, want none", got)
	}
}

func TestProfileSpecsCarryTheRightActions(t *testing.T) {
	dir := t.TempDir()
	a := newTestApp(t, dir)
	saved, err := a.SaveRoutingProfile(plainProfile())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetActiveRoutingProfile(saved.ID); err != nil {
		t.Fatal(err)
	}
	rr := a.config.GetConfig().RoutingRules
	rr.Mode = "global"
	if err := a.config.UpdateRoutingRules(rr); err != nil {
		t.Fatal(err)
	}

	byAction := map[string]proxy.RoutingListSpec{}
	for _, s := range a.buildRoutingProfileSpecs() {
		byAction[s.Action] = s
	}
	if _, ok := byAction["direct"]; !ok {
		t.Error("no direct spec")
	}
	if _, ok := byAction["block"]; !ok {
		t.Error("no block spec")
	}
	for action, spec := range byAction {
		if spec.Tag == "" || spec.Path == "" {
			t.Errorf("%s spec is incomplete: %+v", action, spec)
		}
	}
}

func TestDeleteRoutingProfileRemovesItsRuleSets(t *testing.T) {
	dir := t.TempDir()
	a := newTestApp(t, dir)
	saved, err := a.SaveRoutingProfile(plainProfile())
	if err != nil {
		t.Fatal(err)
	}
	path := proxy.RoutingListCachePath(dir, routingProfileRuleSetID(saved.ID, "direct"))
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("nothing to delete: %v", err)
	}
	if err := a.DeleteRoutingProfile(saved.ID); err != nil {
		t.Fatal(err)
	}
	// A leftover file would keep routing traffic by a profile that is gone.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("rule-set survived the delete: %v", err)
	}
}

func TestProfileNeedsGeoOnlyWhenReferenced(t *testing.T) {
	// No geo tokens: nothing to download, whatever URLs are present.
	site, ip := profileNeedsGeo(config.RoutingProfile{
		DirectSites: []string{"example.com"},
		GeoSiteURL:  "https://example.com/geosite.dat",
		GeoIPURL:    "https://example.com/geoip.dat",
	})
	if site || ip {
		t.Errorf("plain profile wants geo files: site=%v ip=%v", site, ip)
	}

	// Referenced but with no URL to fetch from: still nothing to download —
	// the tokens will simply be reported as unresolved.
	site, ip = profileNeedsGeo(config.RoutingProfile{
		DirectSites: []string{"geosite:private"},
		DirectIPs:   []string{"geoip:private"},
	})
	if site || ip {
		t.Errorf("no URLs but wants a download: site=%v ip=%v", site, ip)
	}

	// Referenced and fetchable.
	site, ip = profileNeedsGeo(config.RoutingProfile{
		DirectSites: []string{"geosite:private"},
		BlockIPs:    []string{"geoip:cn"},
		GeoSiteURL:  "https://example.com/geosite.dat",
		GeoIPURL:    "https://example.com/geoip.dat",
	})
	if !site || !ip {
		t.Errorf("referenced databases not requested: site=%v ip=%v", site, ip)
	}
}

func TestEnsureGeoFileUsesCacheWithoutNetwork(t *testing.T) {
	dir := t.TempDir()
	a := newTestApp(t, dir)
	url := "https://example.invalid/geosite.dat"

	// Seed the cache by hand. The URL is unreachable on purpose: if the cache
	// were ignored, this test would fail rather than quietly go to the network.
	raw := geoSiteListMsgForTest("PRIVATE", "localhost")
	if err := os.MkdirAll(a.geoDataDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.geoCachePath(geoKindSite, url), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	blob, err := a.ensureGeoFile(geoKindSite, url, false)
	if err != nil {
		t.Fatalf("ensureGeoFile: %v", err)
	}
	if len(blob) != len(raw) {
		t.Fatalf("got %d bytes, want the %d cached ones", len(blob), len(raw))
	}
}

func TestEnsureGeoFileRefusesToCacheNonGeoContent(t *testing.T) {
	// Reach the validator directly: an error page must never be stored, or
	// every later compile would fail against it while looking like a cache hit.
	if err := validateGeoBlob(geoKindSite, []byte("<!doctype html><h1>404</h1>")); err == nil {
		t.Fatal("an HTML page passed as a geo database")
	}
	if err := validateGeoBlob(geoKindIP, []byte("not protobuf")); err == nil {
		t.Fatal("garbage passed as a geoip database")
	}
}

func TestGeoCacheIsKeyedByURL(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	one := a.geoCachePath(geoKindSite, "https://a.example/geosite.dat")
	two := a.geoCachePath(geoKindSite, "https://b.example/geosite.dat")
	if one == two {
		t.Fatal("two different databases share one cache file")
	}
	// A profile switching URLs must not silently reuse the old contents.
	if filepath.Dir(one) != filepath.Dir(two) {
		t.Error("cache files scattered across directories")
	}
}

func TestNormalizeRoutingOrder(t *testing.T) {
	if got := proxy.NormalizeRoutingOrder("proxy-direct-block"); len(got) != 3 || got[0] != "proxy" {
		t.Errorf("valid order rejected: %v", got)
	}
	// Anything not a clean permutation falls back rather than being partly
	// honoured — the order decides which rule wins.
	for _, bad := range []string{"", "block-proxy", "block-proxy-nonsense", "block-block-proxy", "block proxy direct"} {
		got := proxy.NormalizeRoutingOrder(bad)
		if len(got) != 3 || got[0] != "block" || got[1] != "proxy" || got[2] != "direct" {
			t.Errorf("%q: got %v, want the default order", bad, got)
		}
	}
}

func TestActiveRoutingOrderFollowsProfile(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p := plainProfile()
	p.RouteOrder = "direct-proxy-block"
	saved, err := a.SaveRoutingProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetActiveRoutingProfile(saved.ID); err != nil {
		t.Fatal(err)
	}
	rr := a.config.GetConfig().RoutingRules
	rr.Mode = "global"
	if err := a.config.UpdateRoutingRules(rr); err != nil {
		t.Fatal(err)
	}
	got := a.activeRoutingOrder()
	if len(got) != 3 || got[0] != "direct" {
		t.Fatalf("order = %v, want the profile's", got)
	}
}

// geoSiteListMsgForTest builds a one-category geosite database. The wire format
// helpers live in internal/proxy's tests, so a small local encoder is used here.
func geoSiteListMsgForTest(category, domain string) []byte {
	pbBytes := func(field int, payload []byte) []byte {
		out := []byte{byte(field<<3 | 2), byte(len(payload))}
		return append(out, payload...)
	}
	pbString := func(field int, s string) []byte { return pbBytes(field, []byte(s)) }

	/* поле 1, wire-type varint (0) — тип домена; 2 = Domain. */
	dom := append([]byte{byte(1 << 3), 2}, pbString(2, domain)...)
	site := append(pbString(1, category), pbBytes(2, dom)...)
	return pbBytes(1, site)
}
