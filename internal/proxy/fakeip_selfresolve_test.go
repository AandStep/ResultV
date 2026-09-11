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
)

func TestRealIPv4sDropsFakeAddresses(t *testing.T) {
	in := []net.IPAddr{
		{IP: net.ParseIP("198.18.0.224")},
		{IP: net.ParseIP("203.0.113.7")},
		{IP: net.ParseIP("2606:4700::1")},
		{IP: net.ParseIP("198.19.1.1")},
		{IP: net.ParseIP("203.0.113.8")},
	}
	got := realIPv4s(in)
	if len(got) != 2 || got[0] != "203.0.113.7" || got[1] != "203.0.113.8" {
		t.Fatalf("realIPv4s = %v, want the two real v4 addresses", got)
	}
	if len(realIPv4s([]net.IPAddr{{IP: net.ParseIP("198.18.0.1")}})) != 0 {
		t.Fatal("an all-fake answer must come back empty so the caller falls back")
	}
}

// The server pin is the worst place a fake address can land: it feeds
// route_exclude_address and the hosts record, so pinning 198.18.x.x sends the
// tunnel's own packets back into the tunnel. It is reachable on a reconnect
// that happens while the TUN is already up — a mode switch or a routing-rule
// change.
func TestResolvePinnedServerIPRejectsAFakeAddress(t *testing.T) {
	prev := selfLookupIPAddr
	selfLookupIPAddr = func(host string) []net.IPAddr {
		return []net.IPAddr{{IP: net.ParseIP("198.18.0.224")}}
	}
	t.Cleanup(func() { selfLookupIPAddr = prev })

	prevDoH := selfDoHResolve
	selfDoHResolve = func(host string) []string { return nil }
	t.Cleanup(func() { selfDoHResolve = prevDoH })

	if got := resolvePinnedServerIP("node.example.com"); got != "" {
		t.Fatalf("resolvePinnedServerIP = %q, want empty rather than a fake address", got)
	}
	if got := resolveAllServerIPs("node.example.com"); len(got) != 0 {
		t.Fatalf("resolveAllServerIPs = %v, want empty rather than fake addresses", got)
	}
}

// With DoH able to answer, the connect path gets the truth instead of nothing.
func TestResolveServerIPsFallsBackToDoHWhenOnlyFakeAnswers(t *testing.T) {
	prev := selfLookupIPAddr
	selfLookupIPAddr = func(host string) []net.IPAddr {
		return []net.IPAddr{{IP: net.ParseIP("198.18.0.224")}}
	}
	t.Cleanup(func() { selfLookupIPAddr = prev })

	prevDoH := selfDoHResolve
	selfDoHResolve = func(host string) []string { return []string{"203.0.113.7", "203.0.113.8"} }
	t.Cleanup(func() { selfDoHResolve = prevDoH })

	if got := resolvePinnedServerIP("node.example.com"); got != "203.0.113.7" {
		t.Fatalf("resolvePinnedServerIP = %q, want the DoH answer", got)
	}
	got := resolveAllServerIPs("node.example.com")
	if len(got) != 2 || got[0] != "203.0.113.7" {
		t.Fatalf("resolveAllServerIPs = %v, want both DoH answers", got)
	}
}

// The AUTO sweep dials its candidates bound to the physical adapter, exactly
// like the pings do, so a fake address makes every candidate time out and the
// sweep picks nothing.
func TestAutoProbeResolveHostRejectsAFakeAddress(t *testing.T) {
	prev := autoProbeLookupIPAddr
	autoProbeLookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("198.18.0.224")}}, nil
	}
	t.Cleanup(func() { autoProbeLookupIPAddr = prev })

	prevDoH := selfDoHResolve
	selfDoHResolve = func(host string) []string { return nil }
	t.Cleanup(func() { selfDoHResolve = prevDoH })

	if ip, ok := autoProbeResolveHost(context.Background(), "node.example.com"); ok {
		t.Fatalf("autoProbeResolveHost accepted a fake address: %q", ip)
	}
}
