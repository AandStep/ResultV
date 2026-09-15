// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package proxy

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestIsFakeIPAddr(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"198.18.0.224", true},
		{"198.19.255.255", true},
		{"198.17.255.255", false},
		{"198.20.0.1", false},
		{"1.1.1.1", false},
		{"192.168.0.11", false},
		{"fc00::1", true},
		{"fdfe:dcba:9876::1", false},
		{"2606:4700::1", false},
		{"", false},
		{"not-an-ip", false},
	}
	for _, c := range cases {
		if got := isFakeIPAddr(net.ParseIP(c.in)); got != c.want {
			t.Errorf("isFakeIPAddr(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// The measured failure: with FakeIP on, the OS resolver hands EVERY process a
// 198.18.x.x address, this one included. Windows resolves names in the DNS
// Client service, not in the calling process, so the process_path_regex rule
// that was supposed to exempt our own binary never matches its lookups.
//
// A fake address is harmless while the connection goes through the TUN, and
// fatal the moment we deliberately bypass it: every LAN-bound ping dialled an
// address that only exists inside the tunnel and timed out after five seconds.
func TestResolvePingHostRejectsAFakeAddressAndFallsBackToDoH(t *testing.T) {
	resetPingResolveCache()
	t.Cleanup(resetPingResolveCache)

	prevLookup := pingLookupIPAddr
	pingLookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("198.18.0.224")}}, nil
	}
	t.Cleanup(func() { pingLookupIPAddr = prevLookup })

	prevDoH := pingDoHResolve
	pingDoHResolve = func(host string) []string { return []string{"203.0.113.9"} }
	t.Cleanup(func() { pingDoHResolve = prevDoH })

	if got := resolvePingHost("node.example.com"); got != "203.0.113.9" {
		t.Fatalf("resolvePingHost = %q, want the real address from DoH", got)
	}
}

// Caching a fake address is what made the breakage outlive the session: the
// entry lives five minutes, so pings kept timing out after disconnecting.
func TestResolvePingHostNeverCachesAFakeAddress(t *testing.T) {
	resetPingResolveCache()
	t.Cleanup(resetPingResolveCache)

	prevLookup := pingLookupIPAddr
	pingLookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("198.18.0.224")}}, nil
	}
	t.Cleanup(func() { pingLookupIPAddr = prevLookup })

	prevDoH := pingDoHResolve
	pingDoHResolve = func(host string) []string { return nil }
	t.Cleanup(func() { pingDoHResolve = prevDoH })

	if got := resolvePingHost("node.example.com"); got != "" {
		t.Fatalf("resolvePingHost = %q, want empty when nothing real resolved", got)
	}
	if ip, ok := lookupPingResolveCache("node.example.com", time.Now()); ok {
		t.Fatalf("a fake address was cached as %q", ip)
	}
}

// A real answer still goes through untouched, cache included.
func TestResolvePingHostKeepsRealAnswers(t *testing.T) {
	resetPingResolveCache()
	t.Cleanup(resetPingResolveCache)

	prevLookup := pingLookupIPAddr
	pingLookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.7")}}, nil
	}
	t.Cleanup(func() { pingLookupIPAddr = prevLookup })

	if got := resolvePingHost("node.example.com"); got != "203.0.113.7" {
		t.Fatalf("resolvePingHost = %q, want 203.0.113.7", got)
	}
	if ip, ok := lookupPingResolveCache("node.example.com", time.Now()); !ok || ip != "203.0.113.7" {
		t.Fatalf("a real address was not cached: %q ok=%v", ip, ok)
	}
}
