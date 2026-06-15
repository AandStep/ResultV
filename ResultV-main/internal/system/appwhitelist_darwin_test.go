//go:build darwin

package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundleRoot(t *testing.T) {
	cases := map[string]string{
		"/Applications/Brave.app":                                  "/Applications/Brave.app",
		"/Applications/Brave.app/":                                 "/Applications/Brave.app",
		"/Applications/Brave.app/Contents/MacOS/Brave":             "/Applications/Brave.app",
		"/Applications/Visual Studio Code.app/Contents/Info.plist": "/Applications/Visual Studio Code.app",
		"/usr/bin/firefox":                                         "",
		"/Applications/Foo.application/x":                          "",
	}
	for in, want := range cases {
		if got := bundleRoot(in); got != want {
			t.Errorf("bundleRoot(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeAppEntryDarwinFallsBackToBundleName(t *testing.T) {
	// No Info.plist on disk -> falls back to "Brave".
	got := normalizeAppEntry("/Applications/Brave.app")
	if got != "Brave" {
		t.Errorf("normalizeAppEntry without plist = %q, want %q", got, "Brave")
	}
}

func TestNormalizeAppEntryDarwinReadsPlist(t *testing.T) {
	tmp := t.TempDir()
	bundle := filepath.Join(tmp, "Visual Studio Code.app")
	if err := os.MkdirAll(filepath.Join(bundle, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key>
  <string>Electron</string>
</dict>
</plist>`
	if err := os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := normalizeAppEntry(bundle); got != "Electron" {
		t.Errorf("normalizeAppEntry with plist = %q, want %q", got, "Electron")
	}
}

func TestNormalizeAppEntryDarwinNonBundle(t *testing.T) {
	if got := normalizeAppEntry("/usr/bin/curl"); got != "curl" {
		t.Errorf("normalizeAppEntry(/usr/bin/curl) = %q, want %q", got, "curl")
	}
}
