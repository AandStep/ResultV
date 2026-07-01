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

package system

// TrafficStats reports cumulative interface counters in bytes. The numbers come
// from the OS — implementations live in system_windows.go / system_unix.go.
type TrafficStats struct {
	Received int64 `json:"received"`
	Sent     int64 `json:"sent"`
}

// ArgsStartInTray reports whether the process was launched in tray-only mode.
// Cross-platform: parsed from os.Args by main, no syscalls.
func ArgsStartInTray(args []string) bool {
	for _, a := range args[1:] {
		if a == "--autostart" || a == "--tray" {
			return true
		}
	}
	return false
}
