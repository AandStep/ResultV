package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompileSmartSRS_RoundTripMatches(t *testing.T) {
	dir := t.TempDir()
	path := SmartSRSPath(dir)
	if err := CompileSmartSRS([]string{"x.com", "instagram.com", "youtube.com"}, path); err != nil {
		t.Fatalf("CompileSmartSRS: %v", err)
	}
	if !localSmartSRSUsable(path) {
		t.Fatal("compiled SRS should be usable")
	}
	m, err := LoadSmartDomainMatcher(path)
	if err != nil {
		t.Fatalf("LoadSmartDomainMatcher: %v", err)
	}
	// Pins the same semantics as engine_smart_test.go: bare AND subdomain.
	for _, host := range []string{"x.com", "instagram.com", "www.instagram.com", "i.instagram.com"} {
		if !m.Match(host) {
			t.Errorf("matcher must match %q", host)
		}
	}
	for _, host := range []string{"fakeinstagram.com", "example.org"} {
		if m.Match(host) {
			t.Errorf("matcher must NOT match %q", host)
		}
	}
}

func TestCompileSmartSRS_AtomicNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	path := SmartSRSPath(dir)
	if err := CompileSmartSRS([]string{"x.com"}, path); err != nil {
		t.Fatal(err)
	}
	// A failed compile must not clobber a good existing file.
	if err := CompileSmartSRS(nil, path); err == nil {
		t.Fatal("empty domain list should error, not write")
	}
	if !localSmartSRSUsable(path) {
		t.Fatal("previous good SRS must survive a failed compile")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file must not be left behind")
	}
}

func TestLocalSmartSRSUsable_RejectsAndRemovesCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := SmartSRSPath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not an srs file at all, but long enough to pass a size check"), 0o600); err != nil {
		t.Fatal(err)
	}
	if localSmartSRSUsable(path) {
		t.Fatal("corrupt SRS must not be reported usable")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("corrupt SRS must be deleted so a refresh can replace it")
	}
}

func TestBuildSmartRuleSet_OnlyWhenUsable(t *testing.T) {
	dir := t.TempDir()
	if got := buildSmartRuleSet(dir); len(got) != 0 {
		t.Fatalf("no SRS on disk should yield no rule_set, got %+v", got)
	}
	if err := CompileSmartSRS([]string{"x.com"}, SmartSRSPath(dir)); err != nil {
		t.Fatal(err)
	}
	got := buildSmartRuleSet(dir)
	if len(got) != 1 {
		t.Fatalf("expected 1 rule_set, got %d", len(got))
	}
	if got[0].Type != "local" || got[0].Format != "binary" || got[0].Tag != smartRuleSetTag {
		t.Fatalf("unexpected rule_set: %+v", got[0])
	}
}
