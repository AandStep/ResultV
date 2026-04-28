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

import (
	"golang.org/x/sys/windows"
)

var procIsWindowVisible = dllUser32Hook.NewProc("IsWindowVisible")

// IsMainWindowVisible returns true if the main Wails window (identified by
// WailsWindowClassResultV) exists and is currently visible on screen.
// Used to detect a dead WebView2 after system suspend/resume.
func IsMainWindowVisible() bool {
	hwnd := findTopLevelMainHWND(WailsWindowClassResultV)
	if hwnd == 0 {
		return false
	}
	ret, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
	return ret != 0
}

// FindMainWindow returns the HWND of the main Wails window, or 0 if not found.
func FindMainWindow() windows.HWND {
	return findTopLevelMainHWND(WailsWindowClassResultV)
}
