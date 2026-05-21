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
