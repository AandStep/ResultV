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

func TestTunStackOverride(t *testing.T) {
	cases := []struct {
		name   string
		env    string
		stored string
		want   string
	}{
		{"без переменной действует настройка", "", "gvisor", "gvisor"},
		{"без переменной дефолт system", "", "", "system"},
		{"переменная перекрывает настройку", "gvisor", "system", "gvisor"},
		{"переменная может вернуть system", "system", "gvisor", "system"},
		{"регистр не важен", "GVISOR", "system", "gvisor"},
		{"неизвестное значение игнорируется", "mystack", "system", "system"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("RESULTV_TUN_STACK", tc.env)
			}
			if got := effectiveTunStack(tc.stored); got != tc.want {
				t.Errorf("effectiveTunStack(%q) при RESULTV_TUN_STACK=%q = %q, ожидалось %q",
					tc.stored, tc.env, got, tc.want)
			}
		})
	}
}
