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
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type BlockedListProvider interface {
	ResolveCountry(ctx context.Context) (string, error)
	FetchBlockedDomains(ctx context.Context, country string) ([]string, error)
}

// HTTPBlockedListProvider fetches blocked-domain lists from CDN-hosted
// sources and resolves the user's country via the project-controlled
// CountryClient. The previous implementation cycled through three
// third-party IP-geolocation services (ip-api.com over plaintext HTTP,
// ipapi.co, geojs.io), leaking the user's real IP each time. That fallback
// chain is gone — country resolution now goes only through the project API.
type HTTPBlockedListProvider struct {
	Client          *http.Client
	Country         *CountryClient
	ListURLTemplate string
}

// NewHTTPBlockedListProvider constructs a provider tied to the given user
// data directory (where the country cache lives). Pass an empty string for
// tests that don't need persistence.
func NewHTTPBlockedListProvider(userDataPath string) *HTTPBlockedListProvider {
	return &HTTPBlockedListProvider{
		Client:          &http.Client{Timeout: 10 * time.Second},
		Country:         NewCountryClient(userDataPath),
		ListURLTemplate: strings.TrimSpace(os.Getenv("RESULTPROXY_BLOCKED_LIST_URL_TEMPLATE")),
	}
}

func (p *HTTPBlockedListProvider) ResolveCountry(ctx context.Context) (string, error) {
	if p.Country == nil {
		return "", fmt.Errorf("country client not initialised")
	}
	return p.Country.LookupSelfCountry(ctx)
}

// compressDomainSuffixes drops entries already covered by a parent suffix in
// the same list (cdn.discord.com is redundant next to discord.com). Keeps
// input order. Input must be normalized (see normalizeDomains).
func compressDomainSuffixes(domains []string) []string {
	if len(domains) < 2 {
		return domains
	}
	set := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		set[d] = struct{}{}
	}
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		covered := false
		for h := d; ; {
			idx := strings.Index(h, ".")
			if idx < 0 {
				break
			}
			h = h[idx+1:]
			if _, ok := set[h]; ok {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, d)
		}
	}
	return out
}

func (p *HTTPBlockedListProvider) FetchBlockedDomains(ctx context.Context, country string) ([]string, error) {
	country = strings.ToLower(strings.TrimSpace(country))
	if country == "" {
		return nil, fmt.Errorf("country is empty")
	}
	templates := p.sourceTemplates(country)
	if len(templates) == 0 {
		return nil, fmt.Errorf("list sources are empty")
	}
	merged := make([]string, 0, 2048)
	var lastErr error
	for _, tpl := range templates {
		u := strings.ReplaceAll(strings.TrimSpace(tpl), "{country}", country)
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
			lastErr = fmt.Errorf("list service http %d: %s", resp.StatusCode, u)
			resp.Body.Close()
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		domains := parseDomainPayload(body)
		if len(domains) == 0 {
			lastErr = fmt.Errorf("empty domain list: %s", u)
			continue
		}
		merged = append(merged, domains...)
	}
	merged = compressDomainSuffixes(normalizeDomains(merged))
	if len(merged) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("empty domain list")
	}
	return merged, nil
}

func (p *HTTPBlockedListProvider) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return &http.Client{Timeout: 10 * time.Second}
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

