// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestNormalizeCIDRs(t *testing.T) {
	got := normalizeCIDRs([]string{
		"  91.108.4.0/22 ",
		"91.108.4.0/22", // duplicate
		"149.154.160.5", // bare IPv4 → /32
		"2a0a:f280::",   // bare IPv6 → /128
		"# comment",
		"",
		"not-an-ip",
		"10.0.0.0/33", // impossible prefix
		"91.108.4.7/22", // host bits set → canonicalised to the network
	})

	want := []string{
		"91.108.4.0/22",
		"149.154.160.5/32",
		"2a0a:f280::/128",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNormalizeCIDRsEmptyIsNeverNil(t *testing.T) {
	// The result is marshalled straight into the route rule; a nil slice and an
	// empty one differ in JSON, and an "ip_cidr": null would be rejected.
	if got := normalizeCIDRs(nil); got == nil || len(got) != 0 {
		t.Errorf("got %#v, want an empty non-nil slice", got)
	}
}

func TestDefaultBlockedCIDRsAreValid(t *testing.T) {
	defaults := defaultBlockedCIDRs()
	if len(defaults) == 0 {
		t.Fatal("no builtin ranges — Telegram would be unreachable offline")
	}
	if got := normalizeCIDRs(defaults); len(got) != len(defaults) {
		t.Errorf("some builtin ranges do not parse: %d of %d survived", len(got), len(defaults))
	}
}

// The invariant that matters most here: a source that ships a cloud-provider
// range would drag most of the internet into the tunnel. 172.64.0.0/13 is all
// of Cloudflare; 40.0.0.0/8 is Azure. Both were rejected upstream for that
// reason, so the builtin set must stay narrow.
func TestBuiltinRangesAreNarrow(t *testing.T) {
	const maxHostBits = 18 // /14 or narrower for IPv4
	for _, cidr := range defaultBlockedCIDRs() {
		if strings.Contains(cidr, ":") {
			continue // IPv6 allocations are legitimately large
		}
		var prefix int
		if _, err := fmt.Sscanf(cidr[strings.Index(cidr, "/")+1:], "%d", &prefix); err != nil {
			t.Fatalf("unparseable prefix in %q", cidr)
		}
		if 32-prefix > maxHostBits {
			t.Errorf("%s is too broad for a service range (/%d)", cidr, prefix)
		}
	}
}

func TestParseCIDRPayloadToleratesRealListFormats(t *testing.T) {
	raw := []byte(`
# Telegram
91.108.4.0/22
149.154.160.0/20  # inline comment
; semicolon comment
// slash comment

2001:67c:4e8::/48	trailing tab junk
`)
	got := parseCIDRPayload(raw)
	want := []string{"91.108.4.0/22", "149.154.160.0/20", "2001:67c:4e8::/48"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBlockedCIDRsCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "cidrs.json")
	in := BlockedCIDRsCache{Source: "remote", CIDRs: []string{"91.108.4.0/22", "1.2.3.4"}}

	if err := SaveBlockedCIDRsCache(path, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := LoadBlockedCIDRsCache(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if strings.Join(out.CIDRs, ",") != "91.108.4.0/22,1.2.3.4/32" {
		t.Errorf("round trip mangled the list: %v", out.CIDRs)
	}
	if out.UpdatedAt == 0 {
		t.Error("UpdatedAt should be stamped on save")
	}
}

func TestBlockedCIDRsCacheRejectsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cidrs.json")
	if err := SaveBlockedCIDRsCache(path, BlockedCIDRsCache{}); err == nil {
		t.Error("saving an empty list should fail rather than poison the cache")
	}
	// A cache file that parses but holds nothing must not be treated as valid,
	// or a bad refresh would silently disable Telegram routing.
	if err := os.WriteFile(path, []byte(`{"cidrs":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBlockedCIDRsCache(path); err == nil {
		t.Error("an empty cache should be reported as an error")
	}
}

type stubCIDRFetcher struct {
	cidrs []string
	err   error
}

func (s stubCIDRFetcher) FetchBlockedCIDRs(context.Context) ([]string, error) {
	return s.cidrs, s.err
}

func TestResolveBlockedCIDRsPrecedence(t *testing.T) {
	t.Run("remote wins and is cached", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cidrs.json")
		res := ResolveBlockedCIDRs(context.Background(),
			stubCIDRFetcher{cidrs: []string{"203.0.113.0/24"}}, path)

		if res.Source != "remote" {
			t.Fatalf("source = %q, want remote", res.Source)
		}
		cached, err := LoadBlockedCIDRsCache(path)
		if err != nil {
			t.Fatalf("remote result was not cached: %v", err)
		}
		if !slices.Contains(cached.CIDRs, "203.0.113.0/24") {
			t.Errorf("cache missing the fetched range: %v", cached.CIDRs)
		}
	})

	t.Run("falls back to cache when the fetch fails", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cidrs.json")
		if err := SaveBlockedCIDRsCache(path, BlockedCIDRsCache{CIDRs: []string{"198.51.100.0/24"}}); err != nil {
			t.Fatal(err)
		}
		res := ResolveBlockedCIDRs(context.Background(),
			stubCIDRFetcher{err: fmt.Errorf("offline")}, path)

		if res.Source != "cache" {
			t.Fatalf("source = %q, want cache", res.Source)
		}
		if !slices.Contains(res.CIDRs, "198.51.100.0/24") {
			t.Errorf("lost the cached range: %v", res.CIDRs)
		}
		if res.Err == nil {
			t.Error("the fetch error should still be reported alongside the fallback")
		}
	})

	t.Run("falls back to builtin with no cache", func(t *testing.T) {
		res := ResolveBlockedCIDRs(context.Background(),
			stubCIDRFetcher{err: fmt.Errorf("offline")},
			filepath.Join(t.TempDir(), "missing.json"))

		if res.Source != "builtin" {
			t.Fatalf("source = %q, want builtin", res.Source)
		}
		if len(res.CIDRs) == 0 {
			t.Error("builtin fallback must never be empty — MTProto has to work offline")
		}
	})
}

// Connect must never wait on a subnet download; this is the network-free path.
func TestLoadCachedBlockedCIDRs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cidrs.json")
	if res := LoadCachedBlockedCIDRs(path); res.Source != "builtin" || len(res.CIDRs) == 0 {
		t.Fatalf("no cache should yield a usable builtin set, got %+v", res)
	}

	if err := SaveBlockedCIDRsCache(path, BlockedCIDRsCache{CIDRs: []string{"198.51.100.0/24"}}); err != nil {
		t.Fatal(err)
	}
	res := LoadCachedBlockedCIDRs(path)
	if res.Source != "cache" || !slices.Contains(res.CIDRs, "198.51.100.0/24") {
		t.Errorf("cached list not used: %+v", res)
	}
}

// A corrupt cache must degrade to builtin, not disable Telegram routing.
func TestLoadCachedBlockedCIDRsSurvivesCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cidrs.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	res := LoadCachedBlockedCIDRs(path)
	if res.Source != "builtin" || len(res.CIDRs) == 0 {
		t.Errorf("corrupt cache should fall back to builtin, got %+v", res)
	}
}

// The whole point of the port: the ranges have to reach the route as an
// ip_cidr rule, because Telegram's native client offers no domain to match on.
func TestSmartModeEmitsIPCIDRRule(t *testing.T) {
	cfg := EngineConfig{
		Mode:                ProxyModeTunnel,
		SmartMode:           true,
		SmartBlockedDomains: []string{"instagram.com"},
		SmartBlockedCIDRs:   []string{"91.108.4.0/22", "149.154.160.0/20"},
		Proxy: ProxyConfig{
			IP: "203.0.113.9", Port: 443, Type: "VLESS",
			Extra: json.RawMessage(`{"uuid":"9b65b7e1-04c8-4717-8f45-2aa61fd25937"}`),
		},
	}

	raw, err := json.Marshal(BuildTunnelModeConfig(cfg))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"ip_cidr"`, "91.108.4.0/22", "149.154.160.0/20"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("route is missing %s:\n%s", want, raw)
		}
	}
}

// Global mode must not acquire the rule — it already sends everything through
// the proxy, and an extra ip_cidr rule would only be noise.
func TestGlobalModeOmitsIPCIDRRule(t *testing.T) {
	cfg := EngineConfig{
		Mode:              ProxyModeTunnel,
		SmartMode:         false,
		SmartBlockedCIDRs: []string{"91.108.4.0/22"},
		Proxy: ProxyConfig{
			IP: "203.0.113.9", Port: 443, Type: "VLESS",
			Extra: json.RawMessage(`{"uuid":"9b65b7e1-04c8-4717-8f45-2aa61fd25937"}`),
		},
	}

	raw, err := json.Marshal(BuildTunnelModeConfig(cfg))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "91.108.4.0/22") {
		t.Errorf("Global mode should not carry the Smart ip_cidr rule:\n%s", raw)
	}
}
