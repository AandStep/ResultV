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
	"testing"

	"resultproxy-wails/internal/config"
)

func TestStartupConnectID(t *testing.T) {
	on := config.AppSettings{ConnectOnStartup: true, LastSelectedProxyID: "last"}
	off := config.AppSettings{LastSelectedProxyID: "last"}
	cases := []struct {
		name   string
		s      config.AppSettings
		resume string
		want   string
	}{
		{"перезапуск с сервером важнее настройки", on, "picked", "picked"},
		{"настройка включена", on, "", "last"},
		{"настройка выключена", off, "", ""},
		{"сервер не выбран", config.AppSettings{ConnectOnStartup: true}, "", ""},
	}
	for _, tc := range cases {
		if got := startupConnectID(tc.s, tc.resume); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}
