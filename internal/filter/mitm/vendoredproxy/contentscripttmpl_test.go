// Copyright (C) 2026 ResultV — GPL-3.0 (see file headers elsewhere).

package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestContentScript_ScriptletRuleFlowsFromListToPayload(t *testing.T) {
	// urlfilter's engine keeps the list files open; on Windows t.TempDir's
	// RemoveAll then fails even though the assertions pass. Cleanup is best-effort.
	dir, err := os.MkdirTemp("", "scriptlet-flow-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	list := filepath.Join(dir, "list.txt")
	content := "! test\n" +
		"example.org#%#//scriptlet('abort-current-inline-script', 'document.dispatchEvent', '/getexoloader/')\n"
	if err := os.WriteFile(list, []byte(content), 0o600); err != nil {
		t.Fatalf("writing list: %v", err)
	}

	eng, err := BuildEngine(map[rules.ListID]string{1: list})
	if err != nil {
		t.Fatalf("BuildEngine: %v", err)
	}

	res := eng.GetCosmeticResult("example.org", rules.CosmeticOptionAll)
	s := testServer()
	code := s.buildContentScriptCode(res)

	// Task 4 commits to: (1) parseScriptletRule function for parsing rule text, and
	// (2) rules must NOT be inlined as arrow function bodies. This locks the
	// content-script template format for cosmetic rules.
	if !strings.Contains(code, "parseScriptletRule") {
		t.Fatal("Task 4 parseScriptletRule function missing; template not updated")
	}
	if strings.Contains(code, "() => { //scriptlet(") {
		t.Fatal("scriptlet rule inlined as arrow-function body (dead code); Task 4 template changes not present")
	}
}
