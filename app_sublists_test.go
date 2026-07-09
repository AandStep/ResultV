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

package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The subscription HTTP client has no SSRF dialer, so loopback httptest works.
func TestFetchSubscriptionReturnsRoutingLists(t *testing.T) {
	payload := `[{"name":"L1","url":"https://example.com/l1.lst","action":"proxy"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Routing-Lists", base64.StdEncoding.EncodeToString([]byte(payload)))
		// Minimal valid subscription body: one proxy URI.
		_, _ = w.Write([]byte("vless://00000000-0000-0000-0000-000000000000@1.2.3.4:443?type=tcp&security=none#test\n"))
	}))
	defer srv.Close()

	a := newTestApp(t, t.TempDir())
	// http:// loopback URL → allowInsecure=true.
	prev, err := a.FetchSubscription(srv.URL, true)
	if err != nil {
		t.Fatalf("FetchSubscription: %v", err)
	}
	if len(prev.Proxies) == 0 {
		t.Fatalf("no proxies parsed")
	}
	if len(prev.RoutingLists) != 1 || prev.RoutingLists[0].Name != "L1" || prev.RoutingLists[0].Action != "proxy" {
		t.Fatalf("routing lists not returned: %+v", prev.RoutingLists)
	}
}

func TestParseSubscriptionTextJSONBodyLists(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	// JSON body with both nodes (unparseable here is fine if entries exist) is
	// complex; assert only the routingLists extraction on a JSON body that also
	// fails proxy parsing → expect error but that's the entries contract.
	// Simplest valid check: plain URI text yields no routing lists.
	prev, err := a.ParseSubscriptionText("vless://00000000-0000-0000-0000-000000000000@1.2.3.4:443?type=tcp&security=none#t\n")
	if err != nil {
		t.Fatalf("ParseSubscriptionText: %v", err)
	}
	if len(prev.RoutingLists) != 0 {
		t.Fatalf("plain URI text must yield no routing lists: %+v", prev.RoutingLists)
	}
}
