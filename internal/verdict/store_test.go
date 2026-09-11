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
	"testing"
	"time"
)

func testStore(now *time.Time) *Store {
	return New([]byte("test-salt"), func() time.Time { return *now })
}

func TestLookupPrefersStrongerSourceOverSpecificity(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")

	// The engine measured the child and guessed "tunnel it".
	s.Learn("a.example.com", Proxy)
	// The user said the whole domain goes direct. That must win.
	s.Seed("example.com", Direct, SourceUser)

	rec, ok := s.Lookup("a.example.com")
	if !ok {
		t.Fatal("expected a verdict")
	}
	if rec.Decision != Direct || rec.Source != SourceUser {
		t.Fatalf("got %v from %v, want direct from user", rec.Decision, rec.Source)
	}
}

func TestLookupPrefersSpecificWhenSourcesTie(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")
	s.Learn("example.com", Direct)
	s.Learn("a.example.com", Proxy)

	rec, _ := s.Lookup("a.example.com")
	if rec.Decision != Proxy {
		t.Fatalf("got %v, want the more specific proxy verdict", rec.Decision)
	}
}

func TestLearnedExpiresOnItsOwnClock(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")
	s.Learn("clean.example", Direct)
	s.Learn("blocked.example", Proxy)

	now = now.Add(TTLDirect + time.Minute)
	if _, ok := s.Lookup("clean.example"); ok {
		t.Error("a clean verdict must not outlive TTLDirect")
	}
	if _, ok := s.Lookup("blocked.example"); !ok {
		t.Error("a blocked verdict must still be alive at TTLDirect + a minute")
	}

	now = now.Add(TTLProxy)
	if _, ok := s.Lookup("blocked.example"); ok {
		t.Error("a blocked verdict must not outlive TTLProxy")
	}
}

func TestSeededSourcesNeverExpire(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")
	s.Seed("floor.example", Proxy, SourceFloor)

	now = now.Add(365 * 24 * time.Hour)
	if _, ok := s.Lookup("floor.example"); !ok {
		t.Error("floor entries are replaced, never aged out")
	}
}

func TestWeakerSourceNeverOverwritesStronger(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")
	s.Seed("x.example", Proxy, SourceUser)
	s.Seed("x.example", Direct, SourceList)

	rec, _ := s.Lookup("x.example")
	if rec.Decision != Proxy || rec.Source != SourceUser {
		t.Fatalf("a list entry overwrote a user rule: %v from %v", rec.Decision, rec.Source)
	}
}

// Censorship is a property of the network, not of the laptop. What was blocked
// on a home ISP says nothing about the same name on a hotel Wi-Fi.
func TestNamespacesAreIsolated(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")
	s.Learn("y.example", Proxy)

	s.SetNamespace("cafe")
	if _, ok := s.Lookup("y.example"); ok {
		t.Error("a verdict leaked across networks")
	}

	s.SetNamespace("home")
	if _, ok := s.Lookup("y.example"); !ok {
		t.Error("switching back to a known network must not lose what it knew")
	}
}

func TestNamesReturnsPlaintextForDiagnostics(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")
	s.Learn("visible.example", Proxy)

	names := s.Names()
	if rec, ok := names["visible.example"]; !ok || rec.Decision != Proxy {
		t.Fatalf("Names() = %v, want visible.example -> proxy", names)
	}
}

func TestUnknownDecisionIsNotStored(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	s := testStore(&now)
	s.SetNamespace("home")
	s.Learn("nothing.example", Unknown)
	if _, ok := s.Lookup("nothing.example"); ok {
		t.Error("Unknown is the absence of a verdict, not a verdict")
	}
}
