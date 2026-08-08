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

// The IP-subnet half of the Smart block-list, ported from the dev branch
// rather than written again — it already worked there, and a second
// implementation would only be a second set of bugs.
//
// Kept in one file on purpose. Upstream splits these across router.go,
// blocked_provider.go and blocked_updater.go; this branch has no router.go, and
// keeping the port contiguous makes it obvious what has to be re-synced when
// either side changes.

package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// normalizeCIDRs validates and de-duplicates a list of CIDR strings. Bare IPs
// are widened to host CIDRs (/32 or /128); blanks, comments and unparseable
// entries are dropped. Output is canonicalised via net.IPNet.String().
func normalizeCIDRs(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if !strings.Contains(s, "/") {
			ip := net.ParseIP(s)
			if ip == nil {
				continue
			}
			if ip.To4() != nil {
				s += "/32"
			} else {
				s += "/128"
			}
		}
		_, ipNet, err := net.ParseCIDR(s)
		if err != nil {
			continue
		}
		n := ipNet.String()
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

// defaultBlockedCIDRs are the Telegram MTProto data-center subnets. Telegram's
// native clients dial these IPs directly — no domain, no TLS SNI — so the
// domain-suffix block-list can never catch them; Smart mode needs an ip_cidr
// rule. Sourced from itdoginfo/allow-domains (Subnets/IPv4+IPv6/telegram.lst);
// kept as a static fallback for when the remote list and cache are unavailable.
func defaultBlockedCIDRs() []string {
	return []string{
		// IPv4
		"5.28.192.0/18",
		"91.105.192.0/23",
		"91.108.4.0/22",
		"91.108.8.0/21",
		"91.108.16.0/21",
		"91.108.56.0/22",
		"95.161.64.0/20",
		"149.154.160.0/20",
		"185.76.151.0/24",
		"194.221.0.0/16",
		// IPv6
		"2001:67c:4e8::/48",
		"2001:b28:f23c::/47",
		"2001:b28:f23f::/48",
		"2a0a:f280::/32",
	}
}

// defaultBlockedCIDRSources are the IP-only blocked services: Telegram MTProto
// data centers and Discord voice media servers (~2300 narrow /32s from
// Re:filter).
//
// Invariant: sources must be NARROW service ranges — never cloud-provider-wide
// blocks. itdog's Subnets/IPv4/discord.lst ships 172.64.0.0/13, which is all of
// Cloudflare and would drag half the internet into the tunnel; Flowseal's ipset
// ships 40.0.0.0/8, all of Azure. Both were rejected for that reason.
func defaultBlockedCIDRSources() []string {
	const itdog = "https://raw.githubusercontent.com/itdoginfo/allow-domains/main/Subnets/"
	return []string{
		itdog + "IPv4/telegram.lst",
		itdog + "IPv6/telegram.lst",
		"https://raw.githubusercontent.com/1andrevich/Re-filter-lists/main/discord_ips.lst",
	}
}

func (p *HTTPBlockedListProvider) blockedCIDRSources() []string {
	override := strings.TrimSpace(os.Getenv("RESULTPROXY_BLOCKED_CIDR_SOURCES"))
	if override == "" {
		// Legacy name from the Telegram-only era; keep working.
		override = strings.TrimSpace(os.Getenv("RESULTPROXY_TELEGRAM_CIDR_SOURCES"))
	}
	if override != "" {
		parts := strings.Split(override, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if s := strings.TrimSpace(part); s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return defaultBlockedCIDRSources()
}

// parseCIDRPayload extracts CIDR/IP entries from a newline-delimited list,
// tolerating blank lines, comments and trailing inline comments.
func parseCIDRPayload(raw []byte) []string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(trimmed, "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") || strings.HasPrefix(s, ";") || strings.HasPrefix(s, "//") {
			continue
		}
		if idx := strings.IndexAny(s, " \t#"); idx >= 0 {
			s = strings.TrimSpace(s[:idx])
		}
		if s != "" {
			out = append(out, s)
		}
	}
	return normalizeCIDRs(out)
}

// FetchBlockedCIDRs downloads the IP-only blocked-service subnets (Telegram
// MTProto data centers + Discord voice media servers) and returns them as
// normalized CIDR strings. Unlike the domain list this is country-independent.
// Sources are overridable via RESULTPROXY_BLOCKED_CIDR_SOURCES
// (comma-separated); the legacy RESULTPROXY_TELEGRAM_CIDR_SOURCES name still
// works as a fallback.
func (p *HTTPBlockedListProvider) FetchBlockedCIDRs(ctx context.Context) ([]string, error) {
	sources := p.blockedCIDRSources()
	if len(sources) == 0 {
		return nil, fmt.Errorf("blocked cidr sources are empty")
	}
	merged := make([]string, 0, 32)
	var lastErr error
	for _, u := range sources {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := p.client().Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("cidr service http %d: %s", resp.StatusCode, u)
			resp.Body.Close()
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		merged = append(merged, parseCIDRPayload(body)...)
	}
	merged = normalizeCIDRs(merged)
	if len(merged) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("empty cidr list")
	}
	return merged, nil
}

