//go:build windows

package system

import "testing"

func TestNormalizeAppEntryWindows(t *testing.T) {
	cases := map[string]string{
		`C:\Program Files\Foo\bar.exe`: "bar.exe",
		`bar.exe`:                      "bar.exe",
		`bar`:                          "bar.exe",
		`C:/forward/slashes/x.exe`:     "x.exe",
		`Chrome.EXE`:                   "Chrome.EXE",
	}
	for in, want := range cases {
		if got := normalizeAppEntry(in); got != want {
			t.Errorf("normalizeAppEntry(%q) = %q, want %q", in, got, want)
		}
	}
}
