// Copyright (C) 2026 ResultV
//
// Licensed under the terms of the GNU General Public License v3 or later.

package system

import (
	"strings"
	"testing"

	"resultproxy-wails/internal/config"
)

// TestServerMenuLabel locks in the visual distinction between AUTO heads and
// individual servers in the tray. The bug this guards against: when AUTO and
// regular entries look identical, users click an "Austria" entry expecting a
// single server and end up at whichever member auto-routing picked.
func TestServerMenuLabel(t *testing.T) {
	cases := []struct {
		name   string
		entry  config.ProxyEntry
		want   string
		mustNotContain []string
	}{
		{
			name: "regular server with country flag",
			entry: config.ProxyEntry{
				Name:    "Austria #1",
				Type:    "VLESS",
				Country: "AT",
				IP:      "1.2.3.4",
				Port:    443,
			},
			want: "\U0001F1E6\U0001F1F9  Austria #1",
		},
		{
			name: "regular server without country falls back to name",
			entry: config.ProxyEntry{
				Name: "My Server",
				Type: "TROJAN",
				IP:   "1.2.3.4",
				Port: 8080,
			},
			want: "My Server",
		},
		{
			name: "regular server with no name falls back to host:port",
			entry: config.ProxyEntry{
				Type:    "SS",
				IP:      "5.6.7.8",
				Port:    1080,
				Country: "DE",
			},
			want: "\U0001F1E9\U0001F1EA  5.6.7.8:1080",
		},
		{
			name: "AUTO head MUST start with [AUTO] marker",
			entry: config.ProxyEntry{
				Name:    "Premium",
				Type:    "AUTO",
				Country: "AT", // even with a country, AUTO must not look country-locked
			},
			want: "[AUTO] Premium  ⚡",
			mustNotContain: []string{"\U0001F1E6\U0001F1F9"}, // no Austria flag
		},
		{
			name: "AUTO head named after a country still does not get a flag",
			entry: config.ProxyEntry{
				Name:    "Austria",
				Type:    "auto", // case-insensitive
				Country: "AT",
			},
			want: "[AUTO] Austria  ⚡",
			mustNotContain: []string{"\U0001F1E6\U0001F1F9"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := serverMenuLabel(tc.entry)
			if got != tc.want {
				t.Fatalf("label mismatch:\ngot:  %q\nwant: %q", got, tc.want)
			}
			for _, banned := range tc.mustNotContain {
				if strings.Contains(got, banned) {
					t.Fatalf("label %q must not contain %q", got, banned)
				}
			}
		})
	}
}

// TestServerMenuTooltipAuto guarantees AUTO tooltips warn the user about
// auto-routing semantics so a click is never a silent surprise.
func TestServerMenuTooltipAuto(t *testing.T) {
	tip := serverMenuTooltip(config.ProxyEntry{
		Name: "Premium",
		Type: "AUTO",
	})
	if !strings.Contains(tip, "лучшему") {
		t.Fatalf("AUTO tooltip should mention routing behaviour, got %q", tip)
	}
}
