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
