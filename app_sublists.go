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

// syncSubscriptionRoutingLists reconciles provider-declared routing lists into
// the config. Provider controls composition (add/update/remove); the user
// controls Enabled (never overwritten). Tombstoned URLs are never re-added.
// No-op sync writes nothing and does not reconnect.
func (a *App) syncSubscriptionRoutingLists(subID string, provided []config.RoutingList, disabledURLs []string) error {
	if a.config == nil || subID == "" {
		return nil
	}
	cfg := a.config.GetConfig()
	var tombstones map[string]bool
	subAllowInsecure := false // spec: provider lists inherit the subscription's plaintext consent
	subFound := false
	for i := range cfg.Subscriptions {
		if cfg.Subscriptions[i].ID == subID {
			subFound = true
			subAllowInsecure = cfg.Subscriptions[i].AllowInsecure
			tombstones = make(map[string]bool, len(cfg.Subscriptions[i].RemovedRoutingListURLs))
			for _, u := range cfg.Subscriptions[i].RemovedRoutingListURLs {
				tombstones[u] = true
			}
			break
		}
	}
	if !subFound {
		return nil
	}
	disabled := make(map[string]bool, len(disabledURLs))
	for _, u := range disabledURLs {
		disabled[proxy.NormalizeRoutingListURL(u)] = true
	}
	providedByURL := make(map[string]config.RoutingList, len(provided))
	for _, p := range provided {
		providedByURL[p.URL] = p // URLs already normalized by the parser
	}

	rr := cfg.RoutingRules
	merged := make([]config.RoutingList, 0, len(rr.RoutingLists)+len(provided))
	existingByURL := make(map[string]bool)
	changed := false
	var needFetch []string // IDs to (re)download

	for _, rl := range rr.RoutingLists {
		if rl.SubscriptionID != subID {
			merged = append(merged, rl)
			continue
		}
		p, still := providedByURL[rl.URL]
		if !still {
			_ = removeFileIfExists(proxy.RoutingListCachePath(a.routingListDataDir(), rl.ID))
			changed = true
			continue
		}
		existingByURL[rl.URL] = true
		if rl.Name != p.Name || rl.Action != p.Action {
			rl.Name, rl.Action = p.Name, p.Action
			changed = true
		}
		merged = append(merged, rl)
	}
	for _, p := range provided {
		if existingByURL[p.URL] || tombstones[p.URL] {
			continue
		}
		nl := config.RoutingList{
			ID: newRoutingListID(), SubscriptionID: subID,
			Name: p.Name, URL: p.URL, Action: p.Action,
			Enabled:       !disabled[p.URL],
			AllowInsecure: subAllowInsecure,
		}
		merged = append(merged, nl)
		needFetch = append(needFetch, nl.ID)
		changed = true
	}
	if !changed {
		return nil
	}
	// Download content for new entries before persisting counts; a failed
	// download keeps the entry with LastError — the import must not fail.
	for i := range merged {
		for _, id := range needFetch {
			if merged[i].ID != id {
				continue
			}
			dn, cn, err := a.fetchParseAndCache(merged[i].ID, merged[i].URL, merged[i].AllowInsecure)
			if err != nil {
				merged[i].LastError = err.Error()
			} else {
				merged[i].DomainCount, merged[i].CIDRCount = dn, cn
				merged[i].UpdatedAt = time.Now().Unix()
				merged[i].LastError = ""
			}
		}
	}
	rr.RoutingLists = merged
	return a.applyRoutingRulesAndReconnect(rr)
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
