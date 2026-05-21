// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

import (
	"net"
	"testing"
)

// SSRF guard: the icon fetcher must reject every address range a hostile
// subscription server could use to probe the user's LAN. Without this, the
// "Profile-Icon-Url" header could point at http://192.168.1.1/ and use
// response timing as an oracle for what's behind the firewall.
func TestIsPrivateOrLoopbackBlocksUnsafeRanges(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		// Public — must be allowed.
		{"1.1.1.1", false},
		{"8.8.8.8", false},
		{"2606:4700:4700::1111", false},
		// Loopback.
		{"127.0.0.1", true},
		{"127.0.0.42", true},
		{"::1", true},
		// RFC1918.
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.0.1", true},
		{"192.168.1.1", true},
		// Link-local.
		{"169.254.0.1", true},
		{"fe80::1", true},
		// CGNAT — same risk profile (carrier-internal infra).
		{"100.64.0.1", true},
		{"100.127.255.255", true},
		// Unique-local v6.
		{"fc00::1", true},
		{"fd00::1", true},
		// Unspecified / broadcast / multicast.
		{"0.0.0.0", true},
		{"255.255.255.255", true},
		{"224.0.0.1", true},
		{"ff02::1", true},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("ParseIP(%q) returned nil", tc.ip)
			}
			if got := isPrivateOrLoopback(ip); got != tc.want {
				t.Fatalf("isPrivateOrLoopback(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

// Referer must only be attached when the icon host matches the subscription
// host. Cross-host icons must not receive the subscription URL (which often
// contains an access token in the path) in the Referer header.
func TestSameHostRefererOnlyOnMatchingHost(t *testing.T) {
	cases := []struct {
		name     string
		referer  string
		imageURL string
		want     string
	}{
		{
			name:     "same host attaches referer",
			referer:  "https://provider.com/sub?token=secret",
			imageURL: "https://provider.com/icon.png",
			want:     "https://provider.com/sub?token=secret",
		},
		{
			name:     "different host strips referer",
			referer:  "https://provider.com/sub?token=secret",
			imageURL: "https://cdn.example.com/icon.png",
			want:     "",
		},
		{
			name:     "case-insensitive host match",
			referer:  "https://Provider.COM/sub",
			imageURL: "https://provider.com/icon.png",
			want:     "https://Provider.COM/sub",
		},
		{
			name:     "empty referer returns empty",
			referer:  "",
			imageURL: "https://provider.com/icon.png",
			want:     "",
		},
		{
			name:     "malformed referer rejected",
			referer:  "not a url",
			imageURL: "https://provider.com/icon.png",
			want:     "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameHostReferer(tc.referer, tc.imageURL); got != tc.want {
				t.Fatalf("sameHostReferer(%q, %q) = %q, want %q",
					tc.referer, tc.imageURL, got, tc.want)
			}
		})
	}
}
