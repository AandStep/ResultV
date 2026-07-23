// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"bufio"
	"os"
	"sort"
	"strings"

	"github.com/AdguardTeam/golibs/log"
	"github.com/AdguardTeam/urlfilter/rules"
)

// CosmeticIndex supplements urlfilter's cosmetic engine with the two cases it
// gets wrong, both of which leave blank ad placeholders on real pages:
//
//  1. Host-specific element-hiding rules on SUBDOMAINS. urlfilter's
//     cosmeticLookupTable.findByHostname (cosmeticengine.go) keys rules by an
//     EXACT hostname map lookup, so an `interfax.ru##.banner` rule never fires
//     on `www.interfax.ru` — the rule.Match subdomain logic is dead because
//     the exact-key bucket is empty. Measured: interfax.ru returns 13 specific
//     rules, www.interfax.ru returns 0. This index walks the hostname's labels
//     (like [[ScriptletIndex]] does) so parent-domain rules apply to
//     subdomains.
//
//  2. ExtendedCSS (`#?#`/`#@?#`) rules. urlfilter's rule parser
//     (rules.NewCosmeticRule) only recognizes `##`/`#@#`; its marker switch
//     falls through to `default: return nil, ErrUnsupportedRule` for `#?#`, so
//     ExtCSS rules are dropped at parse time and CosmeticResult's *ExtCSS
//     arrays are ALWAYS empty for every host. Measured: 0 ExtCSS everywhere.
//     ExtCSS `:has()`/`:contains()` selectors are exactly what collapses the
//     leftover container around a network-blocked ad, so their absence is a
//     primary cause of the blank gaps. This index owns them, and the content
//     script applies them via the vendored @adguard/extended-css runtime.
//
// It deliberately does NOT re-emit generic `##` rules: urlfilter's engine
// already serves those (~57k of them), and duplicating would bloat every page.
// CSS-injection rules (`#$#`/`#$?#`) are out of scope — urlfilter drops them
// too, but they are rarer and higher-risk than element hiding.
type CosmeticIndex struct {
	// specificByDomain indexes domain-restricted `##` rules by each permitted
	// domain string. Match walks the hostname's labels to find candidates.
	specificByDomain map[string][]*rules.CosmeticRule

	// extByDomain indexes domain-restricted `#?#` ExtCSS rules by permitted
	// domain, mirroring specificByDomain.
	extByDomain map[string][]*extCSSRule

	// genericExt holds `#?#` ExtCSS rules with no permitted domains.
	genericExt []*extCSSRule

	// whitelist indexes `#@#` exception rules by content, mirroring
	// cosmeticLookupTable.whitelist.
	whitelist map[string][]*rules.CosmeticRule

	// whitelistExt indexes `#@?#` ExtCSS exception rules by content.
	whitelistExt map[string][]*extCSSRule
}

// extCSSRule is one parsed `#?#`/`#@?#` line. urlfilter's rule type can't hold
// it (the parser rejects the marker), so we carry the minimum ourselves.
type extCSSRule struct {
	content           string
	permittedDomains  []string
	restrictedDomains []string
}

// cosmeticMarkers are the markers CosmeticIndex recognizes, longest-first so a
// prefix test at the first '#' classifies `#@?#` before `#?#` and `#@#` before
// `##`. Markers we intentionally ignore (`#$#`, `#%#`, and their variants) are
// omitted: the first-'#' scan simply finds none and skips the line.
var cosmeticMarkers = []string{"#@?#", "#@#", "#?#", "##"}

// BuildCosmeticIndex parses the filter lists at paths and indexes their
// domain-specific `##`/`#@#` and all `#?#`/`#@?#` rules. A missing or
// unreadable list is logged and skipped rather than failing the whole build,
// so one broken list can't disable the MITM (mirrors BuildScriptletIndex).
func BuildCosmeticIndex(paths map[rules.ListID]string) (*CosmeticIndex, error) {
	ix := &CosmeticIndex{
		specificByDomain: map[string][]*rules.CosmeticRule{},
		extByDomain:      map[string][]*extCSSRule{},
		whitelist:        map[string][]*rules.CosmeticRule{},
		whitelistExt:     map[string][]*extCSSRule{},
	}

	for listID, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			log.Info("cosmeticindex: skipping list %d (%s): %v", listID, path, err)

			continue
		}

		parseCosmeticList(f, listID, ix)
		_ = f.Close()
	}

	return ix, nil
}

// parseCosmeticList scans r line by line, indexing each recognized rule.
func parseCosmeticList(f *os.File, listID rules.ListID, ix *CosmeticIndex) {
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == '!' || line[0] == '[' {
			continue
		}

		addCosmeticLine(ix, listID, line)
	}
	if err := scanner.Err(); err != nil {
		log.Info("cosmeticindex: scan error on list %d: %v", listID, err)
	}
}

// addCosmeticLine classifies one filter-list line by its cosmetic marker and
// indexes it. Domains never contain '#', so the first '#' that begins a known
// marker is the domain/content boundary. Lines with no such marker (network
// rules, CSS-injection, scriptlets, malformed) are ignored.
func addCosmeticLine(ix *CosmeticIndex, listID rules.ListID, line string) {
	marker, domains, content := splitCosmeticMarker(line)
	if marker == "" || content == "" {
		return
	}

	switch marker {
	case "##", "#@#":
		// Reuse urlfilter's own parser: it handles `##`/`#@#` correctly,
		// including domain restrictions and Match() subdomain semantics.
		r, err := rules.NewCosmeticRule(line, listID)
		if err != nil {
			return
		}
		if r.Whitelist {
			// `#@#` exceptions cancel matching rules everywhere they apply,
			// so index them regardless of whether they carry domains.
			ix.whitelist[r.Content] = append(ix.whitelist[r.Content], r)

			return
		}
		if r.IsGeneric() {
			// Generic `##` is already served by urlfilter's engine; skip to
			// avoid duplicating ~57k selectors on every page.
			return
		}
		for _, d := range r.GetPermittedDomains() {
			ix.specificByDomain[d] = append(ix.specificByDomain[d], r)
		}

	case "#?#", "#@?#":
		r := newExtCSSRule(domains, content)
		if marker == "#@?#" {
			ix.whitelistExt[content] = append(ix.whitelistExt[content], r)

			return
		}
		if len(r.permittedDomains) == 0 {
			ix.genericExt = append(ix.genericExt, r)

			return
		}
		for _, d := range r.permittedDomains {
			ix.extByDomain[d] = append(ix.extByDomain[d], r)
		}
	}
}

