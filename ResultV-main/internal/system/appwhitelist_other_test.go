//go:build !windows && !darwin

package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeAppEntryLinuxBasename(t *testing.T) {
	cases := map[string]string{
		"/usr/bin/firefox":              "firefox",
		"/snap/firefox/current/firefox": "firefox",
		"firefox":                       "firefox",
	}
	for in, want := range cases {
		if got := normalizeAppEntry(in); got != want {
			t.Errorf("normalizeAppEntry(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExecFromDesktopFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "firefox.desktop")
	body := "[Desktop Entry]\nName=Firefox\nExec=/usr/lib/firefox/firefox %u\nIcon=firefox\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := normalizeAppEntry(path)
	if got != "firefox" {
		t.Errorf("normalizeAppEntry(%q) = %q, want %q", path, got, "firefox")
	}
}

func TestFirstExecToken(t *testing.T) {
	cases := map[string]string{
		"/usr/bin/x":          "/usr/bin/x",
		"/usr/bin/x %u":       "/usr/bin/x",
		`"/o p/x" --flag`:     "/o p/x",
		`"unterminated quote`: "unterminated quote",
		"":                    "",
		"   ":                 "",
	}
	for in, want := range cases {
		if got := firstExecToken(in); got != want {
			t.Errorf("firstExecToken(%q) = %q, want %q", in, got, want)
		}
	}
}
