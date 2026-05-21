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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultCountryAPIURL is the project-controlled GeoLite2 endpoint. It
// replaces the previous trio of third-party services (ip-api.com over plain
// HTTP, ipapi.co, geojs.io) that leaked the user's real IP to outside
// providers. Override via env RESULTPROXY_COUNTRY_API_URL for tests / staging.
const DefaultCountryAPIURL = "https://result-proxy.ru/countryAPI/api.php"

// countryCacheTTL keeps lookups quiet — both for the user (no network call
// on every UI refresh) and for the upstream host (~200 req/day across 1500
// users at 7-day TTL would still be fine, but 24h is the right trade-off for
// "country might change after a VPN switch").
const countryCacheTTL = 24 * time.Hour

// countryCacheMaxEntries bounds the on-disk IP-cache size. The cache holds
// (ip, country, timestamp) entries for proxy-server country lookups; a
// runaway subscription with thousands of servers shouldn't be able to grow
// the cache forever. Beyond this, the oldest entries are evicted.
const countryCacheMaxEntries = 512

// CountryClient looks up ISO-3166 alpha-2 country codes through the
// project-controlled API and caches results on disk. Two distinct lookups
// are exposed:
//
//   - LookupCountryByIP: pass an IP, get its country. Used for proxy-server
//     flags in the UI. Cached per-IP.
//   - LookupSelfCountry: ask the API to derive country from the request's
//     REMOTE_ADDR. Used by the Smart-mode blocked-list resolver. Cached
//     globally (one "my country" entry).
//
// The client never falls back to other providers — if the project API is
// unreachable, callers receive an error and must handle a missing country
// (e.g. show "??" flag, skip Smart-mode country-specific lists). This
// closes the previous IP-leak surface where unreachable endpoints triggered
// requests to ip-api.com / ipapi.co.
type CountryClient struct {
	httpClient *http.Client
	apiURL     string
	cachePath  string

	mu      sync.Mutex
	cache   countryCacheFile
	loaded  bool
}

type countryCacheEntry struct {
	Country   string `json:"country"`
	UpdatedAt int64  `json:"updatedAt"`
}

type countryCacheFile struct {
	Self    *countryCacheEntry           `json:"self,omitempty"`
	ByIP    map[string]countryCacheEntry `json:"byIP,omitempty"`
	Version int                          `json:"version"`
}

// NewCountryClient builds a client rooted at the given user-data directory.
// The cache file lives alongside the rest of the user's app state.
func NewCountryClient(userDataPath string) *CountryClient {
	apiURL := strings.TrimSpace(os.Getenv("RESULTPROXY_COUNTRY_API_URL"))
	if apiURL == "" {
		apiURL = DefaultCountryAPIURL
	}
	return &CountryClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		apiURL:     apiURL,
		cachePath:  filepath.Join(userDataPath, "country.cache.json"),
	}
}

// LookupCountryByIP returns the lowercase ISO-3166 alpha-2 country code for
// the given host. The argument may be a literal IPv4/IPv6 address OR a
// hostname (VPN servers are often expressed as "game.example.today" instead
// of an IP). Hostnames are resolved through DNS before being sent to the
// API — the API's MaxMind backend rejects non-IP strings.
//
// Cache key is the ORIGINAL host string (domain or IP), so repeat lookups
// for the same proxy entry hit the cache even when DNS rotates the
// underlying IP between calls.
func (c *CountryClient) LookupCountryByIP(ctx context.Context, host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", errors.New("empty host")
	}
	c.mu.Lock()
	c.loadLocked()
	if entry, ok := c.cache.ByIP[host]; ok && !cacheExpired(entry) {
		c.mu.Unlock()
		return entry.Country, nil
	}
	c.mu.Unlock()

	// Resolve to an IP literal if needed. MaxMind's reader strictly requires
	// IPs (it errors on hostnames), and we don't want the country API to
	// have to do DNS for us — that adds load and a second network hop.
	queryIP, err := resolveToIP(ctx, host)
	if err != nil {
		return "", err
	}

	country, err := c.fetch(ctx, queryIP)
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.loadLocked()
	if c.cache.ByIP == nil {
		c.cache.ByIP = make(map[string]countryCacheEntry, 1)
	}
	c.cache.ByIP[host] = countryCacheEntry{Country: country, UpdatedAt: time.Now().Unix()}
	c.evictLocked()
	_ = c.saveLocked()
	c.mu.Unlock()
	return country, nil
}

// resolveToIP turns a host literal into something MaxMind can read. Pure-IP
// inputs are returned unchanged. Hostnames are resolved via the system
// resolver; we prefer the first IPv4 (most subscription servers are still
// dual-stack with IPv4 primary) and fall back to IPv6 only if no v4 exists.
// The resolution is bounded by a short timeout so a slow DNS server never
// stalls a UI render that's waiting on a flag.
func resolveToIP(ctx context.Context, host string) (string, error) {
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), nil
	}
	resolveCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addrs, err := (&net.Resolver{}).LookupIPAddr(resolveCtx, host)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("resolve %q: no addresses", host)
	}
	// IPv4-first preference.
	for _, a := range addrs {
		if v4 := a.IP.To4(); v4 != nil {
			return v4.String(), nil
		}
	}
	return addrs[0].IP.String(), nil
}

