// Copyright (C) 2026 ResultV — GPL-3.0 (see file headers elsewhere).

package proxy

import (
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AdguardTeam/urlfilter/rules"
)

func buildCosmeticIndexFromLines(t *testing.T, lines ...string) *CosmeticIndex {
	t.Helper()
	dir := t.TempDir()
	list := filepath.Join(dir, "list.txt")
	if err := os.WriteFile(list, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing list: %v", err)
	}
	ix, err := BuildCosmeticIndex(map[rules.ListID]string{1: list})
	if err != nil {
		t.Fatalf("BuildCosmeticIndex: %v", err)
	}
	return ix
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}

// The core bug: urlfilter keys host-specific rules by exact hostname, so an
// `interfax.ru##...` rule never fires on `www.interfax.ru`. The index must
// walk labels so the parent-domain rule applies to the subdomain.
func TestCosmeticIndex_SpecificRuleMatchesSubdomain(t *testing.T) {
	ix := buildCosmeticIndexFromLines(t,
		`interfax.ru##.toplogo__advertisement-wrap`,
		`interfax.ru##div[id*="_ban"]`,
	)

	specific, _, _ := ix.Match("www.interfax.ru")
	if !contains(specific, `.toplogo__advertisement-wrap`) {
		t.Fatalf("subdomain www.interfax.ru missed parent-domain rule; got %v", specific)
	}
	if !contains(specific, `div[id*="_ban"]`) {
		t.Fatalf("subdomain missed second rule; got %v", specific)
	}
}

// Generic `##` rules are served by urlfilter's own engine — the index must NOT
// re-emit them (that would double every generic selector).
func TestCosmeticIndex_SkipsGenericElementHiding(t *testing.T) {
	ix := buildCosmeticIndexFromLines(t, `##.generic-banner`)
	specific, genExt, specExt := ix.Match("example.org")
	if len(specific)+len(genExt)+len(specExt) != 0 {
		t.Fatalf("generic ## must be left to the engine; got specific=%v", specific)
	}
}

// ExtCSS (`#?#`) is rejected by urlfilter's parser entirely, so the index owns
// it — both generic and host-specific, including subdomain matching.
func TestCosmeticIndex_ExtCSS(t *testing.T) {
	ix := buildCosmeticIndexFromLines(t,
		`#?#.promo:has(> .ad)`,
		`mail.ru#?#div:has(> a[href*="/ads/"])`,
	)

	_, genExt, specExt := ix.Match("e.mail.ru")
	if !contains(genExt, `.promo:has(> .ad)`) {
		t.Fatalf("generic ExtCSS missing; got %v", genExt)
	}
	if !contains(specExt, `div:has(> a[href*="/ads/"])`) {
		t.Fatalf("specific ExtCSS missed on subdomain; got %v", specExt)
	}
}

// An `#@#` exception cancels the matching `##` rule on that host.
func TestCosmeticIndex_ExceptionCancels(t *testing.T) {
	ix := buildCosmeticIndexFromLines(t,
		`interfax.ru##.banner`,
		`www.interfax.ru#@#.banner`,
	)
	specific, _, _ := ix.Match("www.interfax.ru")
	if contains(specific, `.banner`) {
		t.Fatalf("#@# exception should have cancelled the rule; got %v", specific)
	}
}

// End to end: buildContentScript must merge the index's specific match for a
// www subdomain into the payload's elementHiding.specific array, proving the
// wiring closes the subdomain gap that the engine alone leaves open.
func TestBuildContentScript_MergesCosmeticIndexForSubdomain(t *testing.T) {
	// urlfilter's engine keeps its list files open; on Windows t.TempDir's
	// RemoveAll then fails even though the assertions pass (same workaround as
	// TestContentScript_ScriptletRuleFlowsFromListToPayload). Cleanup is
	// best-effort.
	dir, err := os.MkdirTemp("", "cosmetic-merge-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	emptyList := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(emptyList, []byte(""), 0o600); err != nil {
		t.Fatalf("writing empty list: %v", err)
	}
	eng, err := BuildEngine(map[rules.ListID]string{1: emptyList})
	if err != nil {
		t.Fatalf("BuildEngine: %v", err)
	}

	ix := buildCosmeticIndexFromLines(t, `interfax.ru##.toplogo__advertisement-wrap`)

	s := testServer()
	s.engine = eng
	s.cosmeticIndex = ix

	// hostname is the www subdomain — the engine returns nothing for it.
	url := fmt.Sprintf("http://%s/content-script.js?hostname=www.interfax.ru&option=%d&ts=%d",
		s.InjectionHost, rules.CosmeticOptionAll, s.createdAt.Unix())
	req := httptest.NewRequest("GET", url, nil)
	httpRes := s.buildContentScript(NewSession("t-cosmetic-sub", req))
	if httpRes.StatusCode != 200 {
		t.Fatalf("want 200, got %d", httpRes.StatusCode)
	}
	body, err := io.ReadAll(httpRes.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if !strings.Contains(string(body), `toplogo__advertisement-wrap`) {
		t.Fatal("content script for www subdomain missing merged specific rule")
	}
}
