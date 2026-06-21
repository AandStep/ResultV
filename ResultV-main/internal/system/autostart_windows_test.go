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

//go:build windows

package system

import "testing"

func TestBuildAutostartRunCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		exe  string
		extra []string
		want string
	}{
		{
			exe:  `C:\Program Files\ResultV\ResultV.exe`,
			want: `"C:\Program Files\ResultV\ResultV.exe" --autostart`,
		},
		{
			exe:  `C:/Tools/app.exe`,
			want: `"C:\Tools\app.exe" --autostart`,
		},
		{
			exe:   `C:\app.exe`,
			extra: []string{"--foo"},
			want:  `"C:\app.exe" --autostart --foo`,
		},
	}
	for _, tc := range cases {
		got := buildAutostartRunCommand(tc.exe, tc.extra...)
		if got != tc.want {
			t.Errorf("exe=%q extra=%v\ngot  %q\nwant %q", tc.exe, tc.extra, got, tc.want)
		}
	}
}

func TestArgsStartInTray(t *testing.T) {
	t.Parallel()
	if !ArgsStartInTray([]string{"exe", "--autostart"}) {
		t.Fatal("expected true for --autostart")
	}
	if !ArgsStartInTray([]string{"exe", "--tray"}) {
		t.Fatal("expected true for --tray")
	}
	if ArgsStartInTray([]string{"exe"}) {
		t.Fatal("expected false without flags")
	}
}
