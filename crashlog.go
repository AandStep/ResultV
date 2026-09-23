// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"resultproxy-wails/internal/system"
)

// startCrashLog routes the runtime's fatal output to diag/crash.log. A GUI
// build has no stderr, so without it an unrecovered panic leaves no trace.
func startCrashLog() {
	dir := filepath.Join(system.UserDataDir(), "diag")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	path := filepath.Join(dir, "crash.log")
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if info, err := os.Stat(path); err == nil && info.Size() > 1<<20 {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "=== start %s pid %d ===\n", time.Now().Format("2006-01-02 15:04:05"), os.Getpid())
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
	f.Close()
}
