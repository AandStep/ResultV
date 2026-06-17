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
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package proxy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeBlockedProvider struct {
	country    string
	countryErr error
	domains    []string
	domainsErr error
}

func (f fakeBlockedProvider) ResolveCountry(ctx context.Context) (string, error) {
	if f.countryErr != nil {
		return "", f.countryErr
	}
	return f.country, nil
}

func (f fakeBlockedProvider) FetchBlockedDomains(ctx context.Context, country string) ([]string, error) {
	if f.domainsErr != nil {
		return nil, f.domainsErr
	}
	return f.domains, nil
}

type fakeCIDRFetcher struct {
	cidrs []string
	err   error
}

func (f fakeCIDRFetcher) FetchTelegramCIDRs(ctx context.Context) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.cidrs, nil
}

func TestResolveBlockedCIDRs_RemoteSuccess(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cidr.json")
	res := ResolveBlockedCIDRs(context.Background(), fakeCIDRFetcher{cidrs: []string{"149.154.160.0/20"}}, cachePath)
	if res.Source != "remote" {
		t.Fatalf("expected remote source, got %s (err=%v)", res.Source, res.Err)
	}
	loaded, err := LoadBlockedCIDRsCache(cachePath)
	if err != nil {
		t.Fatalf("load cidr cache after remote: %v", err)
	}
	if len(loaded.CIDRs) != 1 || loaded.CIDRs[0] != "149.154.160.0/20" {
		t.Fatalf("unexpected persisted cidrs: %v", loaded.CIDRs)
	}
}

func TestResolveBlockedCIDRs_FallbackToCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cidr.json")
	if err := SaveBlockedCIDRsCache(cachePath, BlockedCIDRsCache{
		Source: "remote",
		CIDRs:  []string{"91.108.4.0/22"},
	}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	res := ResolveBlockedCIDRs(context.Background(), fakeCIDRFetcher{err: errors.New("offline")}, cachePath)
	if res.Source != "cache" {
		t.Fatalf("expected cache source, got %s", res.Source)
	}
	if len(res.CIDRs) != 1 || res.CIDRs[0] != "91.108.4.0/22" {
		t.Fatalf("unexpected cached cidrs: %v", res.CIDRs)
	}
}

func TestResolveBlockedCIDRs_StaticFallback(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cidr.json") // no file → no cache
	res := ResolveBlockedCIDRs(context.Background(), fakeCIDRFetcher{err: errors.New("offline")}, cachePath)
	if res.Source != "builtin" {
		t.Fatalf("expected builtin fallback, got %s", res.Source)
	}
	if len(res.CIDRs) == 0 {
		t.Fatal("builtin fallback must be non-empty so Telegram works offline")
	}
}

func TestResolveBlockedDomains_FallbackToCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache.json")
	if err := SaveBlockedDomainsCache(cachePath, BlockedDomainsCache{
		Country: "ru",
		Source:  "remote",
		Domains: []string{"cached.com"},
	}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	res := ResolveBlockedDomains(context.Background(), fakeBlockedProvider{
		countryErr: errors.New("country down"),
	}, cachePath)
	if res.Source != "cache" {
		t.Fatalf("expected cache source, got %s", res.Source)
	}
	if len(res.Domains) != 1 || res.Domains[0] != "cached.com" {
		t.Fatalf("unexpected cache domains: %v", res.Domains)
	}
}

func TestResolveBlockedDomains_RemoteSuccess(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache.json")
	res := ResolveBlockedDomains(context.Background(), fakeBlockedProvider{
		country: "ru",
		domains: []string{"remote.com"},
	}, cachePath)
	if res.Source != "remote" {
		t.Fatalf("expected remote source, got %s", res.Source)
	}
	loaded, err := LoadBlockedDomainsCache(cachePath)
	if err != nil {
		t.Fatalf("load cache after remote: %v", err)
	}
	if len(loaded.Domains) != 1 || loaded.Domains[0] != "remote.com" {
		t.Fatalf("unexpected persisted domains: %v", loaded.Domains)
	}
}

func TestRefreshRemoteBlockedDomains_Error(t *testing.T) {
	res := RefreshRemoteBlockedDomains(context.Background(), fakeBlockedProvider{
		countryErr: errors.New("offline"),
	}, filepath.Join(t.TempDir(), "cache.json"))
	if res.Err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveBlockedDomains_SkipCacheWhenCountryChanged(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache.json")
	if err := SaveBlockedDomainsCache(cachePath, BlockedDomainsCache{
		Country: "ru",
		Source:  "remote",
		Domains: []string{"cached.com"},
	}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	localFile := filepath.Join(t.TempDir(), "list.txt")
	if err := os.WriteFile(localFile, []byte("local-fi.com\n"), 0o600); err != nil {
		t.Fatalf("write local list: %v", err)
	}
	res := ResolveBlockedDomains(context.Background(), fakeBlockedProvider{
		country:    "fi",
		domainsErr: errors.New("no sources"),
	}, cachePath, localFile)
	if res.Source != "local" {
		t.Fatalf("expected local source, got %s", res.Source)
	}
	if len(res.Domains) != 1 || res.Domains[0] != "local-fi.com" {
		t.Fatalf("unexpected domains: %v", res.Domains)
	}
}
