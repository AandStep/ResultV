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
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// A subnet sold to another country keeps its old flag until the cache entry
// ages out; the refresh button has to see the new answer immediately.
func TestRefreshCountryByIPBypassesCache(t *testing.T) {
	var answer atomic.Value
	answer.Store("ie")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"country":"%s"}`, answer.Load())
	}))
	defer srv.Close()

	c := &CountryClient{
		httpClient: srv.Client(),
		apiURL:     srv.URL,
		cachePath:  filepath.Join(t.TempDir(), "country.cache.json"),
	}
	ctx := context.Background()
	const ip = "78.17.18.170"

	if got, err := c.LookupCountryByIP(ctx, ip); err != nil || got != "ie" {
		t.Fatalf("first lookup = %q, %v", got, err)
	}
	answer.Store("fi")
	if got, _ := c.LookupCountryByIP(ctx, ip); got != "ie" {
		t.Fatalf("cached lookup = %q, want ie", got)
	}
	if got, err := c.RefreshCountryByIP(ctx, ip); err != nil || got != "fi" {
		t.Fatalf("refresh = %q, %v, want fi", got, err)
	}
	if got, _ := c.LookupCountryByIP(ctx, ip); got != "fi" {
		t.Fatalf("lookup after refresh = %q, want fi", got)
	}
}
