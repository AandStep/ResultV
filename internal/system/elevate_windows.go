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
	"time"

	"golang.org/x/sys/windows"
)

const parentExitTimeout = 20 * time.Second

// WaitForElevationParent blocks until the instance that relaunched us elevated
// has exited: its shutdown restores DNS from the shared snapshot and strips
// routes from our adapter, which by then would be ours.
func WaitForElevationParent(args []string) {
	pid := elevatedFromPID(args)
	if pid <= 0 {
		return
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	_, _ = windows.WaitForSingleObject(h, uint32(parentExitTimeout/time.Millisecond))
}

func KeepInvokerOwnership() func() { return func() {} }
