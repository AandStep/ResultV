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
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// AwaitOtherInstances waits for other processes of this executable in this
// session to exit and ends the ones still running after grace. Next to the
// singleton holder only a quitting instance can be alive, and while it holds
// the TUN adapter every new one fails with "already exists".
func AwaitOtherInstances(grace, killWait time.Duration) OtherInstancesReport {
	exe, err := os.Executable()
	if err != nil {
		return OtherInstancesReport{}
	}
	return awaitProcessesNamed(filepath.Base(exe), grace, killWait)
}

func awaitProcessesNamed(name string, grace, killWait time.Duration) OtherInstancesReport {
	var rep OtherInstancesReport
	handles := map[uint32]windows.Handle{}
	defer func() {
		for _, h := range handles {
			windows.CloseHandle(h)
		}
	}()
	for _, pid := range sameSessionProcessesNamed(name) {
		h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, pid)
		if err != nil {
			h, err = windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
		}
		if err != nil {
			continue
		}
		rep.Found = append(rep.Found, pid)
		handles[pid] = h
	}

	deadline := time.Now().Add(grace)
	for _, pid := range rep.Found {
		if !waitProcess(handles[pid], time.Until(deadline)) {
			if windows.TerminateProcess(handles[pid], 1) == nil {
				rep.Killed = append(rep.Killed, pid)
			}
		}
	}
	for _, pid := range rep.Found {
		if !waitProcess(handles[pid], killWait) {
			rep.Stuck = append(rep.Stuck, pid)
		}
	}
	return rep
}

func waitProcess(h windows.Handle, d time.Duration) bool {
	ms := uint32(0)
	if d > 0 {
		ms = uint32(d / time.Millisecond)
	}
	ev, err := windows.WaitForSingleObject(h, ms)
	return err == nil && ev == windows.WAIT_OBJECT_0
}

func sameSessionProcessesNamed(name string) []uint32 {
	self := uint32(os.Getpid())
	var session uint32
	if windows.ProcessIdToSessionId(self, &session) != nil {
		return nil
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var pids []uint32
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if entry.ProcessID == self || !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), name) {
			continue
		}
		var s uint32
		if windows.ProcessIdToSessionId(entry.ProcessID, &s) == nil && s == session {
			pids = append(pids, entry.ProcessID)
		}
	}
	return pids
}
