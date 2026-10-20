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

//go:build windows

package processtree

import (
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// stillActive is the exit code GetExitCodeProcess reports for a running process.
const stillActive = 259

// pollInterval paces the process-table scan used when no event feed is running.
const pollInterval = time.Second

type platformSource struct{}

func filetimeOf(t time.Time) int64 { return t.UnixNano()/100 + 116444736000000000 }

func filetimeValue(ft windows.Filetime) int64 {
	return int64(uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime))
}

func (platformSource) now() int64 { return filetimeOf(time.Now()) }

func (platformSource) young(created int64, d time.Duration) bool {
	return created != 0 && filetimeOf(time.Now())-created < int64(d/100)
}

func (platformSource) live(pid uint32) (Process, bool) {
	if pid == 0 || pid == 4 {
		return Process{}, false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return Process{}, false
	}
	defer windows.CloseHandle(h)
	// An exited process stays openable while anyone holds a handle to it.
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil || code != stillActive {
		return Process{}, false
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return Process{}, false
	}
	var info windows.PROCESS_BASIC_INFORMATION
	if err := windows.NtQueryInformationProcess(h, windows.ProcessBasicInformation,
		unsafe.Pointer(&info), uint32(unsafe.Sizeof(info)), nil); err != nil {
		return Process{}, false
	}
	return Process{
		PID:       pid,
		ParentPID: uint32(info.InheritedFromUniqueProcessId),
		Created:   filetimeValue(created),
		Image:     imagePath(h),
	}, true
}

func imagePath(h windows.Handle) string {
	for _, n := range []int{512, windows.MAX_LONG_PATH} {
		buf := make([]uint16, n)
		size := uint32(len(buf))
		if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err == nil {
			return windows.UTF16ToString(buf[:size])
		}
	}
	return ""
}

type scanEntry struct {
	proc Process
	ppid uint32
	exe  string
}

// scan reads the process table once. Processes that were already in prev
// under the same parent and name are taken from there instead of being opened
// again.
func scan(t *Tracker, prev map[uint32]scanEntry) map[uint32]scanEntry {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return prev
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return prev
	}
	next := make(map[uint32]scanEntry, len(prev)+16)
	present := make(map[uint32]bool, len(prev)+16)
	var procs []Process
	for {
		pid, ppid := entry.ProcessID, entry.ParentProcessID
		exe := windows.UTF16ToString(entry.ExeFile[:])
		present[pid] = true
		if old, ok := prev[pid]; ok && old.ppid == ppid && old.exe == exe {
			next[pid] = old
			procs = append(procs, old.proc)
		} else if p, ok := (platformSource{}).live(pid); ok {
			next[pid] = scanEntry{proc: p, ppid: ppid, exe: exe}
			procs = append(procs, p)
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	t.rescan(procs, present, filetimeOf(time.Now()))
	return next
}

var (
	platformMu   sync.Mutex
	platformStop chan struct{}
)

func startPlatform(t *Tracker) {
	stop := make(chan struct{})
	platformMu.Lock()
	platformStop = stop
	platformMu.Unlock()

	go func() {
		feed := startEventFeed(t) == nil
		t.setEvents(feed)
		known := scan(t, nil)
		if feed {
			return
		}
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				known = scan(t, known)
			}
		}
	}()
}

func stopPlatform() {
	platformMu.Lock()
	stop := platformStop
	platformStop = nil
	platformMu.Unlock()
	if stop != nil {
		close(stop)
	}
	stopEventFeed()
}
