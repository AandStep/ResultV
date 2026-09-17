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
)

func TestNormalizeHost(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Example.COM", "example.com"},
		{"example.com.", "example.com"},
		{".example.com", "example.com"},
		{"example.com:443", "example.com"},
		{"[2606:4700::1]:443", "2606:4700::1"},
		{"2606:4700::1", "2606:4700::1"},
		{"  ", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeHost(c.in); got != c.want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A verdict on a bare TLD would route the whole internet, so Suffixes must
// stop before it — the same reasoning blockedDomainFloor already records for
// domain_suffix rules (internal/proxy/router.go:498-506).
func TestSuffixesStopsBeforeTLD(t *testing.T) {
	got := Suffixes("a.b.example.com")
	want := []string{"a.b.example.com", "b.example.com", "example.com"}
	if len(got) != len(want) {
		t.Fatalf("Suffixes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Suffixes = %v, want %v", got, want)
		}
	}
	if single := Suffixes("localhost"); len(single) != 1 || single[0] != "localhost" {
		t.Errorf("Suffixes(localhost) = %v, want [localhost]", single)
	}
	if none := Suffixes(""); none != nil {
		t.Errorf("Suffixes(\"\") = %v, want nil", none)
	}
}

func TestParentDomain(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a.example.com", "example.com"},
		{"example.com", ""},
		{"localhost", ""},
	}
	for _, c := range cases {
		if got := ParentDomain(c.in); got != c.want {
			t.Errorf("ParentDomain(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSourceOrdering(t *testing.T) {
	if !(SourceUser > SourceFloor && SourceFloor > SourceLearned && SourceLearned > SourceList) {
		t.Fatal("source constants must rank user > floor > learned > list")
	}
}

func TestIPKeys(t *testing.T) {
	v4 := netip.MustParseAddr("149.154.167.51")
	if got := IPKey(v4); got != "149.154.167.51" {
		t.Errorf("IPKey = %q", got)
	}
	if got := IPParent(v4); got != "149.154.167.0/24" {
		t.Errorf("IPParent = %q, want 149.154.167.0/24", got)
	}
	v6 := netip.MustParseAddr("2001:db8:abcd:1234::1")
	if got := IPParent(v6); got != "2001:db8:abcd::/48" {
		t.Errorf("IPParent v6 = %q, want 2001:db8:abcd::/48", got)
	}
	if got := IPKey(netip.Addr{}); got != "" {
		t.Errorf("IPKey(invalid) = %q, want empty", got)
	}
}
