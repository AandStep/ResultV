// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/AdguardTeam/urlfilter/rules"
)

// writeListFile writes content to a temp file inside t.TempDir() and returns
// its path.
func writeListFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test list file: %v", err)
	}

	return path
}

func TestScriptletIndex(t *testing.T) {
	t.Run("specific rule matches exact domain", func(t *testing.T) {
		path := writeListFile(t, "list.txt", `example.org#%#//scriptlet('abort-on-property-write', 'x')`)

		ix, err := BuildScriptletIndex(map[rules.ListID]string{1: path})
		if err != nil {
			t.Fatalf("BuildScriptletIndex failed: %v", err)
		}

		generic, specific := ix.Match("example.org")
		if len(generic) != 0 {
			t.Errorf("expected no generic rules, got %v", generic)
		}
		want := "//scriptlet('abort-on-property-write', 'x')"
		if len(specific) != 1 || specific[0] != want {
			t.Errorf("specific = %v, want [%q]", specific, want)
		}
	})

	t.Run("specific rule matches subdomain", func(t *testing.T) {
		path := writeListFile(t, "list.txt", `example.org#%#//scriptlet('abort-on-property-write', 'x')`)

		ix, err := BuildScriptletIndex(map[rules.ListID]string{1: path})
		if err != nil {
			t.Fatalf("BuildScriptletIndex failed: %v", err)
		}

		generic, specific := ix.Match("hot.example.org")
		if len(generic) != 0 {
			t.Errorf("expected no generic rules, got %v", generic)
		}
		want := "//scriptlet('abort-on-property-write', 'x')"
		if len(specific) != 1 || specific[0] != want {
			t.Errorf("specific = %v, want [%q]", specific, want)
		}
	})

	t.Run("non-matching hostname returns nothing", func(t *testing.T) {
		path := writeListFile(t, "list.txt", `example.org#%#//scriptlet('abort-on-property-write', 'x')`)

		ix, err := BuildScriptletIndex(map[rules.ListID]string{1: path})
		if err != nil {
			t.Fatalf("BuildScriptletIndex failed: %v", err)
		}

		generic, specific := ix.Match("other.com")
		if len(generic) != 0 || len(specific) != 0 {
			t.Errorf("expected empty result, got generic=%v specific=%v", generic, specific)
		}
	})

	t.Run("generic rule matches any hostname but never as specific", func(t *testing.T) {
		path := writeListFile(t, "list.txt", `#%#window.x=1;`)

		ix, err := BuildScriptletIndex(map[rules.ListID]string{1: path})
		if err != nil {
			t.Fatalf("BuildScriptletIndex failed: %v", err)
		}

		generic, specific := ix.Match("anything.example")
		if len(specific) != 0 {
			t.Errorf("expected no specific rules, got %v", specific)
		}
		if len(generic) != 1 || generic[0] != "window.x=1;" {
			t.Errorf("generic = %v, want [%q]", generic, "window.x=1;")
		}
	})

	t.Run("multi-domain rule matches each domain and their subdomains", func(t *testing.T) {
		path := writeListFile(t, "list.txt", `a.com,b.com#%#code`)

		ix, err := BuildScriptletIndex(map[rules.ListID]string{1: path})
		if err != nil {
			t.Fatalf("BuildScriptletIndex failed: %v", err)
		}

		_, specificA := ix.Match("a.com")
		if len(specificA) != 1 || specificA[0] != "code" {
			t.Errorf("Match(a.com) specific = %v, want [code]", specificA)
		}

		_, specificSubB := ix.Match("sub.b.com")
		if len(specificSubB) != 1 || specificSubB[0] != "code" {
			t.Errorf("Match(sub.b.com) specific = %v, want [code]", specificSubB)
		}
	})

	t.Run("restricted domain excludes itself and its subdomains", func(t *testing.T) {
		path := writeListFile(t, "list.txt", `a.com,~bad.a.com#%#code`)

		ix, err := BuildScriptletIndex(map[rules.ListID]string{1: path})
		if err != nil {
			t.Fatalf("BuildScriptletIndex failed: %v", err)
		}

		_, specificA := ix.Match("a.com")
		if len(specificA) != 1 || specificA[0] != "code" {
			t.Errorf("Match(a.com) specific = %v, want [code]", specificA)
		}

		_, specificBad := ix.Match("bad.a.com")
		if len(specificBad) != 0 {
			t.Errorf("Match(bad.a.com) specific = %v, want empty", specificBad)
		}

		_, specificDeepBad := ix.Match("deep.bad.a.com")
		if len(specificDeepBad) != 0 {
			t.Errorf("Match(deep.bad.a.com) specific = %v, want empty", specificDeepBad)
		}
	})

	t.Run("whitelist suppresses matching content", func(t *testing.T) {
		path := writeListFile(t, "list.txt", "a.com#%#code\na.com#@%#code")

		ix, err := BuildScriptletIndex(map[rules.ListID]string{1: path})
		if err != nil {
			t.Fatalf("BuildScriptletIndex failed: %v", err)
		}

		generic, specific := ix.Match("a.com")
		if len(generic) != 0 || len(specific) != 0 {
			t.Errorf("expected empty result, got generic=%v specific=%v", generic, specific)
		}
	})

	t.Run("whitelist is scoped to matching content only", func(t *testing.T) {
		path := writeListFile(t, "list.txt", "a.com#%#code1\na.com#@%#code2")

		ix, err := BuildScriptletIndex(map[rules.ListID]string{1: path})
		if err != nil {
			t.Fatalf("BuildScriptletIndex failed: %v", err)
		}

		_, specific := ix.Match("a.com")
		if len(specific) != 1 || specific[0] != "code1" {
			t.Errorf("specific = %v, want [code1]", specific)
		}
	})

	t.Run("element-hiding and comment lines are ignored", func(t *testing.T) {
		path := writeListFile(t, "list.txt", "a.com##.banner\n! this is a comment\na.com#%#code")

		ix, err := BuildScriptletIndex(map[rules.ListID]string{1: path})
		if err != nil {
			t.Fatalf("BuildScriptletIndex failed: %v", err)
		}

		_, specific := ix.Match("a.com")
		if len(specific) != 1 || specific[0] != "code" {
			t.Errorf("specific = %v, want [code]", specific)
		}
	})

	t.Run("plain JS content with quotes and commas passes through verbatim", func(t *testing.T) {
		content := `(function(){Object.defineProperty(window,'ExoLoader',{value:1,writable:false});})();`
		path := writeListFile(t, "list.txt", "a.com#%#"+content)

		ix, err := BuildScriptletIndex(map[rules.ListID]string{1: path})
		if err != nil {
			t.Fatalf("BuildScriptletIndex failed: %v", err)
		}

		_, specific := ix.Match("a.com")
		if len(specific) != 1 || specific[0] != content {
			t.Errorf("specific = %v, want [%q]", specific, content)
		}
	})

	t.Run("nil index is safe", func(t *testing.T) {
		var ix *ScriptletIndex

		generic, specific := ix.Match("example.org")
		if generic == nil {
			t.Errorf("expected non-nil empty generic slice")
		}
		if specific == nil {
			t.Errorf("expected non-nil empty specific slice")
		}
		if len(generic) != 0 || len(specific) != 0 {
			t.Errorf("expected empty result, got generic=%v specific=%v", generic, specific)
		}
	})

	t.Run("missing list file does not prevent indexing other lists", func(t *testing.T) {
		validPath := writeListFile(t, "valid.txt", `a.com#%#code`)
		missingPath := filepath.Join(t.TempDir(), "does-not-exist.txt")

		ix, err := BuildScriptletIndex(map[rules.ListID]string{
			1: missingPath,
			2: validPath,
		})
		if err != nil {
			t.Fatalf("BuildScriptletIndex failed: %v", err)
		}

		_, specific := ix.Match("a.com")
		if len(specific) != 1 || specific[0] != "code" {
			t.Errorf("specific = %v, want [code]", specific)
		}
	})

	t.Run("exception marker is distinguished from rule marker", func(t *testing.T) {
		// #@%# must not be misread as #%# with content "@code".
		path := writeListFile(t, "list.txt", "a.com#@%#code\na.com#%#code")

		ix, err := BuildScriptletIndex(map[rules.ListID]string{1: path})
		if err != nil {
			t.Fatalf("BuildScriptletIndex failed: %v", err)
		}

		_, specific := ix.Match("a.com")
		if len(specific) != 0 {
			t.Errorf("specific = %v, want empty (whitelisted)", specific)
		}
	})

	t.Run("results are sorted and deduplicated", func(t *testing.T) {
		path := writeListFile(t, "list.txt", "a.com#%#zebra\na.com#%#apple\na.com#%#apple")

		ix, err := BuildScriptletIndex(map[rules.ListID]string{1: path})
		if err != nil {
			t.Fatalf("BuildScriptletIndex failed: %v", err)
		}

		_, specific := ix.Match("a.com")
		want := []string{"apple", "zebra"}
		if !sort.StringsAreSorted(specific) || len(specific) != len(want) {
			t.Fatalf("specific = %v, want sorted+deduped %v", specific, want)
		}
		for i := range want {
			if specific[i] != want[i] {
				t.Fatalf("specific = %v, want %v", specific, want)
			}
		}
	})
}