// splitCosmeticMarker finds the first recognized cosmetic marker in line and
// returns it along with the domains prefix and content suffix. Returns empty
// strings when the line carries no marker CosmeticIndex handles.
func splitCosmeticMarker(line string) (marker, domains, content string) {
	for i := 0; i < len(line); i++ {
		if line[i] != '#' {
			continue
		}
		rest := line[i:]
		for _, m := range cosmeticMarkers {
			if strings.HasPrefix(rest, m) {
				return m, line[:i], strings.TrimSpace(line[i+len(m):])
			}
		}
		// A '#' that starts no known cosmetic marker (e.g. `#$#`, `#%#`, or a
		// '#' inside a network-rule path) — not ours.
		return "", "", ""
	}

	return "", "", ""
}

// newExtCSSRule parses the comma-separated domains prefix of a `#?#`/`#@?#`
// rule into permitted/restricted lists (mirrors newScriptletRule).
func newExtCSSRule(domains, content string) *extCSSRule {
	r := &extCSSRule{content: content}
	if domains == "" {
		return r
	}
	for _, d := range strings.Split(domains, ",") {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if strings.HasPrefix(d, "~") {
			r.restrictedDomains = append(r.restrictedDomains, d[1:])
		} else {
			r.permittedDomains = append(r.permittedDomains, d)
		}
	}

	return r
}

// Match returns the element-hiding rule contents that apply to hostname but
// that urlfilter's engine misses: domain-specific `##` selectors (walked to
// cover subdomains), generic ExtCSS selectors, and specific ExtCSS selectors.
// Each slice is de-duplicated and sorted for deterministic output. A nil
// *CosmeticIndex returns three empty (non-nil) slices.
func (ix *CosmeticIndex) Match(hostname string) (specific, genericExtCSS, specificExtCSS []string) {
	specific = []string{}
	genericExtCSS = []string{}
	specificExtCSS = []string{}

	if ix == nil {
		return specific, genericExtCSS, specificExtCSS
	}

	labels := hostnameLabels(hostname)

	specificSet := map[string]struct{}{}
	for _, label := range labels {
		for _, r := range ix.specificByDomain[label] {
			// r.Match applies the rule's own permitted/restricted-domain
			// semantics against the real hostname, so a rule reached via the
			// "interfax.ru" bucket still validates for "www.interfax.ru".
			if !r.Match(hostname) {
				continue
			}
			if ix.isWhitelisted(hostname, r.Content) {
				continue
			}
			specificSet[r.Content] = struct{}{}
		}
	}

	genExtSet := map[string]struct{}{}
	for _, r := range ix.genericExt {
		if isDomainOrSubdomainOfAny(hostname, r.restrictedDomains) {
			continue
		}
		if ix.isWhitelistedExt(hostname, r.content) {
			continue
		}
		genExtSet[r.content] = struct{}{}
	}

	specExtSet := map[string]struct{}{}
	for _, label := range labels {
		for _, r := range ix.extByDomain[label] {
			if !isDomainOrSubdomainOfAny(hostname, r.permittedDomains) {
				continue
			}
			if isDomainOrSubdomainOfAny(hostname, r.restrictedDomains) {
				continue
			}
			if ix.isWhitelistedExt(hostname, r.content) {
				continue
			}
			specExtSet[r.content] = struct{}{}
		}
	}

	specific = setToSortedSlice(specificSet)
	genericExtCSS = setToSortedSlice(genExtSet)
	specificExtCSS = setToSortedSlice(specExtSet)

	return specific, genericExtCSS, specificExtCSS
}

// isWhitelisted reports whether a `#@#` rule with the given content applies to
// hostname (using urlfilter's own Match, which respects permitted/restricted
// domains). A domainless `#@#` applies everywhere.
func (ix *CosmeticIndex) isWhitelisted(hostname, content string) bool {
	for _, r := range ix.whitelist[content] {
		if r.Match(hostname) {
			return true
		}
	}

	return false
}

// isWhitelistedExt mirrors isWhitelisted for `#@?#` ExtCSS exceptions.
func (ix *CosmeticIndex) isWhitelistedExt(hostname, content string) bool {
	for _, r := range ix.whitelistExt[content] {
		if isDomainOrSubdomainOfAny(hostname, r.restrictedDomains) {
			continue
		}
		if len(r.permittedDomains) == 0 {
			return true
		}
		if isDomainOrSubdomainOfAny(hostname, r.permittedDomains) {
			return true
		}
	}

	return false
}

// dedupAppend appends each entry of extra to base that is not already present
// in base, preserving order. Used to merge the cosmetic index's matches into
// urlfilter's CosmeticResult without emitting a selector twice.
func dedupAppend(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base))
	for _, s := range base {
		seen[s] = struct{}{}
	}
	for _, s := range extra {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		base = append(base, s)
	}

	return base
}

// setToSortedSlice returns the set's keys as a sorted slice.
func setToSortedSlice(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)

	return out
}
