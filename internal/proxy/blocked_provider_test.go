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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPBlockedListProvider(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/country", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"country":"RU"}`))
	})
	mux.HandleFunc("/list/ru.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("discord.com\nytimg.com\n"))
	})
	// resolveCountryFromEndpoint enforces HTTPS for the country API, so the
	// test server must serve TLS; srv.Client() trusts the self-signed cert.
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	p := &HTTPBlockedListProvider{
		Client:          srv.Client(),
		CountryURL:      srv.URL + "/country",
		ListURLTemplate: srv.URL + "/list/{country}.txt",
	}

	country, err := p.ResolveCountry(context.Background())
	if err != nil {
		t.Fatalf("resolve country: %v", err)
	}
	if country != "ru" {
		t.Fatalf("expected ru, got %s", country)
	}
	domains, err := p.FetchBlockedDomains(context.Background(), country)
	if err != nil {
		t.Fatalf("fetch domains: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(domains))
	}
}

func TestResolveCountry_FallbackEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/bad", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"countryCode":"DE"}`))
	})
	// HTTPS-only country API (see resolveCountryFromEndpoint); srv.Client()
	// carries the CA pool that trusts the httptest cert.
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	p := &HTTPBlockedListProvider{
		Client:     srv.Client(),
		CountryURL: srv.URL + "/bad",
	}
	orig := os.Getenv("RESULTPROXY_COUNTRY_SOURCES")
	_ = os.Setenv("RESULTPROXY_COUNTRY_SOURCES", srv.URL+"/bad,"+srv.URL+"/ok")
	defer func() { _ = os.Setenv("RESULTPROXY_COUNTRY_SOURCES", orig) }()

	country, err := p.ResolveCountry(context.Background())
	if err != nil {
		t.Fatalf("resolve country with fallback: %v", err)
	}
	if country != "de" {
		t.Fatalf("expected de, got %s", country)
	}
}

func TestBlockedDomainsCacheLoadSave(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "blocked_cache.json")
	err := SaveBlockedDomainsCache(cachePath, BlockedDomainsCache{
		Country: "ru",
		Source:  "remote",
		Domains: []string{"Example.com", "example.com", "ytimg.com"},
	})
	if err != nil {
		t.Fatalf("save cache: %v", err)
	}
	cache, err := LoadBlockedDomainsCache(cachePath)
	if err != nil {
		t.Fatalf("load cache: %v", err)
	}
	if cache.Country != "ru" {
		t.Fatalf("expected country ru, got %s", cache.Country)
	}
	if len(cache.Domains) != 2 {
		t.Fatalf("expected 2 normalized domains, got %d", len(cache.Domains))
	}
}

func TestResolveBlockedDomains_FallbackToLocal(t *testing.T) {
	localFile := filepath.Join(t.TempDir(), "list.txt")
	if err := os.WriteFile(localFile, []byte("local-only.com\n"), 0o600); err != nil {
		t.Fatalf("write local list: %v", err)
	}
	res := ResolveBlockedDomains(context.Background(), nil, filepath.Join(t.TempDir(), "missing.json"), localFile)
	if res.Source != "local" {
		t.Fatalf("expected local source, got %s", res.Source)
	}
	if len(res.Domains) != 1 || res.Domains[0] != "local-only.com" {
		t.Fatalf("unexpected local domains: %v", res.Domains)
	}
}

func TestParseDomainPayload_DNSMasqAndHosts(t *testing.T) {
	payload := []byte(`
server=/youtube.com/127.0.0.1
0.0.0.0 facebook.com
||discord.com^
# comment
`)
	domains := parseDomainPayload(payload)
	if len(domains) != 3 {
		t.Fatalf("expected 3 domains, got %d (%v)", len(domains), domains)
	}
}

func TestParseDomainPayload_CSV(t *testing.T) {
	payload := []byte("url,category\nhttps://example.com,NEWS\nhttp://sub.domain.org/path,MEDIA\n")
	domains := parseDomainPayload(payload)
	if len(domains) != 2 {
		t.Fatalf("expected 2 domains, got %d (%v)", len(domains), domains)
	}
	if domains[0] != "example.com" && domains[1] != "example.com" {
		t.Fatalf("expected example.com in parsed domains: %v", domains)
	}
}

// Ported from the desktop tree (ResultV-main) together with the expanded
// source list — see the smart-coverage design. Mobile shipped only citizenlab
// + itdog inside-raw (~3.8k domains) while desktop pulls Re:filter's ~86k.
func TestCompressDomainSuffixes(t *testing.T) {
	in := []string{"discord.com", "cdn.discord.com", "gg.discord.com", "x.com", "api.x.com", "unrelated.net"}
	got := compressDomainSuffixes(in)
	want := []string{"discord.com", "x.com", "unrelated.net"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v (order must be preserved)", got, want)
		}
	}
}

func TestDefaultPublicSourceTemplatesRU(t *testing.T) {
	sources := defaultPublicSourceTemplates("ru")
	wantContains := []string{
		"citizenlab/test-lists/master/lists/global.csv",
		"citizenlab/test-lists/master/lists/ru.csv",
		"itdoginfo/allow-domains/main/Russia/inside-raw.lst",
		"1andrevich/Re-filter-lists/main/domains_all.lst",
		"1andrevich/Re-filter-lists/main/community.lst",
		"itdoginfo/allow-domains/main/Services/discord.lst",
		"itdoginfo/allow-domains/main/Services/youtube.lst",
		"itdoginfo/allow-domains/main/Services/google_ai.lst",
		"Flowseal/zapret-discord-youtube/main/lists/list-general.txt",
	}
	for _, frag := range wantContains {
		found := false
		for _, s := range sources {
			if strings.Contains(s, frag) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ru sources missing %q; got %v", frag, sources)
		}
	}
	for _, s := range sources {
		if strings.Contains(s, "inside-dnsmasq-nfset") {
			t.Error("dnsmasq duplicate source must be removed")
		}
	}
	if got := defaultPublicSourceTemplates("de"); len(got) != 2 {
		t.Errorf("non-ru countries keep citizenlab only, got %v", got)
	}
}
