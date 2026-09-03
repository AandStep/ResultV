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

package main

import (
	"fmt"
	"strings"
	"time"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

// SubscriptionPreview is what the import UI gets from a subscription fetch:
// the nodes plus any provider-declared routing lists (Name/URL/Action only —
// IDs and enabled-state are assigned at AddSubscription time).
type SubscriptionPreview struct {
	Proxies      []config.ProxyEntry  `json:"proxies"`
	RoutingLists []config.RoutingList `json:"routingLists"`
}

// embeddedRoutingListDeclarations synthesizes routing-list declarations for the
// provider's embedded xray routing lists. Each gets the pseudo-URL
// embedded:<action> (its identity within the subscription) and pre-computed
// counts so the import preview can show them before any download. Deterministic
// order so previews and diffs are stable.
func embeddedRoutingListDeclarations(embedded map[string]proxy.ParsedRoutingList) []config.RoutingList {
	out := make([]config.RoutingList, 0, len(embedded))
	for _, action := range []string{"proxy", "direct", "block"} {
		p, ok := embedded[action]
		if !ok {
			continue
		}
		out = append(out, config.RoutingList{
			Name:        "embedded-" + action,
			URL:         "embedded:" + action,
			Action:      action,
			DomainCount: len(p.Domains),
			CIDRCount:   len(p.CIDRs),
		})
	}
	return out
}

// syncSubscriptionRoutingProfile folds everything a subscription says about
// routing into ONE profile.
//
// A subscription used to arrive as a handful of separate routing lists, one per
// action. That was never how a user thinks about it: routing from a provider
// and routing from a link are the same thing — a set of rules that is either in
// force or not — and only one such set is in force at a time. So the provider's
// direct/proxy/block rules become one editable profile, exactly like the one a
// deep link brings.
//
// Inline rules (the provider's embedded xray routing) become tokens; rules the
// provider only links to stay links in ListURLs and are fetched at compile time
// — inlining a 74k-entry list would bloat the config past usefulness.
//
// `activate` marks the profile as the one in force. True when the user just
// said yes to this subscription's routing; false on a background refresh, which
// must not quietly take over from whatever they chose since.
func (a *App) syncSubscriptionRoutingProfile(
	subID string,
	provided []config.RoutingList,
	disabledURLs []string,
	embedded map[string]proxy.ParsedRoutingList,
	activate bool,
) error {
	if a.config == nil || subID == "" {
		return nil
	}
	cfg := a.config.GetConfig()
	var sub *config.Subscription
	for i := range cfg.Subscriptions {
		if cfg.Subscriptions[i].ID == subID {
			sub = &cfg.Subscriptions[i]
			break
		}
	}
	if sub == nil {
		return nil
	}

	disabled := make(map[string]bool, len(disabledURLs))
	for _, u := range disabledURLs {
		disabled[proxy.NormalizeRoutingListURL(u)] = true
	}
	tombstoned := make(map[string]bool, len(sub.RemovedRoutingListURLs))
	for _, u := range sub.RemovedRoutingListURLs {
		tombstoned[u] = true
	}

	profile := config.RoutingProfile{
		Name:           sub.Name,
		OriginName:     sub.Name,
		Source:         "subscription",
		SubscriptionID: subID,
		AllowInsecure:  sub.AllowInsecure,
		UpdatedAt:      time.Now().Unix(),
		ListURLs:       map[string][]string{},
	}

	for _, p := range provided {
		if disabled[p.URL] || tombstoned[p.URL] {
			continue
		}
		if strings.HasPrefix(p.URL, "embedded:") {
			action := strings.TrimPrefix(p.URL, "embedded:")
			parsed, ok := embedded[action]
			if !ok {
				continue
			}
			addProfileTokens(&profile, action, parsed.Domains, parsed.CIDRs)
			continue
		}
		profile.ListURLs[p.Action] = append(profile.ListURLs[p.Action], p.URL)
	}

	total := profile.RuleCount("direct") + profile.RuleCount("proxy") + profile.RuleCount("block")

	// Whatever this subscription used to own as separate lists goes away: the
	// profile replaces them, and leaving both would route by the same rules
	// twice and show them in two places.
	cfgCopy := a.config.GetConfig()
	before := len(cfgCopy.RoutingRules.RoutingLists)
	a.removeSubscriptionRoutingLists(&cfgCopy, subID)
	droppedLegacy := before != len(cfgCopy.RoutingRules.RoutingLists)
	if droppedLegacy {
		if err := a.config.SaveConfig(cfgCopy); err != nil {
			return err
		}
	}

	if total == 0 {
		// The user turned this subscription's routing off, or the provider
		// stopped sending any: drop the profile rather than keep an empty one.
		if err := a.removeSubscriptionRoutingProfile(subID); err != nil {
			return err
		}
		if droppedLegacy {
			return a.applyRoutingRulesAndReconnect(a.config.GetConfig().RoutingRules)
		}
		return nil
	}

	saved, err := a.upsertRoutingProfile(profile, activate)
	if err != nil {
		return err
	}
	if _, cerr := a.compileRoutingProfile(saved, false); cerr != nil {
		a.log.Warning(fmt.Sprintf("Маршрутизация подписки %q сохранена, но правила не собраны: %v", sub.Name, cerr))
	}
	return a.applyRoutingRulesAndReconnect(a.config.GetConfig().RoutingRules)
}

// addProfileTokens appends inline rules to the right pair of fields.
func addProfileTokens(p *config.RoutingProfile, action string, domains, cidrs []string) {
	switch action {
	case "direct":
		p.DirectSites = append(p.DirectSites, domains...)
		p.DirectIPs = append(p.DirectIPs, cidrs...)
	case "proxy":
		p.ProxySites = append(p.ProxySites, domains...)
		p.ProxyIPs = append(p.ProxyIPs, cidrs...)
	case "block":
		p.BlockSites = append(p.BlockSites, domains...)
		p.BlockIPs = append(p.BlockIPs, cidrs...)
	}
}

// removeSubscriptionRoutingProfile drops the profile a subscription owns.
func (a *App) removeSubscriptionRoutingProfile(subID string) error {
	rr := a.config.GetConfig().RoutingRules
	out := make([]config.RoutingProfile, 0, len(rr.Profiles))
	found := false
	for _, p := range rr.Profiles {
		if p.Source == "subscription" && p.SubscriptionID == subID {
			a.removeRoutingProfileRuleSets(p.ID)
			if rr.ActiveProfileID == p.ID {
				rr.ActiveProfileID = ""
			}
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		return nil
	}
	rr.Profiles = out
	return a.config.UpdateRoutingRules(rr)
}

// removeSubscriptionRoutingLists drops every routing list owned by subID from
// cfg (fresh slice) and deletes their cache files. Caller saves cfg.
func (a *App) removeSubscriptionRoutingLists(cfg *config.AppConfig, subID string) {
	if subID == "" {
		return
	}
	out := make([]config.RoutingList, 0, len(cfg.RoutingRules.RoutingLists))
	for _, rl := range cfg.RoutingRules.RoutingLists {
		if rl.SubscriptionID == subID {
			_ = removeFileIfExists(proxy.RoutingListCachePath(a.routingListDataDir(), rl.ID))
			continue
		}
		out = append(out, rl)
	}
	cfg.RoutingRules.RoutingLists = out
}
