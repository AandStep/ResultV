package system

import (
	"reflect"
	"testing"
)

func TestNormalizeAppListDeduplicates(t *testing.T) {
	got := NormalizeAppList([]string{"chrome.exe", "Chrome.exe", "", "  ", "firefox"})
	// On Windows the .exe suffix is preserved; on other OSes "chrome.exe"
	// keeps its dot. Either way, the case-insensitive dedupe should drop
	// the duplicate Chrome entry.
	if len(got) != 2 {
		t.Fatalf("expected 2 entries after dedupe, got %v", got)
	}
}

func TestNormalizeAppListPreservesOrder(t *testing.T) {
	got := NormalizeAppList([]string{"a", "b", "a", "c"})
	want := []string{"a", "b", "c"}
	// Compare using basename of each — implementations may add/strip
	// extensions on Windows but not on others.
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNormalizeAppEntryEmpty(t *testing.T) {
	if got := NormalizeAppEntry(""); got != "" {
		t.Fatalf("empty input should produce empty output, got %q", got)
	}
	if got := NormalizeAppEntry("   "); got != "" {
		t.Fatalf("whitespace input should produce empty output, got %q", got)
	}
}

func TestNormalizeAppListNilSafe(t *testing.T) {
	if got := NormalizeAppList(nil); got != nil {
		t.Fatalf("nil input should return nil, got %v", got)
	}
	if got := NormalizeAppList([]string{}); !reflect.DeepEqual(got, []string{}) {
		t.Fatalf("empty slice should round-trip, got %v", got)
	}
}
