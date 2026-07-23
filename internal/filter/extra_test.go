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
	for _, rule := range []string{
		"||pubserv.pro^",
		"||foxstreetcore.com^",
		"||ofjvnvjf.win^",
		"||betamountwo.com^",
		"||adultmasters.pro^",
		"||nmsrv.run^",
		"||kintg.site^",
	} {
		if !strings.Contains(string(b), rule) {
			t.Errorf("extra list must contain %s, got:\n%s", rule, b)
		}
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

	for _, url := range []string{
		"https://psb-dsp.pubserv.pro/vast.xml",
		"https://cs11.foxstreetcore.com/content/61951/3484725.jpg",
		"https://ofjvnvjf.win/935a7fcb430a69d5/mbn/ssp/4f2cdc01",
		"https://betamountwo.com/x.js",
		"https://adultmasters.pro/?utm_source=site_example.com",
	} {
		req := rules.NewRequest(url, "https://example.com/watch", rules.TypeXmlhttprequest)
		rule := eng.MatchRequest(req).GetBasicResult()
		if rule == nil || rule.Whitelist {
			t.Errorf("expected %s to be network-blocked by the embedded extra rules", url)
		}
	}

	// Sanity: an ordinary host must not be blocked by the supplement.
	okReq := rules.NewRequest("https://example.org/index.html", "", rules.TypeDocument)
	if r := eng.MatchRequest(okReq).GetBasicResult(); r != nil && !r.Whitelist {
		t.Fatalf("example.org unexpectedly blocked by %s", r.String())
	}
}

// The supplement must collapse leftover empty ad slots — the case where the
// network layer blocks the ad request but the page keeps a reserved-height
// container behind, which is what leaves blank gaps (measured on mail.ru:
// div.ads-above-dzen, 300px tall, truly :empty, covered by no public list).
func TestEmbeddedExtraRules_CollapseEmptyAdSlots(t *testing.T) {
	for _, want := range []string{
		`##div[class^="ads-"]:empty`,
		`##div[class*=" ads-"]:empty`,
		`##div[class*="ads_"]:empty`,
		`mail.ru##.ads-above-dzen`,
	} {
		if !strings.Contains(embeddedExtraRules, want) {
			t.Errorf("embeddedExtraRules missing %q", want)
		}
	}
}

// Every cosmetic line in the supplement must parse — a typo would be dropped
// silently at load time and the slot would keep leaking.
func TestEmbeddedExtraRules_CosmeticLinesParse(t *testing.T) {
	for _, line := range strings.Split(embeddedExtraRules, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "!") || !strings.Contains(line, "##") {
			continue
		}
		r, err := rules.NewCosmeticRule(line, extraListID)
		if err != nil {
			t.Errorf("cosmetic rule %q failed to parse: %v", line, err)
			continue
		}
		if r.Content == "" {
			t.Errorf("cosmetic rule %q parsed with empty content", line)
		}
	}
}

// The site-specific supplement rule must apply to the subdomain the browser
// actually loads, not just the apex.
func TestEmbeddedExtraRules_MailRuSlotMatchesSubdomain(t *testing.T) {
	r, err := rules.NewCosmeticRule(`mail.ru##.ads-above-dzen`, extraListID)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	for _, host := range []string{"mail.ru", "www.mail.ru", "e.mail.ru"} {
		if !r.Match(host) {
			t.Errorf("rule should apply to %s", host)
		}
	}
}
