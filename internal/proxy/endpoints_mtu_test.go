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

func TestWireGuardMTUOverride(t *testing.T) {
	cases := []struct {
		name       string
		env        string
		configured int
		want       int
	}{
		{"без переменной берётся значение узла", "", 1408, 1408},
		{"переменная перекрывает узел", "1280", 1408, 1280},
		{"переменная работает и при нулевом значении узла", "1280", 0, 1280},
		{"мусор игнорируется", "не-число", 1408, 1408},
		// Below the IPv4 minimum a path is not required to carry anything, and
		// above 1500 the override would create the very problem it exists to
		// test for.
		{"слишком маленькое игнорируется", "500", 1408, 1408},
		{"слишком большое игнорируется", "9000", 1408, 1408},
		{"граница снизу принимается", "576", 1408, 576},
		{"граница сверху принимается", "1500", 1408, 1500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("RESULTV_WG_MTU", tc.env)
			}
			if got := wireguardMTU(tc.configured); got != tc.want {
				t.Errorf("wireguardMTU(%d) при RESULTV_WG_MTU=%q = %d, ожидалось %d",
					tc.configured, tc.env, got, tc.want)
			}
		})
	}
}
