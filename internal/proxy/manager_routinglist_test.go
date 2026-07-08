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

import "testing"

func TestSetRoutingListsCopies(t *testing.T) {
	m := &Manager{}
	src := []RoutingListSpec{{Tag: "rl-a", Path: "/tmp/a.json", Action: "proxy"}}
	m.SetRoutingLists(src)
	src[0].Action = "block" // mutate caller's slice

	got := m.routingListSpecsLocked()
	if len(got) != 1 || got[0].Action != "proxy" {
		t.Fatalf("SetRoutingLists must copy, got %+v", got)
	}
}
