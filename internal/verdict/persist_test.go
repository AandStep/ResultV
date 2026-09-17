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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	path := filepath.Join(t.TempDir(), "verdicts.cache.json")

	s, err := Load(path, clock)
	if err != nil {
		t.Fatalf("Load on a missing file must succeed: %v", err)
	}
	s.SetNamespace("home")
	s.Learn("kept.example", Proxy)
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}

	back, err := Load(path, clock)
	if err != nil {
		t.Fatal(err)
	}
	back.SetNamespace("home")
	rec, ok := back.Lookup("kept.example")
	if !ok || rec.Decision != Proxy {
		t.Fatalf("round trip lost the verdict: %v, ok=%v", rec.Decision, ok)
	}
}

// The file is the one artefact a VPN must not turn into a browsing history.
func TestFileHoldsNoPlaintextNames(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	path := filepath.Join(t.TempDir(), "verdicts.cache.json")

	s, _ := Load(path, clock)
	s.SetNamespace("home")
	s.Learn("very-private-site.example", Proxy)
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "very-private-site") {
		t.Fatal("a visited domain was written to disk in the clear")
	}
}

func TestSaveDropsExpiredRecords(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	path := filepath.Join(t.TempDir(), "verdicts.cache.json")

	s, _ := Load(path, clock)
	s.SetNamespace("home")
	s.Learn("gone.example", Direct)
	s.Learn("stays.example", Proxy)

	now = now.Add(TTLDirect + time.Hour)
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}

	back, _ := Load(path, clock)
	back.SetNamespace("home")
	if _, ok := back.Lookup("stays.example"); !ok {
		t.Error("a live verdict was pruned")
	}
	if len(back.spaces["home"]) != 1 {
		t.Errorf("expired record survived the save: %d entries", len(back.spaces["home"]))
	}
}

func TestLoadOfCorruptFileStartsClean(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verdicts.cache.json")
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path, nil)
	if err != nil {
		t.Fatalf("a corrupt cache must not be fatal: %v", err)
	}
	if s == nil || len(s.salt) == 0 {
		t.Fatal("Load must hand back a usable store with a fresh salt")
	}
}

func TestSaltSurvivesReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verdicts.cache.json")
	s, _ := Load(path, nil)
	first := string(s.salt)
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	back, _ := Load(path, nil)
	if string(back.salt) != first {
		t.Fatal("a new salt on every start would invalidate every stored key")
	}
}
