//go:build windows

package proxy

import (
	"os"
	"path/filepath"
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

// TestProxySnapshotLifecycle covers the leftover-marker mechanism: the marker
// is written by Set (via writeProxySnapshot) and removed by Disable, and its
// absence makes LeftoverActive report "no leftover" without touching the
// registry. The marker-present branch additionally consults ProxyEnable, which
// we don't mutate from a unit test.
func TestProxySnapshotLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), proxySnapshotFile)
	w := &WindowsSystemProxy{router: NewRouter(), snapshotPath: path}

	if w.LeftoverActive() {
		t.Fatal("LeftoverActive should be false when no snapshot marker exists")
	}

	if err := writeProxySnapshot(path, "127.0.0.1:1080"); err != nil {
		t.Fatalf("writeProxySnapshot: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("snapshot marker missing after write: %v", err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove snapshot marker: %v", err)
	}
	if w.LeftoverActive() {
		t.Fatal("LeftoverActive should be false after the marker is removed")
	}
}