// LookupSelfCountry returns the country of the caller (resolved by the API
// from REMOTE_ADDR). Cached as a single "self" entry — there is only ever
// one current country per machine, and the cache is invalidated by TTL
// rather than by IP change (which would be hard to detect without a network
// call anyway). 24h is short enough that travelling users get a fresh
// answer within a day of crossing a border.
func (c *CountryClient) LookupSelfCountry(ctx context.Context) (string, error) {
	c.mu.Lock()
	c.loadLocked()
	if c.cache.Self != nil && !cacheExpired(*c.cache.Self) {
		country := c.cache.Self.Country
		c.mu.Unlock()
		return country, nil
	}
	c.mu.Unlock()

	country, err := c.fetch(ctx, "")
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.cache.Self = &countryCacheEntry{Country: country, UpdatedAt: time.Now().Unix()}
	_ = c.saveLocked()
	c.mu.Unlock()
	return country, nil
}

// fetch performs the HTTP GET against the country API. When ip is empty the
// API is expected to fall back to REMOTE_ADDR server-side.
func (c *CountryClient) fetch(ctx context.Context, ip string) (string, error) {
	u, err := url.Parse(c.apiURL)
	if err != nil {
		return "", fmt.Errorf("parse api url: %w", err)
	}
	// Pin HTTPS. Even if someone overrides the URL via env, plaintext is
	// refused — the whole point of this fix is no plaintext IP traffic.
	if u.Scheme != "https" {
		return "", fmt.Errorf("country API must use https, got scheme %q", u.Scheme)
	}
	if ip != "" {
		q := u.Query()
		q.Set("ip", ip)
		u.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	// Send a generic UA — versioned UAs fingerprint the install. The country
	// API doesn't need to know which build is asking.
	req.Header.Set("User-Agent", "ResultV")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("country api request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("country api http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	// Schema: {"country":"RU","country_lower":"ru","region":"...","eu":"0",...}
	// Tolerant parser — only the country field is required.
	var payload struct {
		Country      string `json:"country"`
		CountryLower string `json:"country_lower"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("parse country json: %w", err)
	}
	code := strings.ToLower(strings.TrimSpace(payload.CountryLower))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(payload.Country))
	}
	if code == "" || code == "??" {
		return "", fmt.Errorf("country api returned no country code")
	}
	if len(code) != 2 {
		return "", fmt.Errorf("country api returned %q (expected 2 letters)", code)
	}
	return code, nil
}

func cacheExpired(e countryCacheEntry) bool {
	if e.UpdatedAt <= 0 {
		return true
	}
	return time.Since(time.Unix(e.UpdatedAt, 0)) > countryCacheTTL
}

// loadLocked reads the cache file if not already loaded. Caller holds c.mu.
// A missing/corrupted file is treated as empty cache — never fatal.
func (c *CountryClient) loadLocked() {
	if c.loaded {
		return
	}
	c.loaded = true
	c.cache = countryCacheFile{Version: 1, ByIP: make(map[string]countryCacheEntry)}
	if c.cachePath == "" {
		return
	}
	data, err := os.ReadFile(c.cachePath)
	if err != nil {
		return
	}
	var parsed countryCacheFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return
	}
	if parsed.ByIP == nil {
		parsed.ByIP = make(map[string]countryCacheEntry)
	}
	c.cache = parsed
}

func (c *CountryClient) saveLocked() error {
	if c.cachePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.cachePath), 0o700); err != nil {
		return err
	}
	c.cache.Version = 1
	data, err := json.Marshal(c.cache)
	if err != nil {
		return err
	}
	return os.WriteFile(c.cachePath, data, 0o600)
}

// evictLocked trims the per-IP cache to countryCacheMaxEntries by removing
// the oldest entries. Self-country is never evicted.
func (c *CountryClient) evictLocked() {
	if len(c.cache.ByIP) <= countryCacheMaxEntries {
		return
	}
	type kv struct {
		ip string
		ts int64
	}
	list := make([]kv, 0, len(c.cache.ByIP))
	for ip, e := range c.cache.ByIP {
		list = append(list, kv{ip: ip, ts: e.UpdatedAt})
	}
	// Selection-sort-style partial sort: find the (len-max) oldest and drop.
	// Simple bubble works fine at ~512 entries.
	for len(c.cache.ByIP) > countryCacheMaxEntries {
		oldestIdx := 0
		for i := range list {
			if list[i].ts < list[oldestIdx].ts {
				oldestIdx = i
			}
		}
		delete(c.cache.ByIP, list[oldestIdx].ip)
		list = append(list[:oldestIdx], list[oldestIdx+1:]...)
	}
}
