// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package system

import (
	"reflect"
	"testing"
)

func TestExtractDNSIPs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "nil falls back to public defaults",
			in:   nil,
			want: []string{"1.1.1.1", "8.8.8.8"},
		},
		{
			name: "empty entries fall back to defaults",
			in:   []string{"", "   "},
			want: []string{"1.1.1.1", "8.8.8.8"},
		},
		{
			name: "hostnames are dropped (no resolution at kill-switch time)",
			in:   []string{"dns.google", "cloudflare-dns.com"},
			want: []string{"1.1.1.1", "8.8.8.8"},
		},
		{
			name: "valid IPv4 retained verbatim",
			in:   []string{"9.9.9.9"},
			want: []string{"9.9.9.9"},
		},
		{
			name: "host:port suffix stripped",
			in:   []string{"1.1.1.1:53", "9.9.9.9:5353"},
			want: []string{"1.1.1.1", "9.9.9.9"},
		},
		{
			name: "bracketed IPv6 with port",
			in:   []string{"[2606:4700:4700::1111]:53"},
			want: []string{"2606:4700:4700::1111"},
		},
		{
			name: "deduplicates while preserving order",
			in:   []string{"1.1.1.1", "8.8.8.8", "1.1.1.1"},
			want: []string{"1.1.1.1", "8.8.8.8"},
		},
		{
			name: "mixes valid and invalid; only valid survive",
			in:   []string{"1.1.1.1", "not-an-ip", "9.9.9.9"},
			want: []string{"1.1.1.1", "9.9.9.9"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractDNSIPs(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("extractDNSIPs(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// resolveProxyIPs must accept a comma-separated list of literal "host:port"
// entries and return EVERY IP — this is how the kill switch allows all of a CDN
// server's pinned backends so a mid-session failover isn't blocked. Pure
// literals must not trigger any DNS resolution.
func TestResolveProxyIPs_CommaSeparatedLiterals(t *testing.T) {
	got := resolveProxyIPs("203.0.113.7:443,203.0.113.8:443,203.0.113.7:443")
	want := []string{"203.0.113.7", "203.0.113.8"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveProxyIPs(list) = %v, want %v (deduped, order-stable)", got, want)
	}

	if got := resolveProxyIPs("198.51.100.10:8443"); !reflect.DeepEqual(got, []string{"198.51.100.10"}) {
		t.Fatalf("single literal host:port = %v, want [198.51.100.10]", got)
	}

	if got := resolveProxyIPs("[2606:4700::1111]:443,198.51.100.10:443"); !reflect.DeepEqual(got, []string{"2606:4700::1111", "198.51.100.10"}) {
		t.Fatalf("mixed v6/v4 list = %v", got)
	}
}
