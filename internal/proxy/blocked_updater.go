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
	"fmt"
	"strings"
	"time"
)

type BlockedDomainsResolveResult struct {
	Domains []string
	Source  string
	Country string
	Err     error
}

func ResolveBlockedDomains(ctx context.Context, provider BlockedListProvider, cachePath string, localPaths ...string) BlockedDomainsResolveResult {
	var lastErr error
	resolvedCountry := ""
	if provider != nil {
		country, err := provider.ResolveCountry(ctx)
		if err == nil {
			resolvedCountry = strings.ToLower(strings.TrimSpace(country))
			domains, fetchErr := provider.FetchBlockedDomains(ctx, country)
			if fetchErr == nil && len(domains) > 0 {
				cache := BlockedDomainsCache{
					Country:   resolvedCountry,
					UpdatedAt: time.Now().Unix(),
					Source:    "remote",
					Domains:   domains,
				}
				if saveErr := SaveBlockedDomainsCache(cachePath, cache); saveErr != nil {
					lastErr = fmt.Errorf("save cache: %w", saveErr)
				}
				return BlockedDomainsResolveResult{
					Domains: normalizeDomains(domains),
					Source:  "remote",
					Country: cache.Country,
					Err:     lastErr,
				}
			}
			lastErr = fetchErr
		} else {
			lastErr = err
		}
	}

	if cachePath != "" {
		cache, err := LoadBlockedDomainsCache(cachePath)
		if err == nil && len(cache.Domains) > 0 {
			cacheCountry := strings.ToLower(strings.TrimSpace(cache.Country))
			if resolvedCountry != "" && cacheCountry != "" && cacheCountry != resolvedCountry {
				lastErr = fmt.Errorf("cache country mismatch: cache=%s resolved=%s", cacheCountry, resolvedCountry)
			} else {
				return BlockedDomainsResolveResult{
					Domains: cache.Domains,
					Source:  "cache",
					Country: cacheCountry,
					Err:     lastErr,
				}
			}
		} else {
			lastErr = err
		}
	}

	localDomains := LoadBlockedDomainsFromFiles(localPaths...)
	if len(localDomains) > 0 {
		return BlockedDomainsResolveResult{
			Domains: localDomains,
			Source:  "local",
			Country: resolvedCountry,
			Err:     lastErr,
		}
	}

	return BlockedDomainsResolveResult{
		Domains: normalizeDomains(defaultBlockedDomains()),
		Source:  "builtin",
		Country: resolvedCountry,
		Err:     lastErr,
	}
}

// CIDRFetcher fetches the Telegram MTProto subnet list. Narrow interface
// (satisfied by *HTTPBlockedListProvider) so callers and tests don't depend on
// the full BlockedListProvider.
type CIDRFetcher interface {
	FetchTelegramCIDRs(ctx context.Context) ([]string, error)
}

type BlockedCIDRsResolveResult struct {
	CIDRs  []string
	Source string
	Err    error
}

// ResolveBlockedCIDRs resolves the IP-subnet block-list with the same
// remote → cache → builtin precedence as ResolveBlockedDomains, minus the
// country dimension (Telegram ranges are global). Always returns a usable set:
// the static defaultBlockedCIDRs() fallback guarantees Telegram works offline.
func ResolveBlockedCIDRs(ctx context.Context, fetcher CIDRFetcher, cachePath string) BlockedCIDRsResolveResult {
	var lastErr error
	if fetcher != nil {
		cidrs, err := fetcher.FetchTelegramCIDRs(ctx)
		if err == nil && len(cidrs) > 0 {
			cache := BlockedCIDRsCache{
				UpdatedAt: time.Now().Unix(),
				Source:    "remote",
				CIDRs:     cidrs,
			}
			if saveErr := SaveBlockedCIDRsCache(cachePath, cache); saveErr != nil {
				lastErr = fmt.Errorf("save cidr cache: %w", saveErr)
			}
			return BlockedCIDRsResolveResult{CIDRs: normalizeCIDRs(cidrs), Source: "remote", Err: lastErr}
		}
		lastErr = err
	}

	if cachePath != "" {
		cache, err := LoadBlockedCIDRsCache(cachePath)
		if err == nil && len(cache.CIDRs) > 0 {
			return BlockedCIDRsResolveResult{CIDRs: cache.CIDRs, Source: "cache", Err: lastErr}
		}
		if err != nil {
			lastErr = err
		}
	}

	return BlockedCIDRsResolveResult{
		CIDRs:  normalizeCIDRs(defaultBlockedCIDRs()),
		Source: "builtin",
		Err:    lastErr,
	}
}

func RefreshRemoteBlockedDomains(ctx context.Context, provider BlockedListProvider, cachePath string) BlockedDomainsResolveResult {
	if provider == nil {
		return BlockedDomainsResolveResult{Err: fmt.Errorf("provider is nil")}
	}
	country, err := provider.ResolveCountry(ctx)
	if err != nil {
		return BlockedDomainsResolveResult{Err: err}
	}
	domains, err := provider.FetchBlockedDomains(ctx, country)
	if err != nil {
		return BlockedDomainsResolveResult{Err: err}
	}
	if len(domains) == 0 {
		return BlockedDomainsResolveResult{Err: fmt.Errorf("empty domains")}
	}
	cache := BlockedDomainsCache{
		Country:   strings.ToLower(strings.TrimSpace(country)),
		UpdatedAt: time.Now().Unix(),
		Source:    "remote",
		Domains:   domains,
	}
	if err := SaveBlockedDomainsCache(cachePath, cache); err != nil {
		return BlockedDomainsResolveResult{
			Domains: normalizeDomains(domains),
			Source:  "remote",
			Country: cache.Country,
			Err:     err,
		}
	}
	return BlockedDomainsResolveResult{
		Domains: normalizeDomains(domains),
		Source:  "remote",
		Country: cache.Country,
	}
}
