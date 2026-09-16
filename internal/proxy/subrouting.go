// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// Folding everything a subscription says about routing into ONE profile.
//
// A subscription used to arrive as a handful of separate routing lists, one per
// action. That was never how a user thinks about it: routing from a provider
// and routing from a link are the same thing — a set of rules that is either in
// force or not — and only one such set is in force at a time. So the provider's
// direct/proxy/block rules become one editable profile, exactly like the one a
// deep link brings. Ported from the desktop's syncSubscriptionRoutingProfile,
// minus its store.
//
// Inline rules (the provider's embedded xray routing) become tokens; rules the
// provider only linked to stay links in ListURLs and are fetched at compile
// time — inlining a 74k-entry list would bloat the stored file past usefulness.

import (
	"strings"
	"time"

	"resultproxy-wails/internal/config"
)

// BuildSubscriptionRoutingProfile turns a subscription response into a profile.
//
// Returns ok=false when the provider declared no routing at all. An empty
// profile is worse than none: it would take a row in the list, offer to be
// switched on, and route nothing.
func BuildSubscriptionRoutingProfile(
	subID, subName string,
	allowInsecure bool,
	headerVal, body string,
) (config.RoutingProfile, bool) {
	name := strings.TrimSpace(subName)
	if name == "" {
		// A profile with no handle cannot be matched on the next sync —
		// SameRoutingProfile falls back to the displayed name, and an empty one
		// matches every other nameless profile. The id is not pretty, but it is
		// stable, which is the whole job of a handle.
		name = subID
	}
	p := config.RoutingProfile{
		Name:           name,
		OriginName:     name,
		Source:         "subscription",
		SubscriptionID: subID,
		AllowInsecure:  allowInsecure,
		UpdatedAt:      time.Now().Unix(),
		ListURLs:       map[string][]string{},
	}

	// Rules the provider linked to: kept as links.
	for _, decl := range ExtractSubscriptionRoutingLists(headerVal, body) {
		if strings.HasPrefix(decl.URL, "embedded:") {
			continue // handled below, straight from the body
		}
		p.ListURLs[decl.Action] = append(p.ListURLs[decl.Action], decl.URL)
	}

	// Rules the provider inlined into its xray config: become tokens.
	for _, action := range RoutingActions {
		parsed, ok := ExtractEmbeddedRoutingLists(body)[action]
		if !ok {
			continue
		}
		addSubscriptionTokens(&p, action, parsed)
	}

	if len(p.ListURLs) == 0 {
		p.ListURLs = nil
	}
	if p.RuleCount("direct")+p.RuleCount("proxy")+p.RuleCount("block") == 0 {
		return config.RoutingProfile{}, false
	}
	return p, true
}

// addSubscriptionTokens appends inline rules to the right pair of fields.
func addSubscriptionTokens(p *config.RoutingProfile, action string, parsed ParsedRoutingList) {
	// Exact hosts join the suffix list: the profile model has one field per
	// action, and the distinction is re-derived at compile time by
	// ResolveGeoTokens, which reads a bare host as "the host and its
	// sub-domains" — the same reading the embedded-xray parser already applied
	// when it produced these.
	domains := append(append([]string{}, parsed.Domains...), parsed.ExactDomains...)
	switch action {
	case "direct":
		p.DirectSites = append(p.DirectSites, domains...)
		p.DirectIPs = append(p.DirectIPs, parsed.CIDRs...)
	case "proxy":
		p.ProxySites = append(p.ProxySites, domains...)
		p.ProxyIPs = append(p.ProxyIPs, parsed.CIDRs...)
	case "block":
		p.BlockSites = append(p.BlockSites, domains...)
		p.BlockIPs = append(p.BlockIPs, parsed.CIDRs...)
	}
}
