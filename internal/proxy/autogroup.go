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

import "strings"

// parseBalancerGroup reports whether a subscription config declares an xray
// load balancer, and returns the group's display name plus the outbound-tag
// prefixes that make up its pool.
//
// A config is an auto group iff routing.balancers is non-empty. That is the
// provider's own declaration and it survives renames — impio moved from
// "🚀 impVPN Auto" to a Cyrillic "⚡ Авто | ✅ Когда не глушат интернет" and
// split one group into per-tier sections without touching this structure.
//
// Membership follows xray's rule: selector entries are matched against
// outbound tags with strings.HasPrefix, so "basic-proxy" covers
// basic-proxy-2..38 while "premium-proxy" does NOT cover
// "premium-limit-proxy". fallbackTag joins the pool because the balancer
// routes to it when every probe fails.
//
// burstObservatory.subjectSelector is deliberately ignored: it says which
// outbounds get pinged, not which ones are in the pool.
func parseBalancerGroup(obj map[string]interface{}) (name string, prefixes []string, ok bool) {
	routing, ok := asMap(obj["routing"])
	if !ok {
		return "", nil, false
	}
	balancers, ok := asSlice(routing["balancers"])
	if !ok || len(balancers) == 0 {
		return "", nil, false
	}

	seen := make(map[string]struct{})
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, dup := seen[s]; dup {
			return
		}
		seen[s] = struct{}{}
		prefixes = append(prefixes, s)
	}

	firstTag := ""
	for _, b := range balancers {
		bm, ok := asMap(b)
		if !ok {
			continue
		}
		if firstTag == "" {
			firstTag = strings.TrimSpace(asString(bm["tag"]))
		}
		if sel, ok := asSlice(bm["selector"]); ok {
			for _, s := range sel {
				add(asString(s))
			}
		}
		add(asString(bm["fallbackTag"]))
	}
	if len(prefixes) == 0 {
		// A balancer that selects nothing routes nothing. Treat the config as
		// an ordinary server list instead of publishing an empty AUTO card.
		return "", nil, false
	}

	// The group needs a stable key even when the provider ships no remarks:
	// an empty AutoGroup would be indistinguishable from "not a group".
	name = strings.TrimSpace(asString(obj["remarks"]))
	if name == "" {
		name = firstTag
	}
	if name == "" {
		name = "AUTO"
	}
	return name, prefixes, true
}

// tagMatchesSelector reports whether an outbound tag belongs to a balancer
// pool, using xray's prefix rule.
func tagMatchesSelector(tag string, prefixes []string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false
	}
	for _, p := range prefixes {
		if strings.HasPrefix(tag, p) {
			return true
		}
	}
	return false
}
