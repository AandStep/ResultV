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

package main

import (
	"context"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// beforeClosePlatform on Windows/Linux.
//
//   - First close (window red button, quitRequested=false): hide to tray.
//   - Real quit (quitRequested=true, e.g. tray "Выход"): hide the window for
//     instant visual feedback, then run the cleanup in a goroutine that ends
//     in os.Exit(0). Returning true keeps Wails' main loop responsive while
//     sing-box / system proxy / kill switch wind down; the process exits via
//     our os.Exit, not via [NSApp stop:] / WM_QUIT.
func (a *App) beforeClosePlatform(ctx context.Context) bool {
	sdbg("beforeClosePlatform(other): enter")
	a.stateMu.Lock()
	quitRequested := a.quitRequested
	a.stateMu.Unlock()
	a.trayHidden.Store(1)
	wailsRuntime.WindowHide(ctx)
	if quitRequested {
		sdbg("beforeClosePlatform(other): quitRequested, spawning runShutdownTasks")
		go a.runShutdownTasks()
	} else {
		sdbg("beforeClosePlatform(other): hide-to-tray (first close)")
	}
	return true
}
