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

// Package sdbg is a tiny file+stderr tracer used only for diagnosing the
// macOS shutdown / tray quit path. Every Log call is timestamped to the
// microsecond and Sync'd to disk so the trace is durable even if the
// process is killed a moment later.
package sdbg

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu     sync.Mutex
	file   *os.File
	opened bool
)

func path() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "/tmp/resultv-shutdown-trace.log"
	}
	dir := filepath.Join(home, "Library", "Logs", "ResultV")
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		return "/tmp/resultv-shutdown-trace.log"
	}
	return filepath.Join(dir, "shutdown-trace.log")
}

func ensure() {
	if opened {
		return
	}
	opened = true
	p := path()
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[sdbg] cannot open %s: %v\n", p, err)
		return
	}
	file = f
	_, _ = fmt.Fprintf(file, "\n=== process %d started at %s ===\n", os.Getpid(), time.Now().Format("2006-01-02 15:04:05.000000"))
	_ = file.Sync()
}

// Log writes one timestamped line to the shutdown trace file and to stderr.
// Safe to call from any goroutine.
func Log(format string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()
	ensure()
	msg := fmt.Sprintf(format, args...)
	stamp := time.Now().Format("15:04:05.000000")
	line := fmt.Sprintf("[%s pid=%d] %s\n", stamp, os.Getpid(), msg)
	fmt.Fprint(os.Stderr, line)
	if file != nil {
		_, _ = file.WriteString(line)
		_ = file.Sync()
	}
}
