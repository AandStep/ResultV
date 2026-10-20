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
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

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

func selfElevateParams(args []string, parentPID int) string {
	parts := []string{ElevatedFromFlag + strconv.Itoa(parentPID)}
	for _, a := range args[1:] {
		if strings.HasPrefix(a, ElevatedFromFlag) {
			continue
		}
		parts = append(parts, windows.EscapeArg(a))
	}
	return strings.Join(parts, " ")
}

// RelaunchElevated starts this executable elevated with the same arguments.
// false with a nil error means the user declined the UAC prompt.
func RelaunchElevated(args []string) (bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(selfElevateParams(args, os.Getpid()))
	info := shellExecuteInfo{lpVerb: verb, lpFile: file, lpParameters: params, nShow: 1}
	info.cbSize = uint32(unsafe.Sizeof(info))
	ret, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if ret != 0 {
		return true, nil
	}
	if errors.Is(callErr, windows.ERROR_CANCELLED) {
		return false, nil
	}
	return false, fmt.Errorf("ShellExecuteExW: %w", callErr)
}

func KeepInvokerOwnership() func() { return func() {} }
