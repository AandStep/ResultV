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

// Reading a routing list: a remote file of domains and/or CIDRs, either as
// plain lines or as a sing-box source-format rule_set.
//
// PORTED FROM THE DESKTOP, MINUS ITS WRITE HALF. The desktop's copy of this
// file also builds engine rule_sets and writes the cache in source format;
// neither survives here. Its buildRoutingListRuleSets emits SBRuleSet, a type
// that only exists on the desktop (this tree has the flat SBRouteRuleSet), and
// its WriteRoutingListRuleSet writes JSON the core would re-parse on every
// connect. Compilation to a binary rule-set lives in routing_ruleset.go
// instead. Everything below is unchanged, so a future sync against the desktop
// stays a readable diff.

import (
	"encoding/json"
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

// DefaultRoutingOrder is the order rules are emitted in when nothing says
// otherwise: restrictive first, so a host listed twice is blocked rather than
// let through.
var DefaultRoutingOrder = []string{"block", "proxy", "direct"}

// NormalizeRoutingOrder turns a profile's "block-proxy-direct" into the slice
// form, falling back to the default for anything that is not a permutation of
// the three actions. The order decides which rule wins when several match, so
// a half-understood value is refused rather than partially honoured.
func NormalizeRoutingOrder(raw string) []string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(raw)), "-")
	if len(parts) != 3 {
		return DefaultRoutingOrder
	}
	seen := map[string]bool{}
	for _, p := range parts {
		if p != "block" && p != "proxy" && p != "direct" {
			return DefaultRoutingOrder
		}
		if seen[p] {
			return DefaultRoutingOrder
		}
		seen[p] = true
	}
	return parts
}

// srcRuleSetFile mirrors the sing-box source-format rule_set JSON. Input only
// here: a provider may publish a list in this shape, but what this client
// writes is the binary form (see routing_ruleset.go).
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
