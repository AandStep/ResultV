//go:build linux

package proxy

import (
	"strings"
	"testing"
)

func TestFormatGSettingsList(t *testing.T) {
	got := formatGSettingsList([]string{"localhost", "127.0.0.0/8"})
	want := "['localhost', '127.0.0.0/8']"
	if got != want {
		t.Fatalf("formatGSettingsList: want %q, got %q", want, got)
	}
}

func TestFormatGSettingsListEscapesQuotes(t *testing.T) {
	got := formatGSettingsList([]string{"a'b"})
	if !strings.Contains(got, `\'`) {
		t.Fatalf("expected escaped quote, got %q", got)
	}
}

func TestLinuxBackendsRegistered(t *testing.T) {
	p := newSystemProxy(NewRouter()).(*LinuxSystemProxy)
	if len(p.backends) < 3 {
		t.Fatalf("expected at least 3 backends, got %d", len(p.backends))
	}
	names := map[string]bool{}
	for _, b := range p.backends {
		names[b.Name()] = true
	}
	for _, want := range []string{"gnome", "kde", "env"} {
		if !names[want] {
			t.Fatalf("expected backend %q to be registered, got %v", want, names)
		}
	}
}
