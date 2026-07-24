// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"strings"
	"testing"
)

// TestDefaultSources_RUExcludesCitizenlab pins that the RU blocklist is built
// only from real block sources. Citizenlab test-lists are censorship-measurement
// probe lists (popular/control sites incl. ok.ru, mail.ru, vk.com, yandex.ru and
// even state media), NOT blocklists — including them routed those sites through
// the proxy and wrongly matched their apps into the Smart per-app allowlist.
func TestDefaultSources_RUExcludesCitizenlab(t *testing.T) {
	ru := defaultPublicSourceTemplates("ru")
	if len(ru) == 0 {
		t.Fatal("ru sources must not be empty")
	}
	for _, s := range ru {
		if strings.Contains(s, "citizenlab") {
			t.Errorf("citizenlab must not be an RU block source, got %q", s)
		}
	}
	// The real RKN-based list must still be present.
	joined := strings.Join(ru, "\n")
	if !strings.Contains(joined, "Re-filter-lists/main/domains_all.lst") {
		t.Errorf("expected the Re-filter domains_all list in RU sources, got %v", ru)
	}
}

// TestDefaultSources_NonRUKeepsCitizenlab keeps the fallback for countries that
// have no dedicated curated list — citizenlab is imperfect but the only source.
func TestDefaultSources_NonRUKeepsCitizenlab(t *testing.T) {
	de := defaultPublicSourceTemplates("de")
	if len(de) == 0 {
		t.Fatal("non-ru sources must not be empty")
	}
	if !strings.Contains(strings.Join(de, "\n"), "citizenlab") {
		t.Errorf("non-ru should fall back to citizenlab, got %v", de)
	}
}
