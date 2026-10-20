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

//go:build windows && (amd64 || arm64)

package processtree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The API reads these structs by their C layout; a field out of place shifts
// everything after it without any error.
func TestETWStructLayout(t *testing.T) {
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"EVENT_TRACE_PROPERTIES", unsafe.Sizeof(eventTraceProperties{}), 120},
		{"EVENT_TRACE_LOGFILEW", unsafe.Sizeof(eventTraceLogfile{}), 448},
		{"EVENT_TRACE_LOGFILEW.EventRecordCallback", unsafe.Offsetof(eventTraceLogfile{}.EventRecordCallback), 424},
		{"EVENT_HEADER", unsafe.Sizeof(eventHeader{}), 80},
		{"EVENT_RECORD", unsafe.Sizeof(eventRecord{}), 112},
		{"EVENT_RECORD.UserData", unsafe.Offsetof(eventRecord{}.UserData), 96},
		{"PROPERTY_DATA_DESCRIPTOR", unsafe.Sizeof(propertyDataDescriptor{}), 16},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestDosPathMapsTheSystemDrive(t *testing.T) {
	windir := os.Getenv("WINDIR")
	drive := filepath.VolumeName(windir)
	name, _ := windows.UTF16PtrFromString(drive)
	buf := make([]uint16, 1024)
	n, err := windows.QueryDosDevice(name, &buf[0], uint32(len(buf)))
	if err != nil {
		t.Skipf("QueryDosDevice(%s): %v", drive, err)
	}
	device := windows.UTF16ToString(buf[:n])
	want := filepath.Join(windir, "explorer.exe")
	got := dosPath(device + strings.TrimPrefix(want, drive))
	if !strings.EqualFold(got, want) {
		t.Fatalf("dosPath = %q, want %q", got, want)
	}
	if got := dosPath(`C:\already\dos.exe`); got != `C:\already\dos.exe` {
		t.Fatalf("a DOS path changed: %q", got)
	}
}
