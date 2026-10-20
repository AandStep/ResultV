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
	"os"
	"strconv"
	"strings"
	"time"
)

// clockTicks is USER_HZ, the unit of starttime in /proc/<pid>/stat. Linux has
// fixed it at 100 on every mainstream architecture.
const clockTicks = 100

// platformSource reads /proc. There is no event feed: a parent is found while
// it is alive, which covers every launcher that stays up with its game.
type platformSource struct{}

func (platformSource) now() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int64(secs * clockTicks)
}

func (s platformSource) young(created int64, d time.Duration) bool {
	now := s.now()
	return created != 0 && now != 0 && now-created < int64(d.Seconds()*clockTicks)
}

func (platformSource) live(pid uint32) (Process, bool) {
	dir := "/proc/" + strconv.FormatUint(uint64(pid), 10)
	stat, err := os.ReadFile(dir + "/stat")
	if err != nil {
		return Process{}, false
	}
	// The command name in parentheses may itself contain spaces and ')'.
	s := string(stat)
	end := strings.LastIndexByte(s, ')')
	if end < 0 {
		return Process{}, false
	}
	fields := strings.Fields(s[end+1:])
	if len(fields) < 20 {
		return Process{}, false
	}
	ppid, err := strconv.ParseUint(fields[1], 10, 32)
	if err != nil {
		return Process{}, false
	}
	started, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return Process{}, false
	}
	image, _ := os.Readlink(dir + "/exe")
	image = strings.TrimSuffix(image, " (deleted)")
	return Process{PID: pid, ParentPID: uint32(ppid), Created: started, Image: image}, true
}

func startPlatform(*Tracker) {}

func stopPlatform() {}
