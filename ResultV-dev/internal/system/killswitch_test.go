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
