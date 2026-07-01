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
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestParseDoHJSONAnswer(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"a records", `{"Status":0,"Answer":[{"type":1,"data":"1.2.3.4"},{"type":1,"data":"5.6.7.8"}]}`, []string{"1.2.3.4", "5.6.7.8"}},
		{"skips cname and aaaa", `{"Answer":[{"type":5,"data":"cname."},{"type":28,"data":"2606::1"},{"type":1,"data":"9.9.9.9"}]}`, []string{"9.9.9.9"}},
		{"dedupes", `{"Answer":[{"type":1,"data":"1.1.1.1"},{"type":1,"data":"1.1.1.1"}]}`, []string{"1.1.1.1"}},
		{"ignores non-ip data", `{"Answer":[{"type":1,"data":"garbage"},{"type":1,"data":"8.8.8.8"}]}`, []string{"8.8.8.8"}},
		{"empty answer", `{"Status":0,"Answer":[]}`, nil},
		{"malformed json", `not json`, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseDoHJSONAnswer([]byte(c.body)); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("parseDoHJSONAnswer(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

func TestQueryDoHEndpoint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("name"); got != "frankfurt.example.com" {
			t.Errorf("name param = %q", got)
		}
		if got := r.URL.Query().Get("type"); got != "A" {
			t.Errorf("type param = %q", got)
		}
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(`{"Status":0,"Answer":[{"type":1,"data":"203.0.113.9"}]}`))
	}))
	defer ts.Close()

	got := queryDoHEndpoint(ts.Client(), ts.URL, "frankfurt.example.com")
	if want := []string{"203.0.113.9"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("queryDoHEndpoint = %v, want %v", got, want)
	}
}

func TestResolveServerIPsViaDoH_LiteralAndEmptyShortCircuit(t *testing.T) {
	if got := resolveServerIPsViaDoH("203.0.113.7"); got != nil {
		t.Fatalf("literal IP must short-circuit to nil, got %v", got)
	}
	if got := resolveServerIPsViaDoH(""); got != nil {
		t.Fatalf("empty host must short-circuit to nil, got %v", got)
	}
}
