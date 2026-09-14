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

// A server added while the tunnel is up has no flag in the UI, and this is
// where it is lost. On Windows names are resolved by the DNS Client service,
// so with adaptive Smart running the answer for the node's own domain is a
// fake address out of 198.18.0.0/15 — measured 14.09.2026:
// cdn2.failusha.digital came back as 198.18.2.158. MaxMind has no country for
// benchmarking space, so the lookup fails and the row renders flagless
// forever.
func TestResolveToIPRejectsFakeAnswersAndFallsBackToDoH(t *testing.T) {
	swapCountryResolvers(t,
		func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("198.18.2.158")}}, nil
		},
		func(string) []string { return []string{"203.0.113.7"} },
	)
	got, err := resolveToIP(context.Background(), "cdn2.example.test")
	if err != nil {
		t.Fatalf("resolveToIP: %v", err)
	}
	if got != "203.0.113.7" {
		t.Fatalf("resolveToIP = %q, want the DoH answer 203.0.113.7", got)
	}
}

// A real address in the same answer is the one to use; the fake is skipped
// rather than making the whole lookup fall through to DoH.
func TestResolveToIPPrefersRealAddressOverFake(t *testing.T) {
	swapCountryResolvers(t,
		func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{
				{IP: net.ParseIP("198.18.0.9")},
				{IP: net.ParseIP("198.51.100.4")},
			}, nil
		},
		func(string) []string { t.Fatal("DoH must not be needed when a real address is present"); return nil },
	)
	got, err := resolveToIP(context.Background(), "node.example.test")
	if err != nil {
		t.Fatalf("resolveToIP: %v", err)
	}
	if got != "198.51.100.4" {
		t.Fatalf("resolveToIP = %q, want 198.51.100.4", got)
	}
}

// With nothing real anywhere, failing is the right answer: a fake address sent
// to the country API is a guaranteed wrong flag rather than a missing one.
func TestResolveToIPFailsRatherThanReturnFake(t *testing.T) {
	swapCountryResolvers(t,
		func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("198.18.2.158")}}, nil
		},
		func(string) []string { return nil },
	)
	if got, err := resolveToIP(context.Background(), "node.example.test"); err == nil {
		t.Fatalf("resolveToIP = %q, want an error", got)
	}
}

func swapCountryResolvers(
	t *testing.T,
	lookup func(context.Context, string) ([]net.IPAddr, error),
	doh func(string) []string,
) {
	t.Helper()
	prevLookup, prevDoH := countryLookupIPAddr, countryDoHResolve
	countryLookupIPAddr, countryDoHResolve = lookup, doh
	t.Cleanup(func() { countryLookupIPAddr, countryDoHResolve = prevLookup, prevDoH })
}
