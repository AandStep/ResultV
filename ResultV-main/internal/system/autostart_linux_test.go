//go:build linux

package system

import (
	"strings"
	"testing"
)

func TestRenderAutostartDesktopEntry(t *testing.T) {
	got := renderAutostartDesktopEntry("/usr/bin/resultv", nil)
	for _, want := range []string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=ResultV",
		"Exec=/usr/bin/resultv " + autostartTrayToken,
		"X-GNOME-Autostart-enabled=true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in entry:\n%s", want, got)
		}
	}
}

func TestRenderAutostartDesktopEntryQuotesPathWithSpace(t *testing.T) {
	got := renderAutostartDesktopEntry("/opt/Result V/resultv", nil)
	if !strings.Contains(got, `Exec="/opt/Result V/resultv" --autostart`) {
		t.Fatalf("expected quoted Exec for path with space, got:\n%s", got)
	}
}

func TestQuoteDesktopExec(t *testing.T) {
	cases := map[string]string{
		"/usr/bin/x":  "/usr/bin/x",
		"/o p/x":      `"/o p/x"`,
		`/path/"q"/x`: `"/path/\"q\"/x"`,
		`/path/\back`: `"/path/\\back"`,
	}
	for in, want := range cases {
		if got := quoteDesktopExec(in); got != want {
			t.Errorf("quoteDesktopExec(%q) = %q, want %q", in, got, want)
		}
	}
}
