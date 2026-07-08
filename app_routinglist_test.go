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
	"testing"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

// newTestApp builds an App wired to a real config.Manager over a temp dir and
// pins the routing-list cache directory to dataDir so buildRoutingListSpecs
// stats the same place the test wrote rule_sets.
func newTestApp(t *testing.T, dataDir string) *App {
	t.Helper()
	a := NewApp()
	a.ctx = context.Background()
	a.dataDirOverride = dataDir
	mgr := config.NewManager(config.NewCryptoServiceWithID("routing-list-test"))
	if err := mgr.Init(t.TempDir()); err != nil {
		t.Fatalf("config init: %v", err)
	}
	a.config = mgr
	return a
}

func TestBuildRoutingListSpecsFiltersDisabledAndMissing(t *testing.T) {
	dir := t.TempDir()
	// One enabled+cached, one enabled+missing-cache, one disabled+cached.
	if err := proxy.WriteRoutingListRuleSet(dir, "ok", proxy.ParsedRoutingList{Domains: []string{"a.test"}}); err != nil {
		t.Fatal(err)
	}
	if err := proxy.WriteRoutingListRuleSet(dir, "off", proxy.ParsedRoutingList{Domains: []string{"b.test"}}); err != nil {
		t.Fatal(err)
	}
	a := newTestApp(t, dir)
	a.config.UpdateRoutingRules(config.RoutingRules{
		Mode: "smart",
		RoutingLists: []config.RoutingList{
			{ID: "ok", Action: "proxy", Enabled: true},
			{ID: "missing", Action: "block", Enabled: true},
			{ID: "off", Action: "direct", Enabled: false},
		},
	})

	specs := a.buildRoutingListSpecs()
	if len(specs) != 1 {
		t.Fatalf("want 1 spec (enabled+cached), got %d: %+v", len(specs), specs)
	}
	if specs[0].Tag != proxy.RoutingListRuleSetTag("ok") || specs[0].Action != "proxy" {
		t.Errorf("unexpected spec: %+v", specs[0])
	}
}

func TestFetchRoutingListRejectsPrivate(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	_, err := a.fetchRoutingListPayload("http://127.0.0.1:1/list.txt", true)
	if err == nil {
		t.Error("expected SSRF rejection for loopback URL")
	}
}
