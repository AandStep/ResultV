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

func TestIPv6EnabledForTun(t *testing.T) {
	cases := []struct {
		v    uint64
		want bool
	}{
		{0x00, true},
		{0x01, true},  // tunnel interfaces only; Wintun is not one
		{0x20, true},  // prefer IPv4, nothing disabled
		{0x10, false}, // non-tunnel interfaces off
		{0x11, false},
		{0xFF, false},
		{0xFFFFFFFF, false},
	}
	for _, tc := range cases {
		if got := ipv6EnabledForTun(tc.v); got != tc.want {
			t.Errorf("ipv6EnabledForTun(%#x) = %v, want %v", tc.v, got, tc.want)
		}
	}
}
