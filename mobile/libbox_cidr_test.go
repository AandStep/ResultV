// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mobile

import (
	"path/filepath"
	"slices"
	"testing"

	"resultproxy-wails/internal/proxy"
)

// Connect reads the subnet list off disk and never fetches. A cold start with
// no cache still has to produce a usable set, or the native Telegram client
// would go direct on a fresh install — the same failure the bundled SRS seed
// exists to prevent for domains.
func TestSmartBlockedCIDRsNeverBlocksConnect(t *testing.T) {
	dir := t.TempDir()

	got := smartBlockedCIDRs(true, dir)
	if len(got) == 0 {
		t.Fatal("no cache should still yield the builtin ranges")
	}
	if !slices.Contains(got, "91.108.4.0/22") {
		t.Errorf("builtin Telegram range missing: %v", got)
	}
}

func TestSmartBlockedCIDRsPrefersTheCache(t *testing.T) {
	dir := t.TempDir()
	err := proxy.SaveBlockedCIDRsCache(smartCIDRCachePath(dir), proxy.BlockedCIDRsCache{
		Source: "remote",
		CIDRs:  []string{"198.51.100.0/24"},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := smartBlockedCIDRs(true, dir)
	if !slices.Contains(got, "198.51.100.0/24") {
		t.Errorf("refreshed list not used: %v", got)
	}
}

// Global mode already sends everything through the proxy, so carrying the
// rule would only bloat the config.
func TestSmartBlockedCIDRsEmptyOutsideSmartMode(t *testing.T) {
	if got := smartBlockedCIDRs(false, t.TempDir()); got != nil {
		t.Errorf("Global mode should carry no subnet rule, got %v", got)
	}
}

// An unset dataDir is a real case on early startup paths; it must not panic or
// invent a path in the process working directory.
func TestSmartBlockedCIDRsToleratesMissingDataDir(t *testing.T) {
	if got := smartBlockedCIDRs(true, "  "); got != nil {
		t.Errorf("blank dataDir should yield nothing, got %v", got)
	}
}

func TestSmartCIDRCachePathSitsBesideTheDomainCache(t *testing.T) {
	dir := t.TempDir()
	got := smartCIDRCachePath(dir)
	if want := filepath.Join(dir, "smart-blocked-cidrs.json"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// Separate from the domain cache on purpose: one corrupt file must not
	// take the other list down with it.
	if got == filepath.Join(dir, "smart-blocked.json") {
		t.Error("subnet cache must not share the domain cache file")
	}
}
