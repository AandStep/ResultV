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
	// ExactDomains match the host and nothing under it. Only the geo databases
	// distinguish the two (xray's `full:` vs `domain:`); every other source we
	// read is suffix-only and leaves this empty. Kept apart from Domains
	// because folding an exact entry into a suffix silently widens the rule.
	ExactDomains []string
}

// RoutingListSpec is a resolved routing list ready for buildRoute: the
// sing-box rule_set tag, the local source-format cache path, and the action.
type RoutingListSpec struct {
	Tag    string
	Path   string
	Action string // "proxy" | "direct" | "block"
}

// buildRoutingListRuleSets returns a local source-format rule_set per list
// whose cache file exists and is non-empty.
func buildRoutingListRuleSets(specs []RoutingListSpec) []SBRuleSet {
	out := make([]SBRuleSet, 0, len(specs))
	for _, s := range specs {
		if !routingListCacheReady(s.Path) {
			continue
		}
		out = append(out, SBRuleSet{
			Type:         "local",
			Tag:          s.Tag,
			Format:       "source",
			LocalOptions: SBLocalRuleSet{Path: s.Path},
		})
	}
	return out
}

// appendRoutingListRouteRules appends one route rule per list, ordered
// restrictive-first: all block, then proxy, then direct. block → reject;
// proxy/direct → route to the matching outbound.
func appendRoutingListRouteRules(specs []RoutingListSpec, rules []SBRouteRule) []SBRouteRule {
	for _, action := range []string{"block", "proxy", "direct"} {
		for _, s := range specs {
			if s.Action != action || !routingListCacheReady(s.Path) {
				continue
			}
			r := SBRouteRule{RuleSet: []string{s.Tag}}
			switch action {
			case "block":
				r.Action = "reject"
			case "proxy":
				r.Action = "route"
				r.Outbound = "proxy"
			default: // "direct"
				r.Action = "route"
				r.Outbound = "direct"
			}
			rules = append(rules, r)
		}
	}
	return rules
}

func routingListCacheReady(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && st.Size() > 0
}

// srcRuleSetFile mirrors the sing-box source-format rule_set JSON we both
// accept as input and emit as cache.
type srcRuleSetFile struct {
	Version int              `json:"version"`
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
	// An HTML/XML page (the classic "pasted a GitHub blob URL, not the raw
	// file" mistake) is never a valid list — refuse it rather than scraping
	// junk "domains" out of the markup. A real list starts with '{' (source
	// JSON) or a domain/comment, never '<'.
	if strings.HasPrefix(trimmed, "<") {
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
		Domains: compressDomainSuffixes(plausibleDomains(normalizeDomains(domains))),
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
		Domains: compressDomainSuffixes(plausibleDomains(normalizeDomains(domains))),
		CIDRs:   normalizeCIDRs(cidrs),
	}, true
}

// NormalizeRoutingListURL rewrites a GitHub "blob" web-page URL to its raw
// content URL so a normal github.com link fetches the file, not the HTML page:
//
//	https://github.com/OWNER/REPO/blob/REF/PATH → https://raw.githubusercontent.com/OWNER/REPO/REF/PATH
//
// Any query/fragment on the blob URL is dropped. Already-raw, non-blob, and
// non-github URLs are returned unchanged.
func NormalizeRoutingListURL(raw string) string {
	u := strings.TrimSpace(raw)
	lower := strings.ToLower(u)
	var rest string
	switch {
	case strings.HasPrefix(lower, "https://github.com/"):
		rest = u[len("https://github.com/"):]
	case strings.HasPrefix(lower, "http://github.com/"):
		rest = u[len("http://github.com/"):]
	case strings.HasPrefix(lower, "https://www.github.com/"):
		rest = u[len("https://www.github.com/"):]
	default:
		return u
	}
	// rest = OWNER/REPO/blob/REF/PATH...
	parts := strings.SplitN(rest, "/", 4)
	if len(parts) < 4 || parts[2] != "blob" {
		return u
	}
	owner, repo, path := parts[0], parts[1], parts[3]
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	if owner == "" || repo == "" || path == "" {
		return u
	}
	return "https://raw.githubusercontent.com/" + owner + "/" + repo + "/" + path
}

// LooksLikeRoutingListHTML reports whether a fetched body is an HTML/XML page
// rather than a domain/CIDR list — the common mistake of pasting a GitHub
// "blob" page URL instead of the raw file. A real list starts with '{' (source
// JSON) or a domain/comment line, never '<'.
func LooksLikeRoutingListHTML(raw []byte) bool {
	t := strings.TrimSpace(string(raw))
	return strings.HasPrefix(t, "<")
}

// plausibleDomains keeps only entries that look like real hostnames, dropping
// markup/junk tokens (containing '=', '"', '<', spaces, etc.) that the lenient
// line parser might otherwise admit. Input must already be normalized (lower
// case, no scheme/path).
func plausibleDomains(in []string) []string {
	out := make([]string, 0, len(in))
	for _, d := range in {
		if isPlausibleDomain(d) {
			out = append(out, d)
		}
	}
	return out
}

func isPlausibleDomain(s string) bool {
	if s == "" || !strings.Contains(s, ".") {
		return false
	}
	if strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") ||
		strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
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
	if len(p.Domains) == 0 && len(p.CIDRs) == 0 && len(p.ExactDomains) == 0 {
		return fmt.Errorf("routing list %q is empty", id)
	}
	rule := srcRuleSetRule{Domain: p.ExactDomains, DomainSuffix: p.Domains, IPCidr: p.CIDRs}
	f := srcRuleSetFile{Version: routingListRuleSetVersion, Rules: []srcRuleSetRule{rule}}
	blob, err := json.Marshal(f)
	if err != nil {
		return err
	}
	dir := RoutingListsDir(dataDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, id+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(blob); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	// os.Rename replaces an existing destination on both Unix and Windows.
	if err := os.Rename(tmpName, RoutingListCachePath(dataDir, id)); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
