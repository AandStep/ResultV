// Copyright (C) 2026 ResultV
//
// Licensed under the terms of the GNU General Public License v3 or later.

package main

import (
	"encoding/json"
	"testing"

	"resultproxy-wails/internal/config"
)

func mustExtra(t *testing.T, members []string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"members": members})
	if err != nil {
		t.Fatalf("marshal members: %v", err)
	}
	return raw
}

// TestFilterTrayProxies locks in the user-driven contract (2026-05-28):
// the tray shows EVERY connectable server — including former AUTO members
// — and hides ONLY AUTO heads and SECTION labels.
//
// Why: showing AUTO heads in the tray caused "click Austria, connect to
// Germany". The AUTO head looked country-named but actually auto-routed
// to whichever member pinged fastest. Removing AUTO heads from the tray
// guarantees every tray click connects to a single, predictable server.
func TestFilterTrayProxies(t *testing.T) {
	autoHead := config.ProxyEntry{
		ID:    "auto-1",
		Name:  "Premium",
		Type:  "AUTO",
		Extra: mustExtra(t, []string{"m1", "m2", "m3"}),
	}
	member1 := config.ProxyEntry{ID: "m1", Name: "Austria #1", Type: "VLESS", IP: "1.1.1.1", Port: 443, Country: "AT"}
	member2 := config.ProxyEntry{ID: "m2", Name: "Germany #2", Type: "VLESS", IP: "2.2.2.2", Port: 443, Country: "DE"}
	member3 := config.ProxyEntry{ID: "m3", Name: "USA #3", Type: "VLESS", IP: "3.3.3.3", Port: 443, Country: "US"}
	standalone := config.ProxyEntry{ID: "std-1", Name: "Personal Server", Type: "TROJAN", IP: "4.4.4.4", Port: 8443}
	section := config.ProxyEntry{ID: "sec-1", Name: "── Premium servers ──", Type: "SECTION"}

	cases := []struct {
		name    string
		input   []config.ProxyEntry
		wantIDs []string
	}{
		{
			name:    "empty list returns empty",
			input:   nil,
			wantIDs: nil,
		},
		{
			name:    "AUTO head AND its members are stripped (only individuals remain)",
			input:   []config.ProxyEntry{autoHead, member1, member2, member3},
			wantIDs: nil,
		},
		{
			name:    "SECTION rows are hidden, standalone passes through",
			input:   []config.ProxyEntry{section, standalone},
			wantIDs: []string{"std-1"},
		},
		{
			name:    "AUTO head + members + standalone keeps ONLY the standalone (auto cluster is gone)",
			input:   []config.ProxyEntry{autoHead, member1, member2, member3, standalone},
			wantIDs: []string{"std-1"},
		},
		{
			name:    "entries that are not AUTO members and not AUTO heads pass through",
			input:   []config.ProxyEntry{member1, member2, standalone},
			wantIDs: []string{"m1", "m2", "std-1"},
		},
		{
			name:    "lowercase 'auto' type is also stripped (and its members)",
			input:   []config.ProxyEntry{{ID: "ah", Type: "auto", Name: "Smart", Extra: mustExtra(t, []string{"m1"})}, member1, standalone},
			wantIDs: []string{"std-1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := filterTrayProxies(tc.input)
			gotIDs := make([]string, 0, len(got))
			for _, p := range got {
				gotIDs = append(gotIDs, p.ID)
			}
			if !equalStringSlices(gotIDs, tc.wantIDs) {
				t.Fatalf("filterTrayProxies IDs mismatch:\ngot:  %v\nwant: %v", gotIDs, tc.wantIDs)
			}
		})
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
