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

package verdict

import (
	"net/netip"
	"testing"
	"time"
)

func TestThreeAgreeingSubdomainsPromoteToParent(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")

	s.Learn("a.blocked.example", Proxy)
	s.Learn("b.blocked.example", Proxy)
	if _, ok := s.Lookup("never-seen.blocked.example"); ok {
		t.Fatal("two subdomains must not be enough to condemn the parent")
	}

	s.Learn("c.blocked.example", Proxy)
	rec, ok := s.Lookup("never-seen.blocked.example")
	if !ok || rec.Decision != Proxy {
		t.Fatalf("three agreeing subdomains must promote: got %v, ok=%v", rec.Decision, ok)
	}
}

func TestDisagreeingSubdomainsDoNotPromote(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")

	s.Learn("a.mixed.example", Proxy)
	s.Learn("b.mixed.example", Direct)
	s.Learn("c.mixed.example", Proxy)

	if _, ok := s.Lookup("never-seen.mixed.example"); ok {
		t.Fatal("two proxy and one direct is not a majority of three agreeing")
	}
}

// The floor comment in internal/proxy/router.go:498-506 spells out why:
// google.com as a suffix would drag search and everything else into the
// tunnel. Promotion must never reach a bare TLD.
func TestPromotionNeverReachesTLD(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")

	s.Learn("one.example", Proxy)
	s.Learn("two.example", Proxy)
	s.Learn("three.example", Proxy)

	if _, ok := s.Lookup("unrelated.example"); ok {
		t.Fatal("promotion lifted a verdict onto the TLD")
	}
}

func TestIPVerdictsPromoteToPrefix(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")

	s.LearnIP(netip.MustParseAddr("149.154.167.41"), Proxy)
	s.LearnIP(netip.MustParseAddr("149.154.167.51"), Proxy)
	if _, ok := s.LookupIP(netip.MustParseAddr("149.154.167.99")); ok {
		t.Fatal("two neighbours must not be enough")
	}

	s.LearnIP(netip.MustParseAddr("149.154.167.91"), Proxy)
	rec, ok := s.LookupIP(netip.MustParseAddr("149.154.167.99"))
	if !ok || rec.Decision != Proxy {
		t.Fatalf("three agreeing neighbours must promote the /24: got %v, ok=%v", rec.Decision, ok)
	}
	if _, ok := s.LookupIP(netip.MustParseAddr("149.154.168.1")); ok {
		t.Fatal("promotion escaped the /24")
	}
}

func TestLookupIPPrefersExactOverPrefix(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")

	s.Seed("10.0.0.0/24", Proxy, SourceList)
	s.Seed("10.0.0.7", Direct, SourceList)

	rec, ok := s.LookupIP(netip.MustParseAddr("10.0.0.7"))
	if !ok || rec.Decision != Direct {
		t.Fatalf("exact address must win over its prefix: got %v, ok=%v", rec.Decision, ok)
	}
}
