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

package system

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const WindowsAppUserModelID = "ResultV.ResultV"

var (
	shell32                                     = windows.NewLazySystemDLL("shell32.dll")
	procSetCurrentProcessExplicitAppUserModelID = shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
)

func SetProcessAppUserModelID() {
	_ = setProcessAppUserModelID(WindowsAppUserModelID)
}

func setProcessAppUserModelID(id string) error {
	if id == "" {
		return nil
	}
	p, err := windows.UTF16PtrFromString(id)
	if err != nil {
		return err
	}
	r, _, _ := procSetCurrentProcessExplicitAppUserModelID.Call(uintptr(unsafe.Pointer(p)))
	if r != 0 {
		return fmt.Errorf("SetCurrentProcessExplicitAppUserModelID: HRESULT 0x%X", uint32(r))
	}
	return nil
}
