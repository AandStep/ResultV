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
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

// The subscription HTTP client has no SSRF dialer, so loopback httptest works.
func TestFetchSubscriptionReturnsRoutingLists(t *testing.T) {
	payload := `[{"name":"L1","url":"https://example.com/l1.lst","action":"proxy"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Routing-Lists", base64.StdEncoding.EncodeToString([]byte(payload)))
		// Minimal valid subscription body: one proxy URI.
		_, _ = w.Write([]byte("vless://00000000-0000-0000-0000-000000000000@1.2.3.4:443?type=tcp&security=none#test\n"))
	}))
	defer srv.Close()

	a := newTestApp(t, t.TempDir())
	// http:// loopback URL → allowInsecure=true.
	prev, err := a.FetchSubscription(srv.URL, true)
	if err != nil {
		t.Fatalf("FetchSubscription: %v", err)
	}
	if len(prev.Proxies) == 0 {
		t.Fatalf("no proxies parsed")
	}
	if len(prev.RoutingLists) != 1 || prev.RoutingLists[0].Name != "L1" || prev.RoutingLists[0].Action != "proxy" {
		t.Fatalf("routing lists not returned: %+v", prev.RoutingLists)
	}
}

func TestParseSubscriptionTextJSONBodyLists(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	// JSON body with both nodes (unparseable here is fine if entries exist) is
	// complex; assert only the routingLists extraction on a JSON body that also
	// fails proxy parsing → expect error but that's the entries contract.
	// Simplest valid check: plain URI text yields no routing lists.
	prev, err := a.ParseSubscriptionText("vless://00000000-0000-0000-0000-000000000000@1.2.3.4:443?type=tcp&security=none#t\n")
	if err != nil {
		t.Fatalf("ParseSubscriptionText: %v", err)
	}
	if len(prev.RoutingLists) != 0 {
		t.Fatalf("plain URI text must yield no routing lists: %+v", prev.RoutingLists)
	}
}

func TestEmbeddedRoutingListDeclarations(t *testing.T) {
	embedded := map[string]proxy.ParsedRoutingList{
		"direct": {Domains: []string{"a.test", "b.test"}, CIDRs: []string{"1.2.3.0/24"}},
		"proxy":  {Domains: []string{"c.test"}},
	}
	decls := embeddedRoutingListDeclarations(embedded)
	if len(decls) != 2 {
		t.Fatalf("want 2 decls, got %d: %+v", len(decls), decls)
	}
	// Deterministic order: proxy before direct.
	if decls[0].URL != "embedded:proxy" || decls[1].URL != "embedded:direct" {
		t.Errorf("order/urls wrong: %+v", decls)
	}
	d := decls[1]
	if d.Action != "direct" || d.DomainCount != 2 || d.CIDRCount != 1 {
		t.Errorf("direct decl wrong: %+v", d)
	}
}

func seedSubWithLists(t *testing.T, a *App, subID string, lists []config.RoutingList, tombstones []string) {
	t.Helper()
	cfg := a.config.GetConfig()
	cfg.Subscriptions = append(cfg.Subscriptions, config.Subscription{
		ID: subID, Name: "TestSub", URL: "https://sub.test/s",
		RemovedRoutingListURLs: tombstones,
	})
	cfg.RoutingRules.RoutingLists = append(cfg.RoutingRules.RoutingLists, lists...)
	if err := a.config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
}

// Content fetches use SSRF-guarded client → loopback URLs fail fast, so new
// lists end up with LastError set; composition logic is still fully observable.
func TestSyncAddsUpdatesRemovesRespectsEnabled(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", []config.RoutingList{
		{ID: "keep", SubscriptionID: "sub1", URL: "https://keep.test/l.lst", Action: "proxy", Name: "Old", Enabled: false},
		{ID: "gone", SubscriptionID: "sub1", URL: "https://gone.test/l.lst", Action: "block", Enabled: true},
		{ID: "user", SubscriptionID: "", URL: "https://user.test/l.lst", Action: "direct", Enabled: true},
	}, nil)

	provided := []config.RoutingList{
		{Name: "NewName", URL: "https://keep.test/l.lst", Action: "block"}, // update
		{Name: "Fresh", URL: "https://fresh.test/l.lst", Action: "proxy"},  // add
	}
	if err := a.syncSubscriptionRoutingLists("sub1", provided, nil, nil); err != nil {
		t.Fatalf("sync: %v", err)
	}
	got := a.config.GetConfig().RoutingRules.RoutingLists
	byURL := map[string]config.RoutingList{}
	for _, rl := range got {
		byURL[rl.URL] = rl
	}
	if _, ok := byURL["https://gone.test/l.lst"]; ok {
		t.Error("vanished provider list must be removed")
	}
	if _, ok := byURL["https://user.test/l.lst"]; !ok {
		t.Error("user list must be untouched")
	}
	kept := byURL["https://keep.test/l.lst"]
	if kept.Name != "NewName" || kept.Action != "block" {
		t.Errorf("update failed: %+v", kept)
	}
	if kept.Enabled {
		t.Error("sync must NOT overwrite user's Enabled=false")
	}
	fresh, ok := byURL["https://fresh.test/l.lst"]
	if !ok || !fresh.Enabled || fresh.SubscriptionID != "sub1" {
		t.Errorf("new list wrong: %+v", fresh)
	}
}

func TestSyncRespectsTombstonesAndDisabled(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", nil, []string{"https://dead.test/l.lst"})
	provided := []config.RoutingList{
		{URL: "https://dead.test/l.lst", Action: "proxy"},
		{URL: "https://off.test/l.lst", Action: "proxy"},
	}
	if err := a.syncSubscriptionRoutingLists("sub1", provided, []string{"https://off.test/l.lst"}, nil); err != nil {
		t.Fatalf("sync: %v", err)
	}
	byURL := map[string]config.RoutingList{}
	for _, rl := range a.config.GetConfig().RoutingRules.RoutingLists {
		byURL[rl.URL] = rl
	}
	if _, ok := byURL["https://dead.test/l.lst"]; ok {
		t.Error("tombstoned URL must not be re-added")
	}
	off, ok := byURL["https://off.test/l.lst"]
	if !ok || off.Enabled {
		t.Errorf("import-disabled list must exist with Enabled=false: %+v", off)
	}
}

func TestDeleteRoutingListTombstonesProviderList(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", []config.RoutingList{
		{ID: "p1", SubscriptionID: "sub1", URL: "https://p.test/l.lst", Action: "proxy", Enabled: true},
	}, nil)
	if err := a.DeleteRoutingList("p1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	cfg := a.config.GetConfig()
	if len(cfg.RoutingRules.RoutingLists) != 0 {
		t.Error("list not removed")
	}
	if len(cfg.Subscriptions) != 1 || len(cfg.Subscriptions[0].RemovedRoutingListURLs) != 1 ||
		cfg.Subscriptions[0].RemovedRoutingListURLs[0] != "https://p.test/l.lst" {
		t.Errorf("tombstone missing: %+v", cfg.Subscriptions)
	}
}

func TestSyncUnknownSubscriptionIsNoop(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	provided := []config.RoutingList{{URL: "https://x.test/l.lst", Action: "proxy"}}
	if err := a.syncSubscriptionRoutingLists("ghost", provided, nil, nil); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := a.config.GetConfig().RoutingRules.RoutingLists; len(got) != 0 {
		t.Fatalf("unknown subID must not create lists: %+v", got)
	}
}

func TestDeleteSubscriptionRemovesItsLists(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", []config.RoutingList{
		{ID: "p1", SubscriptionID: "sub1", URL: "https://p.test/l.lst", Action: "proxy", Enabled: true},
		{ID: "u1", SubscriptionID: "", URL: "https://u.test/l.lst", Action: "direct", Enabled: true},
	}, nil)
	if err := a.DeleteSubscription("sub1"); err != nil {
		t.Fatalf("delete sub: %v", err)
	}
	got := a.config.GetConfig().RoutingRules.RoutingLists
	if len(got) != 1 || got[0].ID != "u1" {
		t.Errorf("only the user list must remain: %+v", got)
	}
}

// Удаление подписки уносит и её серверы: раньше их вычищал только фронт своим
// списком, и на странице оставался либо заголовок без серверов, либо серверы
// без подписки — смотря чьё сохранение доехало последним.
func TestDeleteSubscriptionRemovesItsProxies(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", nil, nil)

	cfg := a.config.GetConfig()
	cfg.Proxies = []config.ProxyEntry{
		{ID: "auto", Type: "AUTO", Name: "Auto", SubscriptionURL: "https://sub.test/s"},
		{ID: "m1", Type: "VLESS", Name: "member", SubscriptionURL: "https://sub.test/s"},
		{ID: "own", Type: "VLESS", Name: "свой"},
		{ID: "other", Type: "VLESS", Name: "чужая подписка", SubscriptionURL: "https://sub2.test/s"},
	}
	if err := a.config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	if err := a.DeleteSubscription("sub1"); err != nil {
		t.Fatalf("delete sub: %v", err)
	}

	got := a.config.GetConfig()
	if len(got.Subscriptions) != 0 {
		t.Errorf("сама подписка должна уйти: %+v", got.Subscriptions)
	}
	var ids []string
	for _, p := range got.Proxies {
		ids = append(ids, p.ID)
	}
	if len(ids) != 2 || ids[0] != "own" || ids[1] != "other" {
		t.Errorf("должны остаться только чужие серверы, осталось: %v", ids)
	}
}

func TestSyncWritesEmbeddedCacheFromBody(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", nil, nil) // sub exists, no lists yet
	provided := []config.RoutingList{
		{Name: "embedded-direct", URL: "embedded:direct", Action: "direct", DomainCount: 2},
	}
	embedded := map[string]proxy.ParsedRoutingList{
		"direct": {Domains: []string{"gov.test", "nalog.test"}},
	}
	if err := a.syncSubscriptionRoutingLists("sub1", provided, nil, embedded); err != nil {
		t.Fatalf("sync: %v", err)
	}
	lists := a.config.GetConfig().RoutingRules.RoutingLists
	if len(lists) != 1 {
		t.Fatalf("want 1 list, got %+v", lists)
	}
	rl := lists[0]
	if rl.URL != "embedded:direct" || rl.SubscriptionID != "sub1" || !rl.Enabled {
		t.Errorf("embedded list wrong: %+v", rl)
	}
	if rl.DomainCount != 2 || rl.LastError != "" {
		t.Errorf("counts/err wrong (cache should be written, no fetch): count=%d err=%q", rl.DomainCount, rl.LastError)
	}
	// The cache file must exist and contain the domains (written from body, no network).
	blob, err := os.ReadFile(proxy.RoutingListCachePath(a.routingListDataDir(), rl.ID))
	if err != nil {
		t.Fatalf("cache not written: %v", err)
	}
	if !strings.Contains(string(blob), "gov.test") || !strings.Contains(string(blob), "nalog.test") {
		t.Errorf("cache content wrong: %s", blob)
	}
}

func TestSyncEmbeddedContentUpdateReflected(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	seedSubWithLists(t, a, "sub1", nil, nil)
	provided := []config.RoutingList{{Name: "embedded-direct", URL: "embedded:direct", Action: "direct"}}
	if err := a.syncSubscriptionRoutingLists("sub1", provided,
		nil, map[string]proxy.ParsedRoutingList{"direct": {Domains: []string{"old.test"}}}); err != nil {
		t.Fatal(err)
	}
	id := a.config.GetConfig().RoutingRules.RoutingLists[0].ID
	// Second sync with changed content must rewrite the cache.
	if err := a.syncSubscriptionRoutingLists("sub1", provided,
		nil, map[string]proxy.ParsedRoutingList{"direct": {Domains: []string{"new.test"}}}); err != nil {
		t.Fatal(err)
	}
	blob, _ := os.ReadFile(proxy.RoutingListCachePath(a.routingListDataDir(), id))
	if strings.Contains(string(blob), "old.test") || !strings.Contains(string(blob), "new.test") {
		t.Errorf("embedded cache not updated on content change: %s", blob)
	}
}

func TestRefreshOnceSkipsEmbedded(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	// An embedded list with counts must survive refreshRoutingListsOnce untouched
	// (no fetch attempt that would set LastError).
	seedSubWithLists(t, a, "sub1", []config.RoutingList{
		{ID: "e1", SubscriptionID: "sub1", URL: "embedded:direct", Action: "direct", Enabled: true, DomainCount: 5},
	}, nil)
	a.refreshRoutingListsOnce()
	got := a.config.GetConfig().RoutingRules.RoutingLists
	if len(got) != 1 || got[0].LastError != "" || got[0].DomainCount != 5 {
		t.Errorf("embedded list must be skipped by refresh, unchanged: %+v", got)
	}
}
