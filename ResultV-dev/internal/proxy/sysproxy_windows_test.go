//go:build windows

package proxy

import (
	"strings"
	"testing"
)

func TestBuildBypassListUsesSpecificPatternsOnly(t *testing.T) {
	w := NewWindowsSystemProxy(NewRouter())
	got := w.buildBypassList([]string{"localhost", "127.0.0.1", "*.ru"})

	if strings.Contains(got, "*localhost*") || strings.Contains(got, "*127.0.0.1*") || strings.Contains(got, "*ru*") {
		t.Fatalf("unexpected broad wildcard in ProxyOverride: %q", got)
	}
	if !strings.Contains(got, "localhost") || !strings.Contains(got, "*.localhost") {
		t.Fatalf("expected localhost patterns, got: %q", got)
	}
	if !strings.Contains(got, "127.0.0.1") || !strings.Contains(got, "*.127.0.0.1") {
		t.Fatalf("expected loopback patterns, got: %q", got)
	}
	if !strings.Contains(got, "ru") || !strings.Contains(got, "*.ru") {
		t.Fatalf("expected normalized domain patterns, got: %q", got)
	}
	if !strings.HasSuffix(got, "<local>") {
		t.Fatalf("expected local suffix, got: %q", got)
	}
}
