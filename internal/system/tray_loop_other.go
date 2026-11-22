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

//go:build !darwin

package system

import (
	"runtime"

	"github.com/energye/systray"
)

func startTray(onReady, onExit func()) {
	// The native tray window and its message loop must use the same thread.
	go func() {
		runtime.LockOSThread()
		systray.Run(onReady, onExit)
	}()
}

func (t *Tray) configureTrayMenu() {
	// Left click restores the window. The right-click menu is supplied by systray.
	systray.SetOnClick(func(_ systray.IMenu) {
		t.safeCall(func() {
			if t.callbacks.OnShowWindow != nil {
				t.callbacks.OnShowWindow()
			}
		})
	})
}

func applyTrayServerMark(_ *systray.MenuItem, mark trayMark) trayMark {
	return mark
}
