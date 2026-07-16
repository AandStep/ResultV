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

// ScriptletIndex parses and indexes AdGuard `#%#`/`#@%#` cosmetic-JS rules
// so the content script (Task 4) can execute them.
//
// This exists because the vendored urlfilter library's rule parser
// (rules.NewCosmeticRule) only recognizes the `##`/`#@#` markers; its marker
// switch falls through to `default: return nil, ErrUnsupportedRule` for
// `#%#`/`#@%#`, so those rules are dropped at parse time and never reach
// urlfilter's own CosmeticEngine — GetCosmeticResult(...).JS is always
// empty. See the spec CORRECTION. Rather than fork the widely-used `rules`
// package, we parse and match `#%#`/`#@%#` ourselves, mirroring the
// semantics of rules.CosmeticRule and the engine's cosmeticLookupTable
// (github.com/AdguardTeam/urlfilter cosmeticengine.go/rules/cosmetic.go)
// with one deliberate divergence: upstream's lookup table keys rules by an
// exact hostname map lookup (its own TODO notes "Improve hosts matching"),
// but we walk hostname labels so a rule targeting "noodlemagazine.com"
// also fires on "hot.noodlemagazine.com" — the case this feature exists
// to serve.
type ScriptletIndex struct {
	// byDomain indexes specific (domain-restricted) rules by each exact
	// permitted domain string. Match walks the hostname's labels to find
	// candidate entries rather than scanning every rule.
	byDomain map[string][]*scriptletRule

	// generic holds rules with no permitted domains.
	generic []*scriptletRule

	// whitelist indexes #@%# exception rules by their content, mirroring
	// cosmeticLookupTable.whitelist.
	whitelist map[string][]*scriptletRule
}

// scriptletRule is one parsed `#%#`/`#@%#` line.
type scriptletRule struct {
	content           string
	permittedDomains  []string
	restrictedDomains []string
}

// BuildScriptletIndex parses the filter lists at paths and returns an index
// of their `#%#`/`#@%#` rules. A missing or unreadable list is logged and
// skipped rather than failing the whole build, so one broken list can't
// disable the MITM.
func BuildScriptletIndex(paths map[rules.ListID]string) (*ScriptletIndex, error) {
	ix := &ScriptletIndex{
		byDomain:  map[string][]*scriptletRule{},
		whitelist: map[string][]*scriptletRule{},
	}

	for listID, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			log.Info("scriptletindex: skipping list %d (%s): %v", listID, path, err)

			continue
		}

		parseScriptletList(f, ix)
		_ = f.Close()
	}

	return ix, nil
}

// parseScriptletList scans r line by line, adding each recognized
// `#%#`/`#@%#` rule to ix.
func parseScriptletList(r *os.File, ix *ScriptletIndex) {
	scanner := bufio.NewScanner(r)
	// Filter list lines can be long (long scriptlet payloads); grow the
	// buffer past bufio's 64KiB default.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}

		addScriptletLine(ix, line)
	}
}

// addScriptletLine parses a single filter list line and, if it is a
// `#%#`/`#@%#` rule, indexes it. Any other line (element hiding, CSS,
// unrecognized) is ignored.
func addScriptletLine(ix *ScriptletIndex, line string) {
	// Check the exception marker first: "#@%#" does not contain "#%#" as a
	// substring, but checking order defensively keeps this correct even if
	// that ever changes.
	if idx := strings.Index(line, "#@%#"); idx != -1 {
		domains := line[:idx]
		content := line[idx+len("#@%#"):]
		if content == "" {
			return
		}

		r := newScriptletRule(domains, content)
		ix.whitelist[content] = append(ix.whitelist[content], r)

		return
	}

	if idx := strings.Index(line, "#%#"); idx != -1 {
		domains := line[:idx]
		content := line[idx+len("#%#"):]
		if content == "" {
			return
		}

		r := newScriptletRule(domains, content)
		if r.isGeneric() {
			ix.generic = append(ix.generic, r)

			return
		}

		for _, d := range r.permittedDomains {
			ix.byDomain[d] = append(ix.byDomain[d], r)
		}

		return
	}

	// Not a #%#/#@%# rule (e.g. ##, #@#, #$# or malformed) — not ours.
}

// newScriptletRule builds a scriptletRule from the raw comma-separated
// domains prefix and content suffix of a rule line.
func newScriptletRule(domains, content string) *scriptletRule {
	r := &scriptletRule{content: content}

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

// isGeneric reports whether r has no permitted domains.
func (r *scriptletRule) isGeneric() bool {
	return len(r.permittedDomains) == 0
}

// Match returns the generic and specific scriptlet rule contents that apply
// to hostname, each de-duplicated and sorted for deterministic output. A nil
// *ScriptletIndex returns two empty (non-nil) slices.
func (ix *ScriptletIndex) Match(hostname string) (generic, specific []string) {
	generic = []string{}
	specific = []string{}

	if ix == nil {
		return generic, specific
	}

	genericSet := map[string]struct{}{}
	for _, r := range ix.generic {
		if isDomainOrSubdomainOfAny(hostname, r.restrictedDomains) {
			continue
		}
		if ix.isWhitelisted(hostname, r.content) {
			continue
		}

		genericSet[r.content] = struct{}{}
	}

	specificSet := map[string]struct{}{}
	for _, label := range hostnameLabels(hostname) {
		for _, r := range ix.byDomain[label] {
			if isDomainOrSubdomainOfAny(hostname, r.restrictedDomains) {
				continue
			}
			if ix.isWhitelisted(hostname, r.content) {
				continue
			}

			specificSet[r.content] = struct{}{}
		}
	}

	for c := range genericSet {
		generic = append(generic, c)
	}
	for c := range specificSet {
		specific = append(specific, c)
	}

	sort.Strings(generic)
	sort.Strings(specific)

	return generic, specific
}

// isWhitelisted reports whether a #@%# rule with the given content applies
// to hostname (domain-or-subdomain match against its permitted domains,
// respecting any restricted domains).
func (ix *ScriptletIndex) isWhitelisted(hostname, content string) bool {
	for _, r := range ix.whitelist[content] {
		// Restricted domains take precedence: if hostname matches any
		// restricted domain, this rule does not apply.
		if isDomainOrSubdomainOfAny(hostname, r.restrictedDomains) {
			continue
		}

		if len(r.permittedDomains) == 0 {
			// A whitelist rule with no domains applies everywhere
			// (unless restricted above).
			return true
		}

		if isDomainOrSubdomainOfAny(hostname, r.permittedDomains) {
			return true
		}
	}

	return false
}

// hostnameLabels returns hostname and each of its parent domains, e.g.
// "hot.noodlemagazine.com" -> ["hot.noodlemagazine.com",
// "noodlemagazine.com", "com"].
func hostnameLabels(hostname string) []string {
	var labels []string

	for {
		labels = append(labels, hostname)

		idx := strings.Index(hostname, ".")
		if idx == -1 {
			break
		}

		hostname = hostname[idx+1:]
	}

	return labels
}

// isDomainOrSubdomainOfAny reports whether domain equals, or is a
// subdomain of, any entry in domains.
func isDomainOrSubdomainOfAny(domain string, domains []string) bool {
	for _, d := range domains {
		if domain == d || strings.HasSuffix(domain, "."+d) {
			return true
		}
	}

	return false
}
