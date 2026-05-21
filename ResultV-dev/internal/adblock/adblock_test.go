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

package adblock

import (
	"os"
	"testing"
)

func TestFallbackDomains(t *testing.T) {
	b := New()

	if !b.IsAdDomain("doubleclick.net") {
		t.Error("doubleclick.net should be blocked by default")
	}
	if !b.IsAdDomain("googlesyndication.com") {
		t.Error("googlesyndication.com should be blocked by default")
	}
	if b.IsAdDomain("google.com") {
		t.Error("google.com should NOT be blocked")
	}
}

func TestSuffixMatch(t *testing.T) {
	b := New()

	
	if !b.IsAdDomain("ads.doubleclick.net") {
		t.Error("ads.doubleclick.net should match doubleclick.net")
	}
	
	if !b.IsAdDomain("pagead2.googlesyndication.com") {
		t.Error("pagead2.googlesyndication.com should match")
	}
}

func TestEmptyHostname(t *testing.T) {
	b := New()
	if b.IsAdDomain("") {
		t.Error("empty hostname should not be blocked")
	}
}

func TestCaseInsensitive(t *testing.T) {
	b := New()
	if !b.IsAdDomain("DoubleClick.Net") {
		t.Error("should be case insensitive")
	}
}

func TestLoadFromCache(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := tmpDir + "/adblock_domains.txt"
	os.WriteFile(cacheFile, []byte("# comment\ncustom-ad.com\ntracking.io\n"), 0o644)

	b := New()
	initial := b.GetDomainCount()

	if err := b.LoadFromCache(tmpDir); err != nil {
		t.Fatalf("LoadFromCache failed: %v", err)
	}

	after := b.GetDomainCount()
	if after <= initial {
		t.Errorf("expected more domains after cache load, before=%d after=%d", initial, after)
	}

	if !b.IsAdDomain("custom-ad.com") {
		t.Error("custom-ad.com should be blocked after cache load")
	}
}

func TestGetDomains(t *testing.T) {
	b := New()
	domains := b.GetDomains()

	if len(domains) != len(fallbackAdDomains) {
		t.Errorf("expected %d domains, got %d", len(fallbackAdDomains), len(domains))
	}
}
