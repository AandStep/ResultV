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
	"time"

	ps "github.com/mitchellh/go-ps"
)

// platformSource asks go-ps, which knows parents but not creation times, so a
// reused PID cannot be told apart here.
type platformSource struct{}

func (platformSource) now() int64 { return 0 }

func (platformSource) young(int64, time.Duration) bool { return false }

func (platformSource) live(pid uint32) (Process, bool) {
	p, err := ps.FindProcess(int(pid))
	if err != nil || p == nil {
		return Process{}, false
	}
	return Process{PID: pid, ParentPID: uint32(p.PPid()), Image: p.Executable()}, true
}

func startPlatform(*Tracker) {}

func stopPlatform() {}
