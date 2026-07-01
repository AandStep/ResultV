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

//go:build linux

package processtree

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// platformScan walks /proc and returns descendants of the given roots.
//
// We deliberately avoid mitchellh/go-ps here: it parses /proc/<pid>/stat
// for the executable name, which the kernel truncates to TASK_COMM_LEN
// (16 bytes) and renders matching against full basenames unreliable. We
// instead read /proc/<pid>/exe directly — a symlink to the actual binary
// path — and take its basename, giving exact matches every time.
//
// Reading /proc/<pid>/exe for processes owned by other users requires
// CAP_SYS_PTRACE or root. The VPN engine already runs elevated for tunnel
// mode, so this isn't a new requirement; in non-elevated proxy mode some
// children may simply be invisible, which only means they still flow via
// the proxy — no security risk, just a coverage gap.
func platformScan(roots []string) Snapshot {
	snap := Snapshot{Roots: append([]string(nil), roots...)}
	if len(roots) == 0 {
		return snap
	}

	procs := readProcTable()
	if len(procs) == 0 {
		return snap
	}

	rootSet := make(map[string]struct{}, len(roots))
	for _, r := range roots {
		rootSet[r] = struct{}{}
	}

	children := make(map[int][]int, len(procs))
	for pid, info := range procs {
		if info.parentPID == pid || info.parentPID == 0 {
			continue
		}
		children[info.parentPID] = append(children[info.parentPID], pid)
	}

	descSet := make(map[string]struct{})
	visited := make(map[int]bool, len(procs))
	for pid, info := range procs {
		if _, isRoot := rootSet[info.exe]; !isRoot {
			continue
		}
		collectDescendantsLinux(pid, children, procs, descSet, rootSet, visited)
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

type linuxProcInfo struct {
	parentPID int
	exe       string
}

func collectDescendantsLinux(
	pid int,
	children map[int][]int,
	procs map[int]linuxProcInfo,
	out map[string]struct{},
	rootSet map[string]struct{},
	visited map[int]bool,
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
		if _, isRoot := rootSet[info.exe]; !isRoot && info.exe != "" {
			out[info.exe] = struct{}{}
		}
		collectDescendantsLinux(child, children, procs, out, rootSet, visited)
	}
}

// readProcTable enumerates /proc and returns one entry per running PID.
// Errors on individual entries (process exited mid-scan, permission denied)
// are silently skipped — we want a best-effort snapshot, not a strict scan.
func readProcTable() map[int]linuxProcInfo {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	out := make(map[int]linuxProcInfo, 256)
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(ent.Name())
		if err != nil || pid <= 0 {
			continue
		}
		ppid, ok := readPPid(pid)
		if !ok {
			continue
		}
		exe := readExeBasename(pid)
		out[pid] = linuxProcInfo{parentPID: ppid, exe: exe}
	}
	return out
}

// readPPid extracts the PPid: line from /proc/<pid>/status. Cheaper than
// parsing /proc/<pid>/stat (which has a quoting nightmare around the comm
// field) and resilient to short reads — we only need one line.
func readPPid(pid int) (int, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0, false
	}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if !bytes.HasPrefix(line, []byte("PPid:")) {
			continue
		}
		field := strings.TrimSpace(string(line[len("PPid:"):]))
		ppid, err := strconv.Atoi(field)
		if err != nil {
			return 0, false
		}
		return ppid, true
	}
	return 0, false
}

// readExeBasename resolves /proc/<pid>/exe to the absolute path of the
// running binary and returns its lowercased basename. Returns "" when the
// symlink can't be read (permission denied, kernel thread, race).
func readExeBasename(pid int) string {
	target, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if err != nil {
		return ""
	}
	// Kernel appends " (deleted)" when the original binary was unlinked;
	// trim that so a running process whose exe was upgraded still matches.
	if i := strings.LastIndex(target, " (deleted)"); i > 0 {
		target = target[:i]
	}
	base := filepath.Base(target)
	if base == "." || base == "/" {
		return ""
	}
	return strings.ToLower(base)
}
