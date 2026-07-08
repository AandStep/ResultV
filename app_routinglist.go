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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

const routingListMaxBytes = 8 * 1024 * 1024

var validRoutingActions = map[string]bool{"proxy": true, "direct": true, "block": true}

// routingListDataDir is the single source of truth for where routing-list
// caches live. In production it resolves to getUserDataPath() ==
// system.UserDataDir(), the exact directory the engine's buildRoute stats via
// proxy.RoutingListCachePath(cfg.DataDir, id) (cfg.DataDir =
// proxy.resultProxyDataDir() = system.UserDataDir()). Tests pin it to a temp
// dir through dataDirOverride so a cache they write is found here.
func (a *App) routingListDataDir() string {
	if a.dataDirOverride != "" {
		return a.dataDirOverride
	}
	return a.getUserDataPath()
}

func newRoutingListID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// removeFileIfExists deletes path, treating "not found" as success.
func removeFileIfExists(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// buildRoutingListSpecs resolves enabled lists that have an existing cache file
// into engine specs. Enabled-but-missing-cache and disabled lists are skipped
// so the engine is never handed a spec whose rule_set file is absent.
func (a *App) buildRoutingListSpecs() []proxy.RoutingListSpec {
	if a.config == nil {
		return nil
	}
	dir := a.routingListDataDir()
	var out []proxy.RoutingListSpec
	for _, rl := range a.config.GetConfig().RoutingRules.RoutingLists {
		if !rl.Enabled || !validRoutingActions[rl.Action] {
			continue
		}
		path := proxy.RoutingListCachePath(dir, rl.ID)
		if _, err := os.Stat(path); err != nil {
			// No cache yet (never fetched, or a prior fetch failed) — omit it
			// rather than feed the engine a dangling path.
			continue
		}
		out = append(out, proxy.RoutingListSpec{
			Tag:    proxy.RoutingListRuleSetTag(rl.ID),
			Path:   path,
			Action: rl.Action,
		})
	}
	return out
}

// syncRoutingListSpecs pushes the current specs to the proxy manager for the
// next engine start/reload. a.proxy is *proxy.Manager, so SetRoutingLists
// (Task 5) is called directly.
func (a *App) syncRoutingListSpecs() {
	if a.proxy == nil {
		return
	}
	a.proxy.SetRoutingLists(a.buildRoutingListSpecs())
}

// fetchRoutingListPayload downloads a routing list under the same SSRF guard as
// subscription/icon fetches (safeImageDialer blocks private/loopback targets).
// Plaintext http:// requires allowInsecure. The body is bounded to
// routingListMaxBytes.
func (a *App) fetchRoutingListPayload(listURL string, allowInsecure bool) ([]byte, error) {
	u := strings.TrimSpace(listURL)
	if u == "" {
		return nil, fmt.Errorf("empty routing-list URL")
	}
	lower := strings.ToLower(u)
	isHTTP := strings.HasPrefix(lower, "http://")
	isHTTPS := strings.HasPrefix(lower, "https://")
	if isHTTP && !allowInsecure {
		return nil, fmt.Errorf("insecure http:// requires explicit consent")
	}
	if !isHTTP && !isHTTPS {
		return nil, fmt.Errorf("unsupported URL scheme")
	}
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{DialContext: safeImageDialer().DialContext},
	}
	req, err := http.NewRequestWithContext(a.ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("routing-list http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, routingListMaxBytes))
	if err != nil {
		return nil, err
	}
	return body, nil
}

// fetchParseAndCache downloads, parses, and writes the cache, returning counts.
// An empty parse result yields an error from WriteRoutingListRuleSet and leaves
// any previous cache intact (nothing is written).
func (a *App) fetchParseAndCache(id, url string, allowInsecure bool) (domains, cidrs int, err error) {
	body, err := a.fetchRoutingListPayload(url, allowInsecure)
	if err != nil {
		return 0, 0, err
	}
	parsed := proxy.ParseRoutingListPayload(body)
	if err := proxy.WriteRoutingListRuleSet(a.routingListDataDir(), id, parsed); err != nil {
		return 0, 0, err
	}
	return len(parsed.Domains), len(parsed.CIDRs), nil
}

// AddRoutingList validates, fetches, caches, persists, and applies a new list.
func (a *App) AddRoutingList(name, url, action string, allowInsecure bool) (config.RoutingList, error) {
	if a.config == nil {
		return config.RoutingList{}, fmt.Errorf("config manager not initialized")
	}
	if !validRoutingActions[action] {
		return config.RoutingList{}, fmt.Errorf("invalid action %q", action)
	}
	rl := config.RoutingList{
		ID:            newRoutingListID(),
		Name:          strings.TrimSpace(name),
		URL:           strings.TrimSpace(url),
		Action:        action,
		Enabled:       true,
		AllowInsecure: allowInsecure,
	}
	dn, cn, err := a.fetchParseAndCache(rl.ID, rl.URL, allowInsecure)
	if err != nil {
		return config.RoutingList{}, err
	}
	rl.DomainCount, rl.CIDRCount, rl.UpdatedAt = dn, cn, time.Now().Unix()

	rr := a.config.GetConfig().RoutingRules
	rr.RoutingLists = append(rr.RoutingLists, rl)
	if err := a.applyRoutingRulesAndReconnect(rr); err != nil {
		return config.RoutingList{}, err
	}
	return rl, nil
}

// UpdateRoutingList edits metadata (name/action/enabled) of an existing list;
// it does not re-fetch (use RefreshRoutingList for that). Fetch-derived fields
// are preserved.
func (a *App) UpdateRoutingList(rl config.RoutingList) error {
	if a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}
	if !validRoutingActions[rl.Action] {
		return fmt.Errorf("invalid action %q", rl.Action)
	}
	rr := a.config.GetConfig().RoutingRules
	found := false
	for i := range rr.RoutingLists {
		if rr.RoutingLists[i].ID == rl.ID {
			// Preserve fetch-derived fields.
			rl.URL = rr.RoutingLists[i].URL
			rl.AllowInsecure = rr.RoutingLists[i].AllowInsecure
			rl.UpdatedAt = rr.RoutingLists[i].UpdatedAt
			rl.DomainCount = rr.RoutingLists[i].DomainCount
			rl.CIDRCount = rr.RoutingLists[i].CIDRCount
			rl.LastError = rr.RoutingLists[i].LastError
			rr.RoutingLists[i] = rl
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("routing list %q not found", rl.ID)
	}
	return a.applyRoutingRulesAndReconnect(rr)
}

// DeleteRoutingList removes a list and its cache file.
func (a *App) DeleteRoutingList(id string) error {
	if a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}
	rr := a.config.GetConfig().RoutingRules
	// Build a fresh slice — GetConfig returns a value whose RoutingLists shares
	// the live cache's backing array, so an in-place [:0] compaction would
	// mutate it before the save. A new slice keeps the mutation local.
	out := make([]config.RoutingList, 0, len(rr.RoutingLists))
	for _, rl := range rr.RoutingLists {
		if rl.ID != id {
			out = append(out, rl)
		}
	}
	rr.RoutingLists = out
	_ = removeFileIfExists(proxy.RoutingListCachePath(a.routingListDataDir(), id))
	return a.applyRoutingRulesAndReconnect(rr)
}

// RefreshRoutingList re-fetches one list, updating counts / LastError.
func (a *App) RefreshRoutingList(id string) (config.RoutingList, error) {
	if a.config == nil {
		return config.RoutingList{}, fmt.Errorf("config manager not initialized")
	}
	rr := a.config.GetConfig().RoutingRules
	var target *config.RoutingList
	for i := range rr.RoutingLists {
		if rr.RoutingLists[i].ID == id {
			target = &rr.RoutingLists[i]
			break
		}
	}
	if target == nil {
		return config.RoutingList{}, fmt.Errorf("routing list %q not found", id)
	}
	dn, cn, err := a.fetchParseAndCache(target.ID, target.URL, target.AllowInsecure)
	if err != nil {
		target.LastError = err.Error()
		_ = a.config.UpdateRoutingRules(rr)
		return *target, err
	}
	target.DomainCount, target.CIDRCount, target.UpdatedAt, target.LastError = dn, cn, time.Now().Unix(), ""
	if err := a.applyRoutingRulesAndReconnect(rr); err != nil {
		return config.RoutingList{}, err
	}
	return *target, nil
}

// refreshRoutingListsOnce re-fetches every enabled list, updating the cache
// and per-list counts/LastError for the NEXT connect. It never reconnects:
// sing-box has no in-place route reload, so a live session keeps its
// snapshot and a background list change applies on the user's next
// connect — the same convention startSmartBlockedRefresh/
// refreshSmartBlockedOnce already follow (see app.go).
func (a *App) refreshRoutingListsOnce() {
	if a.config == nil {
		return
	}
	rr := a.config.GetConfig().RoutingRules
	if len(rr.RoutingLists) == 0 {
		return
	}
	changed := false
	for i := range rr.RoutingLists {
		rl := &rr.RoutingLists[i]
		if !rl.Enabled {
			continue
		}
		dn, cn, err := a.fetchParseAndCache(rl.ID, rl.URL, rl.AllowInsecure)
		if err != nil {
			rl.LastError = err.Error()
			if a.log != nil {
				a.log.Warning(fmt.Sprintf("[ROUTING] Не удалось обновить список %q: %v", rl.Name, err))
			}
			continue
		}
		rl.DomainCount, rl.CIDRCount, rl.UpdatedAt, rl.LastError = dn, cn, time.Now().Unix(), ""
		changed = true
	}
	if changed {
		_ = a.config.UpdateRoutingRules(rr)
		a.syncRoutingListSpecs()
	}
}

// startRoutingListRefresh runs an initial refresh (after leftover cleanup, as
// Smart lists do) and then re-refreshes on the configured interval.
func (a *App) startRoutingListRefresh() {
	if a.ctx == nil {
		return
	}
	go func() {
		a.waitForLeftoverCleanup(15 * time.Second)
		a.refreshRoutingListsOnce()

		hours := a.config.GetConfig().Settings.EffectiveRoutingListUpdateHours()
		ticker := time.NewTicker(time.Duration(hours) * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				a.refreshRoutingListsOnce()
			}
		}
	}()
}

// applyRoutingRulesAndReconnect persists routing rules, re-syncs specs to the
// manager, and reconnects if currently connected — mirroring the existing
// UpdateRules path but for routing-list mutations. Consolidated so every CRUD
// path behaves identically.
func (a *App) applyRoutingRulesAndReconnect(rr config.RoutingRules) error {
	if err := a.config.UpdateRoutingRules(rr); err != nil {
		return err
	}
	a.syncRoutingListSpecs()
	if a.proxy == nil {
		return nil
	}
	status := a.proxy.GetStatus()
	if !status.IsConnected || status.CurrentProxy == nil {
		return nil
	}
	result := a.proxy.ReconnectWithRoutingRules(
		a.ctx, proxy.RoutingMode(rr.Mode), rr.Whitelist, rr.AppWhitelist, rr.AppForceVPN,
	)
	if !result.Success {
		return fmt.Errorf("%s", result.Message)
	}
	return nil
}
