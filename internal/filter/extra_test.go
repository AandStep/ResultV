// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/AdguardTeam/urlfilter/rules"

	"resultproxy-wails/internal/filter/mitm"
)

// updateWithOneList downloads a single stub list into m so the StartMITM
// precondition (at least one cached list) holds.
func updateWithOneList(t *testing.T, m *Manager) {
	t.Helper()
	list := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("! test list\n", 50)))
	}))
	defer list.Close()

	orig := DefaultSources
	DefaultSources = []ListSource{{ID: 1, Name: "adguard-base", URLs: []string{list.URL}}}
	defer func() { DefaultSources = orig }()

	if err := m.Update(context.Background(), nil); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
}

func TestMITMFilterPaths_IncludesEmbeddedExtraRules(t *testing.T) {
	m := NewManager(t.TempDir())
	updateWithOneList(t, m)

	paths, err := m.mitmFilterPaths()
	if err != nil {
		t.Fatalf("mitmFilterPaths failed: %v", err)
	}
	p, ok := paths[extraListID]
	if !ok {
		t.Fatalf("expected the embedded extra list under id %d, got %+v", extraListID, paths)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("extra list not written: %v", err)
	}
	if !strings.Contains(string(b), "||pubserv.pro^") {
		t.Fatalf("extra list must block pubserv.pro, got:\n%s", b)
	}
}

func TestMITMFilterPaths_StillRequiresDownloadedLists(t *testing.T) {
	m := NewManager(t.TempDir())
	if _, err := m.mitmFilterPaths(); err == nil {
		t.Fatal("expected an error before any list has been downloaded — the supplement alone must not satisfy StartMITM")
	}
}

// End-to-end through the real urlfilter engine: the ad host caught in the
// device log (psb-dsp.pubserv.pro serving video pre-rolls) must be blocked
// by the engine the MITM proxy runs.
func TestEngine_BlocksCaughtAdHost(t *testing.T) {
	// urlfilter's engine keeps the list files open; on Windows t.TempDir's
	// RemoveAll then fails even though the assertions pass (same artifact as
	// the MITM lifecycle tests). Cleanup is best-effort.
	dir, err := os.MkdirTemp("", "extra-engine-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	m := NewManager(dir)
	updateWithOneList(t, m)

	paths, err := m.mitmFilterPaths()
	if err != nil {
		t.Fatalf("mitmFilterPaths failed: %v", err)
	}
	eng, err := mitm.BuildEngine(paths)
	if err != nil {
		t.Fatalf("BuildEngine failed: %v", err)
	}

	req := rules.NewRequest("https://psb-dsp.pubserv.pro/vast.xml", "https://example.com/watch", rules.TypeXmlhttprequest)
	rule := eng.MatchRequest(req).GetBasicResult()
	if rule == nil || rule.Whitelist {
		t.Fatal("expected psb-dsp.pubserv.pro to be network-blocked by the embedded extra rules")
	}

	// Sanity: an ordinary host must not be blocked by the supplement.
	okReq := rules.NewRequest("https://example.org/index.html", "", rules.TypeDocument)
	if r := eng.MatchRequest(okReq).GetBasicResult(); r != nil && !r.Whitelist {
		t.Fatalf("example.org unexpectedly blocked by %s", r.String())
	}
}
