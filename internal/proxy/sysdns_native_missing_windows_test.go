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

package proxy

import (
	"testing"

	"golang.org/x/sys/windows"
)

// SetInterfaceDnsSettings exists only since Windows 10 2004 (build 19041).
// On older builds the native path must fail with an error so setAdapterDNS
// falls back to PowerShell, instead of panicking and killing the process.
func TestSetInterfaceDNSNative_MissingProcIsError(t *testing.T) {
	old := procSetInterfaceDnsSettings
	t.Cleanup(func() { procSetInterfaceDnsSettings = old })
	procSetInterfaceDnsSettings = modIPHLPAPI.NewProc("SetInterfaceDnsSettingsDoesNotExist")

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked instead of returning an error: %v", r)
		}
	}()
	if err := setInterfaceDNSNative(windows.GUID{}, nil, true); err == nil {
		t.Fatal("expected an error for a missing procedure")
	}
}
