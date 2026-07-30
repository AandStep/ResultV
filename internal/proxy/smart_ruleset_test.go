package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/common/srs"
)

func TestCompileSmartRuleSet_WritesReadableRuleSet(t *testing.T) {
	dir := t.TempDir()
	domains := []string{"instagram.com", "discord.com", "x.com"}

	path, err := CompileSmartRuleSet(dir, domains)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(path), "smart-") || !strings.HasSuffix(path, ".srs") {
		t.Fatalf("unexpected file name %q", path)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	// recover=true asks the reader to reconstruct the plain domain lists from
	// the compiled matcher; without it Read only returns the matcher itself.
	compat, err := srs.Read(f, true)
	if err != nil {
		t.Fatalf("srs read back: %v", err)
	}
	plain, err := compat.Upgrade()
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if len(plain.Rules) != 1 {
		t.Fatalf("expected 1 headless rule, got %d", len(plain.Rules))
	}
	// The compiled matcher does not keep the exact/suffix split, so Dump() may
	// hand entries back in either bucket. What must hold is that every domain
	// survives the round-trip.
	got := append([]string(nil), plain.Rules[0].DefaultOptions.Domain...)
	got = append(got, plain.Rules[0].DefaultOptions.DomainSuffix...)
	if len(got) != len(domains) {
		t.Fatalf("domain round-trip mismatch: got %d entries %v, want %d", len(got), got, len(domains))
	}
	have := map[string]bool{}
	for _, d := range got {
		have[d] = true
	}
	for _, d := range domains {
		if !have[d] {
			t.Fatalf("domain %q missing after round-trip, got %v", d, got)
		}
	}

	// Publication is a rename, so a successful compile must leave no temp file
	// behind for the engine (or the next compile) to trip over.
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp"))
	if len(leftovers) != 0 {
		t.Fatalf("temp files left after compile: %v", leftovers)
	}
}

func TestCompileSmartRuleSet_CachesByContent(t *testing.T) {
	dir := t.TempDir()
	domains := []string{"instagram.com", "discord.com"}

	first, err := CompileSmartRuleSet(dir, domains)
	if err != nil {
		t.Fatalf("first compile: %v", err)
	}
	stFirst, err := os.Stat(first)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	second, err := CompileSmartRuleSet(dir, domains)
	if err != nil {
		t.Fatalf("second compile: %v", err)
	}
	if second != first {
		t.Fatalf("same list must reuse the same path: %q vs %q", first, second)
	}
	stSecond, err := os.Stat(second)
	if err != nil {
		t.Fatalf("stat cached: %v", err)
	}
	if !stSecond.ModTime().Equal(stFirst.ModTime()) {
		t.Fatalf("cached compile must not rewrite the file")
	}

	changed, err := CompileSmartRuleSet(dir, append(domains, "x.com"))
	if err != nil {
		t.Fatalf("changed compile: %v", err)
	}
	if changed == first {
		t.Fatalf("a different list must produce a different path, both %q", changed)
	}
}

func TestCompileSmartRuleSet_PrunesStaleFiles(t *testing.T) {
	dir := t.TempDir()
	first, err := CompileSmartRuleSet(dir, []string{"a.com"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := CompileSmartRuleSet(dir, []string{"b.com"})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("stale rule-set %q must be removed, stat err=%v", first, err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("current rule-set must survive: %v", err)
	}
}

func TestCompileSmartRuleSet_RejectsBadInput(t *testing.T) {
	if _, err := CompileSmartRuleSet("", []string{"a.com"}); err == nil {
		t.Fatal("empty data dir must return an error so the caller falls back to inline rules")
	}
	if _, err := CompileSmartRuleSet(t.TempDir(), nil); err == nil {
		t.Fatal("empty domain list must return an error so the caller falls back to inline rules")
	}
}
