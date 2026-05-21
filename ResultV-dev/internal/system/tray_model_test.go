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

package system

import (
	"strings"
	"testing"

	"resultproxy-wails/internal/config"
)

func TestBuildTrayMenuGroups_SortsAndFallbacks(t *testing.T) {
	proxies := []config.ProxyEntry{
		{ID: "1", Name: "B-node", Country: "de", Provider: "Beta"},
		{ID: "2", Name: "A-node", Country: "us", Provider: ""},
		{ID: "3", Name: "C-node", Country: "", Provider: "Beta"},
		{ID: "4", Name: "D-node", Country: "de", Provider: "Alpha"},
	}

	got := BuildTrayMenuGroups(proxies, 20)
	if len(got) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(got))
	}

	if got[0].Provider != "Alpha" || got[1].Provider != "Beta" || got[2].Provider != "Мои прокси" {
		t.Fatalf("unexpected provider sort/fallback: %+v", []string{got[0].Provider, got[1].Provider, got[2].Provider})
	}

	beta := got[1]
	if len(beta.Countries) != 2 {
		t.Fatalf("expected 2 countries in Beta, got %d", len(beta.Countries))
	}
	if beta.Countries[0].Country != "DE" || beta.Countries[1].Country != "Unknown" {
		t.Fatalf("unexpected country sort/fallback in Beta: %+v", []string{beta.Countries[0].Country, beta.Countries[1].Country})
	}
}

func TestBuildTrayMenuGroups_RespectsPerCountryLimit(t *testing.T) {
	proxies := []config.ProxyEntry{
		{ID: "1", Name: "A", Country: "US", Provider: "P"},
		{ID: "2", Name: "B", Country: "US", Provider: "P"},
		{ID: "3", Name: "C", Country: "US", Provider: "P"},
	}

	got := BuildTrayMenuGroups(proxies, 2)
	if len(got) != 1 || len(got[0].Countries) != 1 {
		t.Fatalf("unexpected grouping shape: %+v", got)
	}
	country := got[0].Countries[0]
	if len(country.Servers) != 2 {
		t.Fatalf("expected 2 visible servers, got %d", len(country.Servers))
	}
	if country.HiddenCount != 1 {
		t.Fatalf("expected hiddenCount=1, got %d", country.HiddenCount)
	}
}

func TestCountryToFlag(t *testing.T) {
	if got := countryToFlag("de"); got != "🇩🇪" {
		t.Fatalf("expected DE flag, got %q", got)
	}
	if got := countryToFlag("Unknown"); got != "🌐" {
		t.Fatalf("expected globe fallback, got %q", got)
	}
}

func TestFormatCountryTitle(t *testing.T) {
	got := formatCountryTitle("fi")
	if !strings.Contains(got, "Финляндия") {
		t.Fatalf("expected localized country name, got %q", got)
	}
	if strings.Contains(strings.ToLower(got), "fi") {
		t.Fatalf("expected no country code in title, got %q", got)
	}
}

func TestFormatServerTitle(t *testing.T) {
	server := TrayServer{
		ID:      "x",
		Name:    "Berlin #1",
		Country: "DE",
		PingMs:  73,
	}
	got := formatServerTitle(server, true)
	if got == "" {
		t.Fatal("title must not be empty")
	}
	if !strings.HasPrefix(got, "✅") {
		t.Fatalf("expected connected marker prefix, got %q", got)
	}
}

func TestFormatServerTitle_StripsCountryCodePrefixFromServerName(t *testing.T) {
	server := TrayServer{
		ID:      "x",
		Name:    "kz Kazakhstan [QazNode-1]",
		Country: "KZ",
		PingMs:  41,
	}
	got := formatServerTitle(server, false)
	if strings.Contains(strings.ToLower(got), "kz kazakhstan") {
		t.Fatalf("expected country code prefix to be removed from server name, got %q", got)
	}
	if !strings.Contains(got, "Kazakhstan [QazNode-1]") {
		t.Fatalf("expected server name to keep readable country label, got %q", got)
	}
}

func TestCountryISOCode(t *testing.T) {
	if got := countryISOCode("kz"); got != "kz" {
		t.Fatalf("expected kz, got %q", got)
	}
	if got := countryISOCode("Unknown"); got != "" {
		t.Fatalf("expected empty for unknown, got %q", got)
	}
	if got := countryISOCode("Kazakhstan"); got != "" {
		t.Fatalf("expected empty for non-iso country value, got %q", got)
	}
}
