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
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// platformScan walks the Windows process table once, builds a parent→children
// map, then DFS-collects every process whose ancestry chain leads to one of
// roots. Returns the lowercased basenames of all such descendants (excluding
// the root processes themselves — those are already handled by the user's
// whitelist directly).
func platformScan(roots []string) Snapshot {
	snap := Snapshot{Roots: append([]string(nil), roots...)}
	if len(roots) == 0 {
		return snap
	}

	procs, err := snapshotProcesses()
	if err != nil || len(procs) == 0 {
		return snap
	}

	rootSet := make(map[string]struct{}, len(roots))
	for _, r := range roots {
		rootSet[r] = struct{}{}
	}

	// children[parentPID] -> list of child PIDs
	children := make(map[uint32][]uint32, len(procs))
	for pid, info := range procs {
		// PID 0 is the System Idle Process and ParentProcessID == ProcessID
		// for some kernel processes — skip self-loops to keep DFS finite.
		if info.parentPID == pid {
			continue
		}
		children[info.parentPID] = append(children[info.parentPID], pid)
	}

	// Find every PID whose name matches a root, then DFS its descendants.
	descSet := make(map[string]struct{})
	visited := make(map[uint32]bool, len(procs))
	for pid, info := range procs {
		if _, isRoot := rootSet[info.exe]; !isRoot {
			continue
		}
		collectDescendants(pid, children, procs, descSet, rootSet, visited)
	}

	if len(descSet) == 0 {
		return snap
	}
	out := make([]string, 0, len(descSet))
	for name := range descSet {
		out = append(out, name)
	}
	snap.Descendants = out
	return snap
}

type procInfo struct {
	parentPID uint32
	exe       string
}

func collectDescendants(
	pid uint32,
	children map[uint32][]uint32,
	procs map[uint32]procInfo,
	out map[string]struct{},
	rootSet map[string]struct{},
	visited map[uint32]bool,
) {
	if visited[pid] {
		return
	}
	visited[pid] = true

	for _, child := range children[pid] {
		info, ok := procs[child]
		if !ok {
			continue
		}
		// Skip if the descendant's exe is itself one of the user-specified
		// roots — the route rule already covers it via the user's entry.
		if _, isRoot := rootSet[info.exe]; !isRoot {
			out[info.exe] = struct{}{}
		}
		collectDescendants(child, children, procs, out, rootSet, visited)
	}
}

// snapshotProcesses uses Toolhelp32 to capture the current process table.
// Returns a map keyed by PID. Errors (e.g., transient access denied during
// process teardown) cause the offending entry to be skipped, not the whole
// scan to fail.
func snapshotProcesses() (map[uint32]procInfo, error) {
	handle, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(handle)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	if err := windows.Process32First(handle, &entry); err != nil {
		return nil, err
	}

	out := make(map[uint32]procInfo, 256)
	for {
		exe := windows.UTF16ToString(entry.ExeFile[:])
		if exe != "" {
			out[entry.ProcessID] = procInfo{
				parentPID: entry.ParentProcessID,
				exe:       strings.ToLower(exe),
			}
		}
		if err := windows.Process32Next(handle, &entry); err != nil {
			// ERROR_NO_MORE_FILES is the normal end-of-iteration signal.
			break
		}
	}
	return out, nil
}
