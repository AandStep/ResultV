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

//go:build !windows && !linux

package processtree

import (
	"strings"

	ps "github.com/mitchellh/go-ps"
)

// platformScan walks the OS process table via mitchellh/go-ps. Used on
// macOS, FreeBSD, OpenBSD, Solaris — anywhere we don't have a hand-rolled
// implementation.
//
// Caveat: on these platforms go-ps reads the comm field, which the kernel
// truncates (TASK_COMM_LEN on Linux is 16 bytes; macOS uses a 15-char
// comm in kinfo_proc). Process names longer than that get cropped, so
// matching against a regex built from the full basename can miss them.
// In practice common apps (steam, steamwebhelper, discord) are short
// enough; long-named binaries are an acceptable gap on niche platforms.
// Linux gets a dedicated implementation that reads /proc/<pid>/exe and
// avoids the truncation entirely.
func platformScan(roots []string) Snapshot {
	snap := Snapshot{Roots: append([]string(nil), roots...)}
	if len(roots) == 0 {
		return snap
	}

	procs, err := ps.Processes()
	if err != nil || len(procs) == 0 {
		return snap
	}

	rootSet := make(map[string]struct{}, len(roots))
	for _, r := range roots {
		rootSet[r] = struct{}{}
	}

	type info struct {
		ppid int
		exe  string
	}
	infos := make(map[int]info, len(procs))
	children := make(map[int][]int, len(procs))
	for _, p := range procs {
		pid := p.Pid()
		ppid := p.PPid()
		exe := strings.ToLower(strings.TrimSpace(p.Executable()))
		infos[pid] = info{ppid: ppid, exe: exe}
		if ppid != pid && ppid != 0 {
			children[ppid] = append(children[ppid], pid)
		}
	}

	descSet := make(map[string]struct{})
	visited := make(map[int]bool, len(procs))
	var dfs func(pid int)
	dfs = func(pid int) {
		if visited[pid] {
			return
		}
		visited[pid] = true
		for _, child := range children[pid] {
			ci, ok := infos[child]
			if !ok {
				continue
			}
			if _, isRoot := rootSet[ci.exe]; !isRoot && ci.exe != "" {
				descSet[ci.exe] = struct{}{}
			}
			dfs(child)
		}
	}
	for pid, i := range infos {
		if _, isRoot := rootSet[i.exe]; isRoot {
			dfs(pid)
		}
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
