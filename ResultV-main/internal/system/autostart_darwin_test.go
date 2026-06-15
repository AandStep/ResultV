//go:build darwin

package system

import (
	"strings"
	"testing"
)

func TestRenderLaunchAgentPlistContainsRequiredKeys(t *testing.T) {
	got := renderLaunchAgentPlist("/Applications/ResultV.app/Contents/MacOS/ResultV", []string{"--foo"})
	for _, want := range []string{
		"<key>Label</key>",
		"<string>" + launchAgentLabel + "</string>",
		"<key>ProgramArguments</key>",
		"<string>/Applications/ResultV.app/Contents/MacOS/ResultV</string>",
		"<string>" + autostartTrayToken + "</string>",
		"<string>--foo</string>",
		"<key>RunAtLoad</key>",
		"<true/>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in plist:\n%s", want, got)
		}
	}
}

func TestRenderLaunchAgentPlistEscapesSpecial(t *testing.T) {
	got := renderLaunchAgentPlist("/o&p/<x>/r", nil)
	if strings.Contains(got, "<x>") || !strings.Contains(got, "&lt;x&gt;") {
		t.Fatalf("expected XML-escaping of </> in path, got:\n%s", got)
	}
	if !strings.Contains(got, "&amp;") {
		t.Fatalf("expected & to be escaped, got:\n%s", got)
	}
}
