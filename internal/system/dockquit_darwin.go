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

package system

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include "dockquit_darwin.h"
*/
import "C"

import (
	"sync"
	"time"

	"resultproxy-wails/internal/sdbg"
)

var (
	dockQuitMu       sync.Mutex
	dockQuitCallback func()
	dockShowCallback func()
)

// RegisterDockQuitHandler registers callbacks invoked from the custom Dock menu.
// quit is called for "Завершить ResultV"; show is optional for "Показать ResultV".
func RegisterDockQuitHandler(quit, show func()) {
	dockQuitMu.Lock()
	dockQuitCallback = quit
	dockShowCallback = show
	dockQuitMu.Unlock()
}

//export resultvDockQuitGoCallback
func resultvDockQuitGoCallback() {
	sdbg.Log("dock menu: user chose 'Завершить ResultV'")
	dockQuitMu.Lock()
	cb := dockQuitCallback
	dockQuitMu.Unlock()
	if cb != nil {
		cb()
	}
}

//export resultvDockShowGoCallback
func resultvDockShowGoCallback() {
	sdbg.Log("dock menu: user chose 'Показать ResultV'")
	dockQuitMu.Lock()
	cb := dockShowCallback
	dockQuitMu.Unlock()
	if cb != nil {
		cb()
	}
}

// resultvDockMenuRequestedGoCallback is invoked from applicationDockMenu: every
// time the Dock asks for our context menu (i.e. user right-clicked the icon).
// If this log line never appears but "dock menu: installed" did, the swizzle is
// in place but macOS isn't routing the request to our delegate — typical when
// the process runs as root in another session and Dock falls back to the
// hard-coded "Force Quit" entry.
//
//export resultvDockMenuRequestedGoCallback
func resultvDockMenuRequestedGoCallback() {
	sdbg.Log("dock menu: applicationDockMenu: invoked by Dock (right-click)")
}

// InstallDockQuitMenu adds applicationDockMenu: to the Wails NSApplication delegate
// so Dock right-click always offers a graceful quit (same path as Cmd+Q), even
// when macOS only shows "Force Quit" for the default terminate flow.
//
// Blocks up to maxWait until the delegate exists and the menu is installed.
// Call from startup after the tray is running so [NSApp delegate] is set.
func InstallDockQuitMenu() bool {
	const (
		step    = 50 * time.Millisecond
		maxWait = 3 * time.Second
	)
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if C.resultvInstallDockQuitMenuSync() != 0 {
			sdbg.Log("dock menu: installed (Dock right-click should show 'Завершить ResultV')")
			return true
		}
		time.Sleep(step)
	}
	sdbg.Log("dock menu: install failed after %v (delegate may be nil)", maxWait)
	return false
}
