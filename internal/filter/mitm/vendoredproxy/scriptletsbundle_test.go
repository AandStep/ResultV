package proxy

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AdguardTeam/urlfilter/rules"
)

func TestScriptletsBundle_EmbeddedAndPlausible(t *testing.T) {
	if len(scriptletsBundle) < 50_000 {
		t.Fatalf("scriptlets bundle suspiciously small: %d bytes", len(scriptletsBundle))
	}
	if !bytes.Contains(scriptletsBundle, []byte("invoke")) {
		t.Fatal("scriptlets bundle does not export invoke")
	}
	if !bytes.Contains(scriptletsBundle, []byte("window.scriptlets = scriptlets")) {
		t.Fatal("scriptlets bundle does not assign to window.scriptlets")
	}
	if bytes.Contains(scriptletsBundle, []byte("\nexport ")) {
		t.Fatal("scriptlets bundle still contains ESM export statement")
	}
}

func testServer() *Server {
	return &Server{
		createdAt: time.Now(),
		Config:    Config{InjectionHost: defaultInjectionsHost},
	}
}

func TestBuildScriptletsJS_ServesBundleWithCache(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest("GET", "http://"+defaultInjectionsHost+scriptletsPath+"?ts=1", nil)
	res := s.injectionResponse(NewSession("t1", req))
	if res.StatusCode != 200 {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	if cc := res.Header.Get("Cache-Control"); cc == "" {
		t.Fatal("expected cache headers on the scriptlets bundle response")
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Equal(body, scriptletsBundle) {
		t.Fatalf("body is not the embedded bundle (%d vs %d bytes)", len(body), len(scriptletsBundle))
	}
}

func TestBuildScriptletsJS_RejectsNonGET(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest("POST", "http://"+defaultInjectionsHost+scriptletsPath, nil)
	res := s.injectionResponse(NewSession("t2", req))
	if res.StatusCode != 404 {
		t.Fatalf("want 404 for POST, got %d", res.StatusCode)
	}
}

func TestInjectionResponse_FallsThroughToContentScript(t *testing.T) {
	s := testServer()
	// Bad content-script params -> newNotFoundResponse, proving dispatch
	// reached buildContentScript rather than the bundle handler.
	req := httptest.NewRequest("GET", "http://"+defaultInjectionsHost+"/content-script.js", nil)
	res := s.injectionResponse(NewSession("t3", req))
	if res.StatusCode != 404 {
		t.Fatalf("want 404 from buildContentScript on missing params, got %d", res.StatusCode)
	}
}

func TestBuildInjectionCode_ScriptletsTagBeforeContentScript(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest("GET", "https://example.org/", nil)
	session := NewSession("t4", req)
	session.Result = &rules.MatchingResult{}
	code := s.buildInjectionCode(session)

	iBundle := strings.Index(code, scriptletsPath)
	iCS := strings.Index(code, "/content-script.js")
	if iBundle == -1 {
		t.Fatalf("injection code missing %s tag:\n%s", scriptletsPath, code)
	}
	if iCS == -1 {
		t.Fatalf("injection code missing content-script tag:\n%s", code)
	}
	if iBundle > iCS {
		t.Fatalf("scriptlets bundle must load before the content script:\n%s", code)
	}
}
