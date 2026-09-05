// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

package proxy

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
)

const discordVoiceNet = "66.22.192.0/18"

func hasCIDR(list []string, want string) bool {
	for _, c := range list {
		if c == want {
			return true
		}
	}
	return false
}

// 66.22.192.0/18 is registered at ARIN as US-DISCORD1 — the whole /18 is
// Discord's, with per-region sub-allocations (discord-nlrtm1, discord-brcoa1,
// discord-sgsin1). The upstream list ships it as scattered /32s and covered
// 11.8% of it when measured on 2026-08-26 (28 of 64 /24s touched at all), so
// which voice server a guild lands on decided whether the call worked. The
// floor must be present whatever the list source did.
func TestResolveBlockedCIDRs_DiscordVoiceFloorPresentForEverySource(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(t *testing.T) (CIDRFetcher, string)
		source string
	}{
		{
			name: "remote",
			setup: func(t *testing.T) (CIDRFetcher, string) {
				return fakeCIDRFetcher{cidrs: []string{"66.22.206.163/32"}}, filepath.Join(t.TempDir(), "cidr.json")
			},
			source: "remote",
		},
		{
			name: "cache",
			setup: func(t *testing.T) (CIDRFetcher, string) {
				p := filepath.Join(t.TempDir(), "cidr.json")
				if err := SaveBlockedCIDRsCache(p, BlockedCIDRsCache{Source: "remote", CIDRs: []string{"91.108.4.0/22"}}); err != nil {
					t.Fatalf("seed cache: %v", err)
				}
				return fakeCIDRFetcher{err: errors.New("offline")}, p
			},
			source: "cache",
		},
		{
			name: "builtin",
			setup: func(t *testing.T) (CIDRFetcher, string) {
				return fakeCIDRFetcher{err: errors.New("offline")}, filepath.Join(t.TempDir(), "cidr.json")
			},
			source: "builtin",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fetcher, cachePath := tc.setup(t)
			res := ResolveBlockedCIDRs(context.Background(), fetcher, cachePath)
			if res.Source != tc.source {
				t.Fatalf("expected source %s, got %s (err=%v)", tc.source, res.Source, res.Err)
			}
			if !hasCIDR(res.CIDRs, discordVoiceNet) {
				t.Fatalf("%s source lost the Discord voice floor %s: %v", tc.source, discordVoiceNet, res.CIDRs)
			}
		})
	}
}

// The floor must not evict what the source supplied — it is a union, not a
// replacement.
func TestResolveBlockedCIDRs_FloorKeepsSourceEntries(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cidr.json")
	res := ResolveBlockedCIDRs(context.Background(), fakeCIDRFetcher{cidrs: []string{"149.154.160.0/20"}}, cachePath)
	if !hasCIDR(res.CIDRs, "149.154.160.0/20") {
		t.Fatalf("remote entry dropped: %v", res.CIDRs)
	}
	if !hasCIDR(res.CIDRs, discordVoiceNet) {
		t.Fatalf("floor missing: %v", res.CIDRs)
	}
}

// The persisted cache stays a faithful record of what the remote list said.
// Mixing the floor into it would make a later "did upstream change?" read lie.
func TestResolveBlockedCIDRs_FloorNotWrittenIntoCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cidr.json")
	ResolveBlockedCIDRs(context.Background(), fakeCIDRFetcher{cidrs: []string{"149.154.160.0/20"}}, cachePath)
	loaded, err := LoadBlockedCIDRsCache(cachePath)
	if err != nil {
		t.Fatalf("load cache: %v", err)
	}
	if hasCIDR(loaded.CIDRs, discordVoiceNet) {
		t.Fatalf("floor leaked into the persisted remote cache: %v", loaded.CIDRs)
	}
}

// The floor has to survive the curated-list width guard: normalizeBlockedCIDRs
// drops IPv4 prefixes wider than /16, and a /18 must stay.
func TestBlockedCIDRFloor_SurvivesWidthGuard(t *testing.T) {
	if !hasCIDR(normalizeBlockedCIDRs(blockedCIDRFloor()), discordVoiceNet) {
		t.Fatalf("width guard ate the floor: %v", normalizeBlockedCIDRs(blockedCIDRFloor()))
	}
}

// Guard the claim the floor rests on: every observed Discord voice address in
// 66.22.x must fall inside the aggregate we ship.
func TestBlockedCIDRFloor_CoversObservedDiscordVoiceHosts(t *testing.T) {
	_, ipNet, err := net.ParseCIDR(discordVoiceNet)
	if err != nil {
		t.Fatal(err)
	}
	// Sampled from the live blocked_cidr_cache.json on 2026-08-26, spanning
	// the lowest and highest /24 the upstream list touched.
	for _, ip := range []string{"66.22.196.10", "66.22.206.163", "66.22.220.7", "66.22.248.1"} {
		if !ipNet.Contains(net.ParseIP(ip)) {
			t.Fatalf("%s is outside %s — the aggregate is wrong", ip, discordVoiceNet)
		}
	}
}
