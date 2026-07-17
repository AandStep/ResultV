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
	"text/template"

	"github.com/AdguardTeam/urlfilter"
	"github.com/AdguardTeam/urlfilter/rules"
)

// cosmeticResultWithJS builds a CosmeticResult carrying one scriptlet rule
// and one plain-JS rule, the way urlfilter returns them (raw text after the
// #%# marker). urlfilter v0.23.2's CosmeticResult.JS is a plain
// ScriptsResult{Generic, Specific []string} struct — there is no
// NewCosmeticResult constructor and nothing in this vendored subset
// populates JS from CosmeticRule objects, so the rule text is set directly.
func cosmeticResultWithJS(t *testing.T) urlfilter.CosmeticResult {
	t.Helper()
	return urlfilter.CosmeticResult{
		JS: urlfilter.ScriptsResult{
			Generic: []string{
				`//scriptlet('abort-current-inline-script', 'document.dispatchEvent', '/getexoloader/')`,
			},
			Specific: []string{
				`window.__resultvPlainRule = true;`,
			},
		},
	}
}

func TestContentScript_CarriesJSRulesAsStrings(t *testing.T) {
	s := testServer()
	code := s.buildContentScriptCode(cosmeticResultWithJS(t))

	if !strings.Contains(code, `//scriptlet(`) {
		t.Fatalf("scriptlet rule text missing from payload:\n%s", code)
	}
	if !strings.Contains(code, `__resultvPlainRule`) {
		t.Fatalf("plain JS rule missing from payload:\n%s", code)
	}
	// Rules must be data (quoted strings), not template-inlined arrow
	// function bodies — scriptlet text is a JS comment when inlined.
	if strings.Contains(code, "() => { //scriptlet(") {
		t.Fatal("scriptlet rule inlined as arrow-function body (dead code)")
	}
}

func TestContentScript_HasJSRuleExecutor(t *testing.T) {
	s := testServer()
	code := s.buildContentScriptCode(cosmeticResultWithJS(t))

	for _, marker := range []string{
		"applyJSRules",
		"parseScriptletRule",
		"scriptlets.invoke",
		"new Function",
	} {
		if !strings.Contains(code, marker) {
			t.Fatalf("content script missing %q:\n%s", marker, code)
		}
	}
}

func TestContentScript_BalancedDelimiters(t *testing.T) {
	s := testServer()
	code := s.buildContentScriptCode(cosmeticResultWithJS(t))
	for _, p := range []struct{ open, close string }{
		{"(", ")"}, {"{", "}"}, {"[", "]"},
	} {
		if strings.Count(code, p.open) != strings.Count(code, p.close) {
			t.Fatalf("unbalanced %s%s in generated content script", p.open, p.close)
		}
	}
}

// TestContentScript_ScriptletRuleFlowsFromListToPayload exercises the real
// pipeline: a `#%#` scriptlet rule in a filter list is indexed by
// BuildScriptletIndex (urlfilter's own engine rejects such rules at parse
// time and never surfaces them via GetCosmeticResult), the resulting
// ScriptletIndex is wired onto a Server the way NewServer does it, and
// buildContentScript merges the index's match into CosmeticResult.JS. It
// asserts the scriptlet doesn't just appear somewhere in the served file,
// but specifically inside the payload's "js" block.
func TestContentScript_ScriptletRuleFlowsFromListToPayload(t *testing.T) {
	// urlfilter's engine keeps the list files open; on Windows t.TempDir's
	// RemoveAll then fails even though the assertions pass. Cleanup is best-effort.
	dir, err := os.MkdirTemp("", "scriptlet-flow-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	list := filepath.Join(dir, "list.txt")
	const scriptletText = `//scriptlet('abort-current-inline-script', 'document.dispatchEvent', '/getexoloader/')`
	content := "! test\n" +
		"example.org#%#" + scriptletText + "\n"
	if err := os.WriteFile(list, []byte(content), 0o600); err != nil {
		t.Fatalf("writing list: %v", err)
	}

	ix, err := BuildScriptletIndex(map[rules.ListID]string{1: list})
	if err != nil {
		t.Fatalf("BuildScriptletIndex: %v", err)
	}

	s := testServerWithScriptletIndex(t, dir, ix)

	url := fmt.Sprintf("http://%s/content-script.js?hostname=example.org&option=%d&ts=%d",
		s.InjectionHost, rules.CosmeticOptionAll, s.createdAt.Unix())
	req := httptest.NewRequest("GET", url, nil)
	res := s.buildContentScript(NewSession("t-scriptlet-flow", req))
	if res.StatusCode != 200 {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	code := string(body)

	// The template renders rule text through the "js" escaper (single quotes
	// become \'), so compare against the escaped form.
	wantText := template.JSEscapeString(scriptletText)
	if !strings.Contains(code, wantText) {
		t.Fatalf("scriptlet rule from the list did not reach the payload:\n%s", code)
	}

	// Prove the rule landed inside the "js" block specifically, not merely
	// somewhere in the file (e.g. a comment or an unrelated block).
	jsBlockIdx := strings.Index(code, `"js": {`)
	if jsBlockIdx == -1 {
		t.Fatalf("payload missing \"js\" block:\n%s", code)
	}
	if !strings.Contains(code[jsBlockIdx:], wantText) {
		t.Fatalf("scriptlet rule present in payload but not inside the \"js\" block:\n%s", code)
	}
}
