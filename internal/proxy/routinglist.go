// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"encoding/json"
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
