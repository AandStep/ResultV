//go:build darwin

package proxy

import (
	"slices"
	"strings"
	"testing"
)

func TestDarwinBuildBypassIncludesLocalDefaults(t *testing.T) {
	d := &DarwinSystemProxy{router: NewRouter()}
	got := d.buildBypass(nil)
	for _, want := range []string{"localhost", "127.0.0.1", "::1", "*.local", "169.254/16"} {
		if !slices.Contains(got, want) {
			t.Fatalf("expected default bypass to include %q, got %v", want, got)
		}
	}
}

func TestDarwinBuildBypassExpandsWhitelist(t *testing.T) {
	d := &DarwinSystemProxy{router: NewRouter()}
	got := d.buildBypass([]string{"example.com"})
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "example.com") || !strings.Contains(joined, "*.example.com") {
		t.Fatalf("expected example.com and *.example.com in bypass, got %v", got)
	}
}
