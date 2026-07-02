// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

import "testing"

func TestDefaultSources_IncludesAnnoyancesAndMobileAds(t *testing.T) {
	names := make(map[string]bool, len(DefaultSources))
	for _, s := range DefaultSources {
		names[s.Name] = true
	}
	if !names["adguard-annoyances"] {
		t.Fatal("expected DefaultSources to include adguard-annoyances")
	}
	if !names["adguard-mobile-ads"] {
		t.Fatal("expected DefaultSources to include adguard-mobile-ads")
	}
}
