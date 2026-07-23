// Copyright (C) 2026 ResultV — GPL-3.0 (see file headers elsewhere).

package proxy

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"text/template"

	"github.com/AdguardTeam/urlfilter"
	"github.com/AdguardTeam/urlfilter/rules"
)

// cosmeticResultWithExtCSS builds a CosmeticResult carrying ExtendedCSS rules
// the way urlfilter returns them: element-hiding ExtCSS entries are bare
// selectors, css ExtCSS entries are full rule text (selector + declaration).
func cosmeticResultWithExtCSS() urlfilter.CosmeticResult {
	return urlfilter.CosmeticResult{
		ElementHiding: urlfilter.StylesResult{
			GenericExtCSS:  []string{`.banner:has(> .ad)`},
			SpecificExtCSS: []string{`div[id="wrap"]:has(> iframe[src*="ads"])`},
		},
		CSS: urlfilter.StylesResult{
			GenericExtCSS: []string{`.promo:contains(Advert) { display: none!important }`},
		},
	}
}

func TestExtendedCssBundle_EmbeddedAndPlausible(t *testing.T) {
	if len(extendedCSSBundle) < 20_000 {
		t.Fatalf("extended-css bundle suspiciously small: %d bytes", len(extendedCSSBundle))
	}
	if !bytes.Contains(extendedCSSBundle, []byte("ExtendedCss")) {
		t.Fatal("extended-css bundle does not expose ExtendedCss")
	}
}

func TestBuildExtendedCssJS_ServesBundleWithCache(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest("GET", "http://"+defaultInjectionsHost+extendedCSSPath+"?ts=1", nil)
	res := s.injectionResponse(NewSession("e1", req))
	if res.StatusCode != 200 {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	if cc := res.Header.Get("Cache-Control"); cc == "" {
		t.Fatal("expected cache headers on the extended-css bundle response")
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Equal(body, extendedCSSBundle) {
		t.Fatalf("body is not the embedded bundle (%d vs %d bytes)", len(body), len(extendedCSSBundle))
	}
}

func TestBuildExtendedCssJS_RejectsNonGET(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest("POST", "http://"+defaultInjectionsHost+extendedCSSPath, nil)
	res := s.injectionResponse(NewSession("e2", req))
	if res.StatusCode != 404 {
		t.Fatalf("want 404 for POST, got %d", res.StatusCode)
	}
}

func TestBuildInjectionCode_ExtendedCssTagBeforeContentScript(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest("GET", "https://example.org/", nil)
	session := NewSession("e3", req)
	session.Result = &rules.MatchingResult{}
	code := s.buildInjectionCode(session)

	iExt := strings.Index(code, extendedCSSPath)
	iCS := strings.Index(code, "/content-script.js")
	if iExt == -1 {
		t.Fatalf("injection code missing %s tag:\n%s", extendedCSSPath, code)
	}
	if iCS == -1 {
		t.Fatalf("injection code missing content-script tag:\n%s", code)
	}
	if iExt > iCS {
		t.Fatalf("extended-css runtime must load before the content script:\n%s", code)
	}
}

func TestContentScript_AppliesExtendedCssRules(t *testing.T) {
	s := testServer()
	code := s.buildContentScriptCode(cosmeticResultWithExtCSS())

	for _, marker := range []string{"applyExtendedCss", "ExtendedCss"} {
		if !strings.Contains(code, marker) {
			t.Fatalf("content script missing %q:\n%s", marker, code)
		}
	}

	// The ExtCSS selectors must reach the payload as data (JS-escaped).
	wantGeneric := template.JSEscapeString(`.banner:has(> .ad)`)
	if !strings.Contains(code, wantGeneric) {
		t.Fatalf("generic element-hiding ExtCSS selector missing from payload:\n%s", code)
	}
	wantCSS := template.JSEscapeString(`.promo:contains(Advert)`)
	if !strings.Contains(code, wantCSS) {
		t.Fatalf("css ExtCSS rule missing from payload:\n%s", code)
	}
}
