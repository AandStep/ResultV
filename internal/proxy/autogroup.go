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
	"strings"
	"unicode"
	"unicode/utf8"

	"resultproxy-wails/internal/config"
)

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

// AutoGroup is one auto-routing pool: a display name and the member entries
// the app picks between.
type AutoGroup struct {
	Name    string
	Members []config.ProxyEntry
}

// SplitAutoEntries separates auto-routing pools from ordinary servers.
//
// Two strategies, in order:
//
//  1. Structural — entries carrying AutoGroup, stamped by the parser from a
//     provider-declared xray balancer. Authoritative, language-agnostic, and
//     the only one that can express more than one pool.
//  2. Name heuristic — the legacy fallback for subscriptions that arrive as a
//     list of vless:// lines, where no structure exists to read. Capped at
//     one pool, exactly as before.
//
// A structural pool is kept even at a single member: the provider explicitly
// declared a balancer, so that node is an auto section, not a server card.
// The name heuristic still needs two, because one name proves nothing.
func SplitAutoEntries(entries []config.ProxyEntry) (groups []AutoGroup, individual []config.ProxyEntry, ok bool) {
	if len(entries) == 0 {
		return nil, nil, false
	}
	if g, indv, found := splitAutoEntriesByGroupKey(entries); found {
		return g, indv, true
	}
	return splitAutoEntriesByName(entries)
}

func splitAutoEntriesByGroupKey(entries []config.ProxyEntry) ([]AutoGroup, []config.ProxyEntry, bool) {
	order := make([]string, 0, 4)
	byKey := make(map[string][]config.ProxyEntry, 4)
	var individual []config.ProxyEntry
	for _, e := range entries {
		key := strings.TrimSpace(e.AutoGroup)
		if key == "" {
			individual = append(individual, e)
			continue
		}
		if _, seen := byKey[key]; !seen {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], e)
	}
	if len(order) == 0 {
		return nil, nil, false
	}
	groups := make([]AutoGroup, 0, len(order))
	for _, key := range order {
		groups = append(groups, AutoGroup{Name: key, Members: byKey[key]})
	}
	return groups, individual, true
}

func splitAutoEntriesByName(entries []config.ProxyEntry) ([]AutoGroup, []config.ProxyEntry, bool) {
	var autoEntries, individual []config.ProxyEntry
	for _, e := range entries {
		_, base := StripLeadingFlagEmoji(e.Name)
		if containsWordAuto(base) {
			autoEntries = append(autoEntries, e)
		} else {
			individual = append(individual, e)
		}
	}
	if len(autoEntries) < 2 {
		return nil, entries, false
	}
	name, ok := ExtractAutoGroupName(autoEntries)
	if !ok {
		return nil, entries, false
	}
	return []AutoGroup{{Name: name, Members: autoEntries}}, individual, true
}

// containsWordAuto checks whether s contains "auto" (Latin) or "авто"
// (Cyrillic) as a case-insensitive whole word — "autostart" and "Автобан"
// must not match.
//
// The boundary test runs on runes. The byte-wise predecessor compared
// low[end] against 'a'..'z', which can only ever see the FIRST byte of a
// multi-byte rune: for a Cyrillic name it inspected half a letter and the
// whole check was meaningless.
//
// Every occurrence is examined, not just the first: "Автобан Авто" is an
// auto group even though its leading match is not a word.
func containsWordAuto(s string) bool {
	low := strings.ToLower(s)
	for _, word := range []string{"auto", "авто"} {
		for offset := 0; offset < len(low); {
			idx := strings.Index(low[offset:], word)
			if idx < 0 {
				break
			}
			end := offset + idx + len(word)
			if end >= len(low) {
				return true
			}
			next, _ := utf8.DecodeRuneInString(low[end:])
			if !unicode.IsLetter(next) {
				return true
			}
			offset = end
		}
	}
	return false
}
