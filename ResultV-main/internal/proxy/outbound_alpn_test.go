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
	"testing"
)

func TestXHTTPPreferH2ALPN(t *testing.T) {
	got := xhttpPreferH2ALPN([]string{"h3", "h2", "http/1.1"}, false)
	if len(got) < 3 || got[0] != "h2" || got[1] != "h3" {
		t.Fatalf("got %v", got)
	}
	if xhttpPreferH2ALPN([]string{"h2", "h3"}, false)[0] != "h2" {
		t.Fatal("h2 first should stay")
	}
	empty := xhttpPreferH2ALPN(nil, false)
	if len(empty) < 1 || empty[0] != "h2" {
		t.Fatalf("default: %v", empty)
	}
}
