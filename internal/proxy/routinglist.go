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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ParsedRoutingList is the normalized output of a fetched routing list:
// suffix-compressed domains and canonical CIDRs.
type ParsedRoutingList struct {
	Domains []string
	CIDRs   []string
}

// srcRuleSetFile mirrors the sing-box source-format rule_set JSON we both
// accept as input and emit as cache.
type srcRuleSetFile struct {
	Version int             `json:"version"`
	Rules   []srcRuleSetRule `json:"rules"`
}

type srcRuleSetRule struct {
	Domain       []string `json:"domain,omitempty"`
	DomainSuffix []string `json:"domain_suffix,omitempty"`
	IPCidr       []string `json:"ip_cidr,omitempty"`
}

// ParseRoutingListPayload autodetects the format. A body starting with '{'
// is parsed as a sing-box source-JSON rule-set; otherwise it is treated as a
// newline list of domains and/or CIDRs. Output is normalized: domains are
// suffix-compressed, CIDRs are canonical, bare IPs widened to host CIDRs.
func ParseRoutingListPayload(raw []byte) ParsedRoutingList {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ParsedRoutingList{}
	}
	if strings.HasPrefix(trimmed, "{") {
		if p, ok := parseSourceJSONRuleSet(raw); ok {
			return p
		}
		// Fall through to line parsing if JSON was malformed.
	}
	var domains, cidrs []string
	for _, line := range strings.Split(trimmed, "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") || strings.HasPrefix(s, ";") ||
			strings.HasPrefix(s, "!") || strings.HasPrefix(s, "//") {
			continue
		}
		// A token containing '/' with a leading digit, or a bare IP, is a CIDR.
		if looksLikeCIDROrIP(s) {
			cidrs = append(cidrs, s)
			continue
		}
		if d := extractDomainFromLine(s); d != "" {
			domains = append(domains, d)
		}
	}
	return ParsedRoutingList{
		Domains: compressDomainSuffixes(normalizeDomains(domains)),
		CIDRs:   normalizeCIDRs(cidrs),
	}
}

func parseSourceJSONRuleSet(raw []byte) (ParsedRoutingList, bool) {
	var f srcRuleSetFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return ParsedRoutingList{}, false
	}
	var domains, cidrs []string
	for _, r := range f.Rules {
		domains = append(domains, r.Domain...)
		domains = append(domains, r.DomainSuffix...)
		cidrs = append(cidrs, r.IPCidr...)
	}
	return ParsedRoutingList{
		Domains: compressDomainSuffixes(normalizeDomains(domains)),
		CIDRs:   normalizeCIDRs(cidrs),
	}, true
}

// looksLikeCIDROrIP reports whether a raw list line is an IP/CIDR rather than
// a domain. A line qualifies only if, after stripping any inline comment, it
// consists solely of IP-legal characters (hex digits, '.', ':', '/') AND
// contains at least one decimal digit or a ':'. This deliberately excludes
// adblock forms like `||example.com/path^` (letters '|','x','p'… fail the
// charset test) so they route to the domain parser, not normalizeCIDRs (which
// would silently drop them).
func looksLikeCIDROrIP(s string) bool {
	if idx := strings.IndexAny(s, " \t#"); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	if s == "" {
		return false
	}
	hasDigit, hasColon := false, false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == ':':
			hasColon = true
		case r == '.' || r == '/':
			// allowed separators
		case (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F'):
			// hex digit (IPv6) — allowed but not a "digit" signal
		default:
			return false // any other char (letters g-z, '|', '*', '^', '_') → domain
		}
	}
	return hasDigit || hasColon
}

const (
	routingListsSubdir        = "routing/lists"
	routingListRuleSetVersion = 3
)

// RoutingListsDir is where per-list source-format rule_set caches live.
func RoutingListsDir(dataDir string) string {
	return filepath.Join(dataDir, routingListsSubdir)
}

// RoutingListCachePath is the cache file for one list id. The id is used
// verbatim as the base name; callers must pass sanitized ids (see the app
// layer, which generates them). No path separators are permitted.
func RoutingListCachePath(dataDir, id string) string {
	return filepath.Join(RoutingListsDir(dataDir), id+".json")
}

// RoutingListRuleSetTag is the sing-box rule_set tag for a list id.
func RoutingListRuleSetTag(id string) string {
	return "rl-" + id
}

// WriteRoutingListRuleSet writes the parsed list as a sing-box source-format
// rule_set. Returns an error (and writes nothing) when the list is empty, so
// callers can reject the add and preserve any previous cache on refresh.
func WriteRoutingListRuleSet(dataDir, id string, p ParsedRoutingList) error {
	if len(p.Domains) == 0 && len(p.CIDRs) == 0 {
		return fmt.Errorf("routing list %q is empty", id)
	}
	rule := srcRuleSetRule{DomainSuffix: p.Domains, IPCidr: p.CIDRs}
	f := srcRuleSetFile{Version: routingListRuleSetVersion, Rules: []srcRuleSetRule{rule}}
	blob, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(RoutingListsDir(dataDir), 0o700); err != nil {
		return err
	}
	return os.WriteFile(RoutingListCachePath(dataDir, id), blob, 0o600)
}