// defaultBlockedCIDRSources are the IP-only blocked services: Telegram MTProto
// data centers and Discord voice media servers (~2300 narrow /32s from
// Re:filter). Invariant: sources must be NARROW service ranges — never
// cloud-provider-wide blocks (itdog's Subnets/IPv4/discord.lst ships
// 172.64.0.0/13 = all of Cloudflare and would drag half the internet into
// the tunnel; Flowseal's ipset ships 40.0.0.0/8 = Azure. Both rejected).
func defaultBlockedCIDRSources() []string {
	const itdog = "https://raw.githubusercontent.com/itdoginfo/allow-domains/main/Subnets/"
	return []string{
		itdog + "IPv4/telegram.lst",
		itdog + "IPv6/telegram.lst",
		"https://raw.githubusercontent.com/1andrevich/Re-filter-lists/main/discord_ips.lst",
	}
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

type BlockedDomainsCache struct {
	Country   string   `json:"country"`
	UpdatedAt int64    `json:"updatedAt"`
	Source    string   `json:"source"`
	Domains   []string `json:"domains"`
}

func LoadBlockedDomainsCache(path string) (BlockedDomainsCache, error) {
	var out BlockedDomainsCache
	data, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	out.Domains = normalizeDomains(out.Domains)
	if len(out.Domains) == 0 {
		return out, fmt.Errorf("cache is empty")
	}
	return out, nil
}

func SaveBlockedDomainsCache(path string, cache BlockedDomainsCache) error {
	cache.Domains = normalizeDomains(cache.Domains)
	if len(cache.Domains) == 0 {
		return fmt.Errorf("domains are empty")
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

func LoadBlockedDomainsFromFiles(paths ...string) []string {
	if len(paths) == 0 {
		return nil
	}
	all := make([]string, 0, 128)
	for _, p := range paths {
		all = append(all, loadDomainFile(p)...)
	}
	return normalizeDomains(all)
}

func parseDomainPayload(raw []byte) []string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	if csvDomains := parseDomainsFromCSV(raw); len(csvDomains) > 0 {
		return normalizeDomains(csvDomains)
	}
	var lines []string
	if strings.HasPrefix(trimmed, "[") {
		var arr []string
		if err := json.Unmarshal(raw, &arr); err == nil {
			return normalizeDomains(arr)
		}
	}
	for _, line := range strings.Split(trimmed, "\n") {
		s := extractDomainFromLine(line)
		if s == "" {
			continue
		}
		lines = append(lines, s)
	}
	return normalizeDomains(lines)
}

func parseDomainsFromCSV(raw []byte) []string {
	reader := csv.NewReader(bytes.NewReader(raw))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		return nil
	}
	header := records[0]
	urlIdx := -1
	for i, col := range header {
		if strings.EqualFold(strings.TrimSpace(col), "url") {
			urlIdx = i
			break
		}
	}
	if urlIdx < 0 {
		return nil
	}
	out := make([]string, 0, len(records)-1)
	for _, rec := range records[1:] {
		if urlIdx >= len(rec) {
			continue
		}
		host := hostFromURLLike(rec[urlIdx])
		if host == "" {
			continue
		}
		out = append(out, host)
	}
	return out
}

func extractDomainFromLine(line string) string {
	s := strings.TrimSpace(strings.ToLower(line))
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "#") || strings.HasPrefix(s, ";") || strings.HasPrefix(s, "!") || strings.HasPrefix(s, "//") {
		return ""
	}
	if idx := strings.Index(s, "#"); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	if strings.HasPrefix(s, "server=/") {
		rest := strings.TrimPrefix(s, "server=/")
		if idx := strings.Index(rest, "/"); idx >= 0 {
			return normalizeRule(rest[:idx])
		}
	}
	if strings.Contains(s, " ") || strings.Contains(s, "\t") {
		parts := strings.Fields(s)
		if len(parts) >= 2 {
			last := parts[len(parts)-1]
			if last != "localhost" && last != "local" {
				return normalizeRule(last)
			}
		}
	}
	if strings.HasPrefix(s, "||") {
		s = strings.TrimPrefix(s, "||")
		if idx := strings.IndexAny(s, "^/$"); idx >= 0 {
			s = s[:idx]
		}
		return normalizeRule(s)
	}
	return normalizeRule(s)
}

func hostFromURLLike(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return normalizeRule(raw)
	}
	return normalizeRule(u.Hostname())
}

func (p *HTTPBlockedListProvider) sourceTemplates(country string) []string {
	overrideMany := strings.TrimSpace(os.Getenv("RESULTPROXY_BLOCKED_LIST_SOURCES"))
	if overrideMany != "" {
		parts := strings.Split(overrideMany, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if strings.TrimSpace(p.ListURLTemplate) != "" {
		return []string{p.ListURLTemplate}
	}
	return defaultPublicSourceTemplates(country)
}

func defaultPublicSourceTemplates(country string) []string {
	cc := strings.ToLower(strings.TrimSpace(country))
	if cc == "" {
		return nil
	}
	if cc == "ru" {
		// NO citizenlab/test-lists here. That corpus is a censorship-MEASUREMENT
		// list — sites researchers probe for blocking — not a "blocked, send via
		// VPN" list. Its global.csv+ru.csv contributed ~20k .ru entries to the
		// block-list (kremlin.ru, ria.ru, 1tv.ru, fsb.ru as research targets;
		// yandex.ru, mail.ru, vk.com, ok.ru, 2ip.ru as probe endpoints). Since
		// buildRoute emits the block-list as domain_suffix, a bare 2LD like
		// yandex.ru swallowed every subdomain, so Smart mode dragged Russian
		// traffic — including mc.yandex.ru and other counters embedded in nearly
		// every RU page — through the tunnel. RU is covered by the registry and
		// service lists below, which is what "blocked" actually means here.
		return []string{
			// Реестровые + «режут RU» списки. Re:filter domains_all ~86k записей,
			// 1.4 MB — в лимит 8 MB LimitReader укладывается с запасом.
			"https://raw.githubusercontent.com/itdoginfo/allow-domains/main/Russia/inside-raw.lst",
			"https://raw.githubusercontent.com/1andrevich/Re-filter-lists/main/domains_all.lst",
			"https://raw.githubusercontent.com/1andrevich/Re-filter-lists/main/community.lst",
			// Точечные сервис-списки: побочные домены Discord/YouTube и сервисы,
			// гео-блокирующие RU (Google AI Studio) — их нет в реестровых списках.
			"https://raw.githubusercontent.com/itdoginfo/allow-domains/main/Services/discord.lst",
			"https://raw.githubusercontent.com/itdoginfo/allow-domains/main/Services/youtube.lst",
			"https://raw.githubusercontent.com/itdoginfo/allow-domains/main/Services/google_ai.lst",
			// Курируемый сообществом Discord-веер + Cloudflare-ECH хвосты.
			"https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/main/lists/list-general.txt",
		}
	}
	// Countries other than RU have no curated registry source, so citizenlab
	// stays their only input despite its noise.
	base := "https://raw.githubusercontent.com/citizenlab/test-lists/master/lists/"
	return []string{
		base + "global.csv",
		base + cc + ".csv",
	}
}

// (Previously this file also defined countryEndpoints and
// resolveCountryFromEndpoint, which cycled through ip-api.com / ipapi.co /
// geojs.io. They are removed — see CountryClient in country.go.)