type BlockedCIDRsCache struct {
	UpdatedAt int64    `json:"updatedAt"`
	Source    string   `json:"source"`
	CIDRs     []string `json:"cidrs"`
}

func LoadBlockedCIDRsCache(path string) (BlockedCIDRsCache, error) {
	var out BlockedCIDRsCache
	data, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	out.CIDRs = normalizeCIDRs(out.CIDRs)
	if len(out.CIDRs) == 0 {
		return out, fmt.Errorf("cidr cache is empty")
	}
	return out, nil
}

func SaveBlockedCIDRsCache(path string, cache BlockedCIDRsCache) error {
	cache.CIDRs = normalizeCIDRs(cache.CIDRs)
	if len(cache.CIDRs) == 0 {
		return fmt.Errorf("cidrs are empty")
	}
	if cache.UpdatedAt == 0 {
		cache.UpdatedAt = time.Now().Unix()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o600)
}

// CIDRFetcher fetches the IP-subnet block-list. Narrow interface (satisfied by
// *HTTPBlockedListProvider) so callers and tests don't depend on the full
// BlockedListProvider.
type CIDRFetcher interface {
	FetchBlockedCIDRs(ctx context.Context) ([]string, error)
}

type BlockedCIDRsResolveResult struct {
	CIDRs  []string
	Source string
	Err    error
}

// LoadCachedBlockedCIDRs is the network-free path: cache → builtin. The static
// defaultBlockedCIDRs() fallback is Telegram-only, but it always guarantees a
// usable set so MTProto works offline and at first launch; Discord ranges
// arrive with the first successful remote refresh.
//
// This is what Smart mode uses at connect time, so a cold start never waits on
// a subnet download — the same reasoning as the bundled Smart SRS seed.
func LoadCachedBlockedCIDRs(cachePath string) BlockedCIDRsResolveResult {
	if cachePath != "" {
		cache, err := LoadBlockedCIDRsCache(cachePath)
		if err == nil && len(cache.CIDRs) > 0 {
			return BlockedCIDRsResolveResult{CIDRs: cache.CIDRs, Source: "cache"}
		}
	}
	return BlockedCIDRsResolveResult{
		CIDRs:  normalizeCIDRs(defaultBlockedCIDRs()),
		Source: "builtin",
	}
}

// ResolveBlockedCIDRs resolves the IP-subnet block-list with the same
// remote → cache → builtin precedence as ResolveBlockedDomains, minus the
// country dimension (the ranges are global). Always returns a usable set.
func ResolveBlockedCIDRs(ctx context.Context, fetcher CIDRFetcher, cachePath string) BlockedCIDRsResolveResult {
	var lastErr error
	if fetcher != nil {
		cidrs, err := fetcher.FetchBlockedCIDRs(ctx)
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
