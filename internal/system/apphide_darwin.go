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
#include "apphide_darwin.h"
*/
import "C"

import "sync"

var (
	appHideMu       sync.Mutex
	appHideCallback func()
)

//export resultvAppHideDidHideCallback
func resultvAppHideDidHideCallback() {
	appHideMu.Lock()
	cb := appHideCallback
	appHideMu.Unlock()
	if cb != nil {
		cb()
	}
}

// StartAppHideObserver registers for NSApplicationDidHideNotification (e.g. red
// button with HideWindowOnClose). Returns a stop function to remove the observer.
func StartAppHideObserver(onHide func()) func() {
	appHideMu.Lock()
	appHideCallback = onHide
	appHideMu.Unlock()
	C.resultvStartAppHideObserver()
	return func() {
		C.resultvStopAppHideObserver()
		appHideMu.Lock()
		appHideCallback = nil
		appHideMu.Unlock()
	}
}
