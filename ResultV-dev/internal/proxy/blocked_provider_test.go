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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// newTestCountryServer spins up a TLS server that mimics the project's
// api.php response shape. The returned client has a CA pool that trusts the
// httptest cert.
func newTestCountryServer(t *testing.T, hits *int32, body string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api.php", func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	return httptest.NewTLSServer(mux)
}

func TestHTTPBlockedListProvider(t *testing.T) {
	var hits int32
	countrySrv := newTestCountryServer(t, &hits, `{"country":"RU","country_lower":"ru"}`)
	defer countrySrv.Close()

	listMux := http.NewServeMux()
	listMux.HandleFunc("/list/ru.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("discord.com\nytimg.com\n"))
	})
	listSrv := httptest.NewServer(listMux)
	defer listSrv.Close()

	t.Setenv("RESULTPROXY_COUNTRY_API_URL", countrySrv.URL+"/api.php")
	cc := NewCountryClient(t.TempDir())
	cc.httpClient = countrySrv.Client()

	p := &HTTPBlockedListProvider{
		Client:          listSrv.Client(),
		Country:         cc,
		ListURLTemplate: listSrv.URL + "/list/{country}.txt",
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

// Calling ResolveCountry twice should hit the network only once — the
// CountryClient caches "self" for countryCacheTTL.
func TestCountryClientCachesSelfLookup(t *testing.T) {
	var hits int32
	srv := newTestCountryServer(t, &hits, `{"country":"DE","country_lower":"de"}`)
	defer srv.Close()

	cc := NewCountryClient(t.TempDir())
	cc.apiURL = srv.URL + "/api.php"
	cc.httpClient = srv.Client()

	for i := 0; i < 3; i++ {
		country, err := cc.LookupSelfCountry(context.Background())
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if country != "de" {
			t.Fatalf("call %d: expected de, got %s", i, country)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected 1 network hit (cached after first), got %d", got)
	}
}

// Per-IP cache: same IP queried twice = one network call; different IPs =
// separate calls.
func TestCountryClientCachesByIP(t *testing.T) {
	var hits int32
	srv := newTestCountryServer(t, &hits, `{"country":"US","country_lower":"us"}`)
	defer srv.Close()

	cc := NewCountryClient(t.TempDir())
	cc.apiURL = srv.URL + "/api.php"
	cc.httpClient = srv.Client()

	for i := 0; i < 2; i++ {
		if _, err := cc.LookupCountryByIP(context.Background(), "1.2.3.4"); err != nil {
			t.Fatalf("call %d for 1.2.3.4: %v", i, err)
		}
	}
	if _, err := cc.LookupCountryByIP(context.Background(), "5.6.7.8"); err != nil {
		t.Fatalf("call for 5.6.7.8: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("expected 2 hits (one per unique IP), got %d", got)
	}
}

// Hostname inputs (e.g. AmneziaWG `game.sunsetglow.today`) must be resolved
// via DNS before being sent to the API — MaxMind only understands IP
// literals and would return "??" otherwise. The query that reaches the
// upstream server must NOT contain the raw hostname.
func TestCountryClientResolvesHostnameBeforeQuery(t *testing.T) {
	var seen string
	mux := http.NewServeMux()
	mux.HandleFunc("/api.php", func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Query().Get("ip")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"country":"US","country_lower":"us"}`))
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	cc := NewCountryClient(t.TempDir())
	cc.apiURL = srv.URL + "/api.php"
	cc.httpClient = srv.Client()

	// localhost resolves to 127.0.0.1 on every CI host — used as a stand-in
	// for an arbitrary hostname so the test doesn't depend on external DNS.
	country, err := cc.LookupCountryByIP(context.Background(), "localhost")
	if err != nil {
		t.Fatalf("lookup hostname: %v", err)
	}
	if country != "us" {
		t.Fatalf("expected us, got %s", country)
	}
	if seen == "localhost" {
		t.Fatalf("raw hostname leaked to upstream API: %q", seen)
	}
	if net.ParseIP(seen) == nil {
		t.Fatalf("API received non-IP value: %q", seen)
	}
}

// Plaintext HTTP overrides must be rejected even when the env var points at
// http://... — the whole purpose of this client is no plaintext IP traffic.
func TestCountryClientRefusesPlaintext(t *testing.T) {
	cc := NewCountryClient(t.TempDir())
	cc.apiURL = "http://attacker.example/api.php"

	_, err := cc.LookupSelfCountry(context.Background())
	if err == nil {
		t.Fatal("expected error for plaintext http URL, got nil")
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
