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
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Class name registered by energye/systray for its hidden owner window.
// Hardcoded here because the library does not export it. If energye renames
// it in a future release, enableTrayDarkMode silently no-ops.
const energyeSystrayClass = "SystrayClass"

const (
	// Ordinal 135 in uxtheme.dll is SetPreferredAppMode (Win10 1903+).
	// PreferredAppMode = 2 == ForceDark.
	preferredAppModeForceDark = 2

	// DWMWA_USE_IMMERSIVE_DARK_MODE attribute index.
	// 19 on Win10 1809–1909; 20 on Win10 2004 and Win11. We try the modern
	// one first; if it fails, fall back to the older index.
	dwmwaUseImmersiveDarkModeNew = 20
	dwmwaUseImmersiveDarkModeOld = 19
)

var (
	dllUxtheme          = windows.NewLazyDLL("uxtheme.dll")
	procSetPreferredApp = dllUxtheme.NewProc("#135")

	dllDwmapi           = windows.NewLazyDLL("dwmapi.dll")
	procDwmSetWindowAtt = dllDwmapi.NewProc("DwmSetWindowAttribute")
)

// procFindWindowW is declared in instance_messenger_windows.go.

// enableTrayDarkMode forces the popup tray menu (and any other native menus
// owned by this process) to render with the Win11 dark theme — rounded
// corners, dark grey background, hover accent. Equivalent visual to the
// screenshot the design was based on.
//
// The two-step incantation is:
//  1. SetPreferredAppMode(ForceDark) tells UXTheme to draw controls dark
//     for the entire process.
//  2. DwmSetWindowAttribute(DWMWA_USE_IMMERSIVE_DARK_MODE) on the systray
//     owner window propagates dark mode to popups created by that window.
//
// Both calls degrade silently on older Windows (the LazyDLL/LazyProc.Find()
// returns an error and we no-op). The systray icon still works either way.
func enableTrayDarkMode() {
	enableProcessDarkMode()
	enableDarkModeForSystrayWindow()
}

func enableProcessDarkMode() {
	if err := procSetPreferredApp.Find(); err != nil {
		return
	}
	_, _, _ = procSetPreferredApp.Call(uintptr(preferredAppModeForceDark))
}

func enableDarkModeForSystrayWindow() {
	if err := procDwmSetWindowAtt.Find(); err != nil {
		return
	}
	hwnd := findSystrayWindow()
	if hwnd == 0 {
		return
	}
	enabled := int32(1)
	if r1, _, _ := procDwmSetWindowAtt.Call(
		uintptr(hwnd),
		uintptr(dwmwaUseImmersiveDarkModeNew),
		uintptr(unsafe.Pointer(&enabled)),
		unsafe.Sizeof(enabled),
	); r1 != 0 {
		// Fall back to the pre-2004 attribute index for older Win10 builds.
		_, _, _ = procDwmSetWindowAtt.Call(
			uintptr(hwnd),
			uintptr(dwmwaUseImmersiveDarkModeOld),
			uintptr(unsafe.Pointer(&enabled)),
			unsafe.Sizeof(enabled),
		)
	}
}

func findSystrayWindow() windows.HWND {
	classPtr, err := syscall.UTF16PtrFromString(energyeSystrayClass)
	if err != nil {
		return 0
	}
	h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(classPtr)), 0)
	return windows.HWND(h)
}
