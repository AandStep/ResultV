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
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

const (
	routingListMaxBytes = 8 * 1024 * 1024
	// A routing-list host (often a Fastly/GitHub CDN edge) occasionally stalls
	// before sending response headers. Retry a few times with a short per-
	// attempt timeout so a one-off stall recovers on the next try in ~1-2s
	// instead of failing the whole add after one long wait.
	routingListFetchAttempts  = 3
	routingListAttemptTimeout = 8 * time.Second
)

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
	// Rewrite a GitHub blob (web page) URL to its raw form so an ordinary
	// github.com link fetches the file, not the HTML page. Idempotent, so it
	// also fixes blob URLs stored by an older build on refresh.
	u := proxy.NormalizeRoutingListURL(strings.TrimSpace(listURL))
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
		Timeout:   routingListAttemptTimeout,
		Transport: &http.Transport{DialContext: safeImageDialer().DialContext},
	}
	var lastErr error
	for attempt := 1; attempt <= routingListFetchAttempts; attempt++ {
		if a.ctx != nil && a.ctx.Err() != nil {
			return nil, a.ctx.Err()
		}
		body, status, err := tryFetchRoutingList(a.ctx, client, u)
		if err == nil && status == http.StatusOK {
			return body, nil
		}
		if err != nil {
			lastErr = err
			continue
		}
		lastErr = fmt.Errorf("routing-list http %d", status)
		// A definitive client error (not found / auth / gone) won't fix itself
		// on retry — stop early. Retry only on 5xx and 429.
		if status >= 400 && status < 500 && status != http.StatusTooManyRequests {
			break
		}
	}
	return nil, lastErr
}

// tryFetchRoutingList performs one GET attempt, returning the body (on 200),
// the HTTP status, and any transport error.
func tryFetchRoutingList(ctx context.Context, client *http.Client, u string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, routingListMaxBytes))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// fetchParseAndCache downloads, parses, and writes the cache, returning counts.
// An empty parse result yields an error from WriteRoutingListRuleSet and leaves
// any previous cache intact (nothing is written).
func (a *App) fetchParseAndCache(id, url string, allowInsecure bool) (domains, cidrs int, err error) {
	body, err := a.fetchRoutingListPayload(url, allowInsecure)
	if err != nil {
		return 0, 0, err
	}
	if proxy.LooksLikeRoutingListHTML(body) {
		return 0, 0, fmt.Errorf("ссылка вернула веб-страницу (HTML), а не список — для GitHub используйте ссылку Raw")
	}
	parsed := proxy.ParseRoutingListPayload(body)
	if len(parsed.Domains) == 0 && len(parsed.CIDRs) == 0 {
		return 0, 0, fmt.Errorf("в списке не найдено доменов или подсетей — проверьте, что ссылка ведёт на список доменов/CIDR или sing-box rule-set (JSON)")
	}
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
		URL:           proxy.NormalizeRoutingListURL(strings.TrimSpace(url)),
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
	next := make([]config.RoutingList, len(rr.RoutingLists), len(rr.RoutingLists)+1)
	copy(next, rr.RoutingLists)
	rr.RoutingLists = append(next, rl)
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
	lists := make([]config.RoutingList, len(rr.RoutingLists))
	copy(lists, rr.RoutingLists)
	found := false
	for i := range lists {
		if lists[i].ID == rl.ID {
			// Preserve fetch-derived fields.
			rl.URL = lists[i].URL
			rl.AllowInsecure = lists[i].AllowInsecure
			rl.UpdatedAt = lists[i].UpdatedAt
			rl.DomainCount = lists[i].DomainCount
			rl.CIDRCount = lists[i].CIDRCount
			rl.LastError = lists[i].LastError
			lists[i] = rl
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("routing list %q not found", rl.ID)
	}
	rr.RoutingLists = lists
	return a.applyRoutingRulesAndReconnect(rr)
}

// DeleteRoutingList removes a list and its cache file.
func (a *App) DeleteRoutingList(id string) error {
	if a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}
	if id == "" || filepath.Base(id) != id {
		return fmt.Errorf("invalid routing list id")
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
	var url string
	var insecure bool
	found := false
	for _, rl := range a.config.GetConfig().RoutingRules.RoutingLists {
		if rl.ID == id {
			url, insecure, found = rl.URL, rl.AllowInsecure, true
			break
		}
	}
	if !found {
		return config.RoutingList{}, fmt.Errorf("routing list %q not found", id)
	}
	dn, cn, ferr := a.fetchParseAndCache(id, url, insecure)

	rr := a.config.GetConfig().RoutingRules
	merged := make([]config.RoutingList, len(rr.RoutingLists))
	copy(merged, rr.RoutingLists)
	var updated config.RoutingList
	stillThere := false
	for i := range merged {
		if merged[i].ID != id {
			continue
		}
		stillThere = true
		if ferr != nil {
			merged[i].LastError = ferr.Error()
		} else {
			merged[i].DomainCount, merged[i].CIDRCount = dn, cn
			merged[i].UpdatedAt = time.Now().Unix()
			merged[i].LastError = ""
		}
		updated = merged[i]
		break
	}
	if !stillThere {
		return config.RoutingList{}, fmt.Errorf("routing list %q not found", id)
	}
	rr.RoutingLists = merged
	if ferr != nil {
		_ = a.config.UpdateRoutingRules(rr)
		return updated, ferr
	}
	if err := a.applyRoutingRulesAndReconnect(rr); err != nil {
		return config.RoutingList{}, err
	}
	return updated, nil
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
	type fetchJob struct {
		id, url  string
		insecure bool
	}
	var jobs []fetchJob
	for _, rl := range a.config.GetConfig().RoutingRules.RoutingLists {
		if rl.Enabled {
			jobs = append(jobs, fetchJob{rl.ID, rl.URL, rl.AllowInsecure})
		}
	}
	if len(jobs) == 0 {
		return
	}
	type fetchResult struct {
		dn, cn  int
		lastErr string
		ok      bool
	}
	results := make(map[string]fetchResult, len(jobs))
	for _, j := range jobs {
		dn, cn, err := a.fetchParseAndCache(j.id, j.url, j.insecure)
		if err != nil {
			results[j.id] = fetchResult{lastErr: err.Error()}
			if a.log != nil {
				a.log.Warning(fmt.Sprintf("[ROUTING] Не удалось обновить список %q: %v", j.url, err))
			}
			continue
		}
		results[j.id] = fetchResult{dn: dn, cn: cn, ok: true}
	}
	// Re-read fresh and merge by id into a NEW slice so a concurrent
	// Add/Delete during the fetches is not overwritten.
	rr := a.config.GetConfig().RoutingRules
	merged := make([]config.RoutingList, len(rr.RoutingLists))
	copy(merged, rr.RoutingLists)
	changed := false
	for i := range merged {
		r, found := results[merged[i].ID]
		if !found {
			continue
		}
		if r.ok {
			merged[i].DomainCount, merged[i].CIDRCount = r.dn, r.cn
			merged[i].UpdatedAt = time.Now().Unix()
			merged[i].LastError = ""
		} else {
			merged[i].LastError = r.lastErr
		}
		changed = true
	}
	if changed {
		rr.RoutingLists = merged
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
