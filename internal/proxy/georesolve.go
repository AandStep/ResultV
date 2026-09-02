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

// Resolution of xray routing tokens into the rule-set model.
//
// A routing profile lists its rules the way xray writes them — "geosite:private",
// "geoip:whitelist", "domain:example.com", "8.8.8.8/32". This turns one such
// list into a ParsedRoutingList, expanding geo references against the databases
// read by geodat.go.
//
// Anything that cannot survive the trip is reported rather than dropped in
// silence: a profile that imports half its rules must be able to say which half
// and why, otherwise the user sees traffic take the wrong route with no
// explanation anywhere.

import (
	"fmt"
	"sort"
	"strings"
)

// GeoResolveReport accounts for everything the resolver could not turn into a
// rule. Counts and reasons, not a log: the caller decides how loudly to say it.
type GeoResolveReport struct {
	// Unresolved maps a token to why it did not make it.
	Unresolved map[string]string
	// DroppedFromDB counts geosite entries the database itself carried in a
	// form no rule-set can express (keyword and regex matchers).
	DroppedFromDB int
}

func (r GeoResolveReport) Empty() bool {
	return len(r.Unresolved) == 0 && r.DroppedFromDB == 0
}

// Tokens returns the unresolved tokens in a stable order for messages.
func (r GeoResolveReport) Tokens() []string {
	out := make([]string, 0, len(r.Unresolved))
	for t := range r.Unresolved {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// GeoDatabases holds the two parsed files. A nil map simply means that database
// was not loaded — every reference into it is then reported as unresolved,
// which is the honest outcome and never a silent pass.
type GeoDatabases struct {
	Sites map[string][]GeoDomain
	IPs   map[string][]string
	// SiteDropped carries the count ParseGeoSiteDat reported, so a resolve
	// against these databases can pass it through to its own report.
	SiteDropped int
	// InvertedIPs are geoip categories that carry inverse_match and were
	// therefore not loaded; referencing one is an error, not an empty result.
	InvertedIPs map[string]struct{}
}

// ResolveGeoTokens expands one action's token list — the DirectSites, ProxyIp,
// BlockSites… of a routing profile — into rules.
//
// Domains and IPs share a token list in some panels, so both are accepted here
// and sorted by shape rather than by which field they came from.
func ResolveGeoTokens(tokens []string, db GeoDatabases) (ParsedRoutingList, GeoResolveReport) {
	report := GeoResolveReport{Unresolved: map[string]string{}}
	var suffixes, exact, cidrs []string

	for _, raw := range tokens {
		token := strings.TrimSpace(raw)
		if token == "" || strings.HasPrefix(token, "#") {
			continue
		}
		lower := strings.ToLower(token)

		switch {
		case strings.HasPrefix(lower, "geosite:"):
			s, e, err := resolveGeoSiteToken(lower, db)
			if err != nil {
				report.Unresolved[token] = err.Error()
				continue
			}
			suffixes = append(suffixes, s...)
			exact = append(exact, e...)

		case strings.HasPrefix(lower, "geoip:"):
			c, err := resolveGeoIPToken(lower, db)
			if err != nil {
				report.Unresolved[token] = err.Error()
				continue
			}
			cidrs = append(cidrs, c...)

		case strings.HasPrefix(lower, "ext:") || strings.HasPrefix(lower, "ext-ip:"):
			// Points at a .dat file shipped alongside the client. We have no
			// such file and no way to fetch it from the token alone.
			report.Unresolved[token] = "external geo file is not supported"

		case strings.HasPrefix(lower, "regexp:"):
			report.Unresolved[token] = "regular expressions are not supported by the rule engine"

		case strings.HasPrefix(lower, "keyword:"):
			report.Unresolved[token] = "substring matching is not supported by the rule engine"

		case strings.HasPrefix(lower, "full:"):
			if v := strings.TrimSpace(lower[len("full:"):]); v != "" {
				exact = append(exact, v)
			}

		case strings.HasPrefix(lower, "domain:"):
			if v := strings.TrimSpace(lower[len("domain:"):]); v != "" {
				suffixes = append(suffixes, v)
			}

		case looksLikeCIDROrIP(lower):
			cidrs = append(cidrs, lower)

		default:
			// A bare host. xray reads this as a substring match, but every
			// list we have ever been handed writes plain host names here and
			// means the host with its sub-domains — and that is already how
			// the embedded-xray parser (sublists.go) reads them. Kept the same
			// on purpose: two readings of the same token in one client would
			// route identical lists differently depending on where they came
			// from.
			if d := extractDomainFromLine(lower); d != "" {
				suffixes = append(suffixes, d)
			} else {
				report.Unresolved[token] = "not a domain, IP or known geo reference"
			}
		}
	}

	report.DroppedFromDB = db.SiteDropped

	parsed := ParsedRoutingList{
		Domains: compressDomainSuffixes(plausibleDomains(normalizeDomains(suffixes))),
		CIDRs:   normalizeCIDRs(cidrs),
	}
	// An exact entry covered by a suffix in the same rule is redundant: the
	// suffix already matches the host itself.
	parsed.ExactDomains = dropCoveredBySuffix(
		plausibleDomains(normalizeDomains(exact)), parsed.Domains)
	return parsed, report
}

func resolveGeoSiteToken(token string, db GeoDatabases) (suffixes, exact []string, err error) {
	name := strings.TrimSpace(token[len("geosite:"):])
	if name == "" {
		return nil, nil, fmt.Errorf("empty category")
	}
	// `geosite:google@ads` filters a category by attribute. The attributes are
	// skipped when the database is read, so honouring the base category alone
	// would import far more than the author asked for.
	if i := strings.IndexByte(name, '@'); i >= 0 {
		return nil, nil, fmt.Errorf("attribute filter %q is not supported", name[i:])
	}
	if strings.HasPrefix(name, "!") {
		return nil, nil, fmt.Errorf("negated categories are not supported")
	}
	if db.Sites == nil {
		return nil, nil, fmt.Errorf("geosite database is not loaded")
	}
	entries, ok := db.Sites[name]
	if !ok {
		return nil, nil, fmt.Errorf("category %q is not in the geosite database", name)
	}
	for _, e := range entries {
		if e.Exact {
			exact = append(exact, e.Value)
		} else {
			suffixes = append(suffixes, e.Value)
		}
	}
	return suffixes, exact, nil
}

func resolveGeoIPToken(token string, db GeoDatabases) ([]string, error) {
	name := strings.TrimSpace(token[len("geoip:"):])
	if name == "" {
		return nil, fmt.Errorf("empty category")
	}
	if strings.HasPrefix(name, "!") {
		return nil, fmt.Errorf("negated categories are not supported")
	}
	if _, inverted := db.InvertedIPs[name]; inverted {
		return nil, fmt.Errorf("category %q is an inverse match and cannot be expressed", name)
	}
	if db.IPs == nil {
		return nil, fmt.Errorf("geoip database is not loaded")
	}
	cidrs, ok := db.IPs[name]
	if !ok {
		return nil, fmt.Errorf("category %q is not in the geoip database", name)
	}
	return cidrs, nil
}

// dropCoveredBySuffix removes exact hosts already matched by one of the
// suffixes. Suffixes are expected normalized.
func dropCoveredBySuffix(exact, suffixes []string) []string {
	if len(exact) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(suffixes))
	for _, s := range suffixes {
		set[s] = struct{}{}
	}
	out := make([]string, 0, len(exact))
	for _, host := range exact {
		if covered(host, set) {
			continue
		}
		out = append(out, host)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func covered(host string, suffixes map[string]struct{}) bool {
	if _, ok := suffixes[host]; ok {
		return true
	}
	for i := 0; i < len(host); i++ {
		if host[i] != '.' {
			continue
		}
		if _, ok := suffixes[host[i+1:]]; ok {
			return true
		}
	}
	return false
}
