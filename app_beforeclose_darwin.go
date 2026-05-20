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

//go:build darwin

package main

import (
	"context"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"resultproxy-wails/internal/system"
)

// beforeClosePlatform handles Dock Quit / Cmd+Q / tray "Выход" on macOS.
// Window close (red button) is intercepted by Wails' HideWindowOnClose and
// never reaches this hook.
//
// Strategy:
//   - Hide the window IMMEDIATELY so the user sees instant feedback the moment
//     they click Quit. Without this the window would linger on screen for the
//     few seconds sing-box / networksetup take to wind down.
//   - Kick off the actual cleanup in a goroutine; it ends in os.Exit(0), which
//     is what closes the process for real (releases the singleton flock, makes
//     the entry disappear from Activity Monitor, lets the next launch acquire
//     the tray).
//   - Return `true` so Wails does NOT call [NSApp stop:]. The Cocoa main thread
//     stays in its run loop and keeps pumping events the entire time we're
//     cleaning up, so macOS never escalates to the "App not responding"
//     dialog. The process exits anyway via os.Exit from our goroutine.
func (a *App) beforeClosePlatform(ctx context.Context) bool {
	sdbg("beforeClosePlatform(darwin): enter (Wails-spawned goroutine)")
	a.markQuitRequested()
	system.ReplyToApplicationShouldTerminate()
	sdbg("beforeClosePlatform(darwin): replyToApplicationShouldTerminate:YES dispatched")
	sdbg("beforeClosePlatform(darwin): calling WindowHide")
	wailsRuntime.WindowHide(ctx)
	sdbg("beforeClosePlatform(darwin): WindowHide returned, spawning runShutdownTasks goroutine")
	go a.runShutdownTasks()
	sdbg("beforeClosePlatform(darwin): returning true (Wails will NOT call NSApp stop)")
	return true
}
