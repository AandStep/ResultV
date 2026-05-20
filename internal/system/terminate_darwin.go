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
#include "terminate_darwin.h"
*/
import "C"

import (
	"sync"
	"time"

	"resultproxy-wails/internal/sdbg"
)

var (
	terminateMu       sync.Mutex
	terminateCallback func()
)

// RegisterTerminateHandler is invoked from the swizzled applicationShouldTerminate:
// instead of Wails' blocking processMessage("Q") path.
func RegisterTerminateHandler(cb func()) {
	terminateMu.Lock()
	terminateCallback = cb
	terminateMu.Unlock()
}

//export resultvTerminateGoCallback
func resultvTerminateGoCallback() {
	sdbg.Log("terminate interceptor: graceful quit (Cmd+Q / Dock Quit)")
	terminateMu.Lock()
	cb := terminateCallback
	terminateMu.Unlock()
	if cb != nil {
		cb()
	}
}

// ReplyToApplicationShouldTerminate tells macOS the app accepted the pending
// quit request after NSTerminateCancel.
func ReplyToApplicationShouldTerminate() {
	C.resultvReplyToApplicationShouldTerminate()
}

// InstallTerminateInterceptor replaces applicationShouldTerminate: on the Wails
// delegate. Blocks up to maxWait until [NSApp delegate] exists.
func InstallTerminateInterceptor() bool {
	const (
		step    = 50 * time.Millisecond
		maxWait = 3 * time.Second
	)
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if C.resultvInstallTerminateInterceptorSync() != 0 {
			sdbg.Log("terminate interceptor: installed")
			return true
		}
		time.Sleep(step)
	}
	sdbg.Log("terminate interceptor: install failed after %v", maxWait)
	return false
}
