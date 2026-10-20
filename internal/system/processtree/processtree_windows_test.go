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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestLiveDescribesThisProcess(t *testing.T) {
	p, ok := platformSource{}.live(uint32(os.Getpid()))
	if !ok {
		t.Fatal("live query of the test process failed")
	}
	if p.ParentPID != uint32(os.Getppid()) {
		t.Fatalf("parent %d, want %d", p.ParentPID, os.Getppid())
	}
	exe, _ := os.Executable()
	if !strings.EqualFold(filepath.Base(p.Image), filepath.Base(exe)) {
		t.Fatalf("image %q, want %q", p.Image, exe)
	}
	if !(platformSource{}).young(p.Created, time.Hour) {
		t.Fatalf("creation time %d is not within the last hour", p.Created)
	}
}

func TestLiveRejectsExitedProcess(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := uint32(cmd.Process.Pid)
	_ = cmd.Wait()
	// The test still holds a handle, so the PID cannot have been reused.
	if _, ok := (platformSource{}).live(pid); ok {
		t.Fatal("an exited process was reported alive")
	}
}

func TestAncestryOfSpawnedChild(t *testing.T) {
	cmd := exec.Command("ping.exe", "-n", "3", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	chain := newTracker(platformSource{}).Ancestry(uint32(cmd.Process.Pid))
	if len(chain) < 2 || chain[1].PID != uint32(os.Getpid()) {
		t.Fatalf("chain %v, want the child then this process", pids(chain))
	}
}

func isAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// The end-to-end case the event feed exists for: cmd starts ping in the
// background and exits at once, and ping's chain must still lead back here.
func TestEventFeedKeepsExitedIntermediate(t *testing.T) {
	if !isAdmin() {
		t.Skip("the ETW session needs administrator rights")
	}
	tr := newTracker(platformSource{})
	if err := startEventFeed(tr); err != nil {
		t.Fatalf("event feed: %v", err)
	}
	defer stopEventFeed()
	tr.setEvents(true)
	scan(tr, nil)

	before := filetimeOf(time.Now())
	cmd := exec.Command("cmd.exe", "/c", "start", "/b", "ping.exe", "-n", "4", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	cmdPID := uint32(cmd.Process.Pid)
	_ = cmd.Wait()

	var ping Process
	deadline := time.Now().Add(3 * time.Second)
	for ping.PID == 0 && time.Now().Before(deadline) {
		for _, p := range tr.Live() {
			if p.ParentPID == cmdPID && p.Created >= before && strings.EqualFold(filepath.Base(p.Image), "ping.exe") {
				ping = p
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if ping.PID == 0 {
		t.Fatal("the feed never reported ping.exe")
	}
	defer func() {
		if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, ping.PID); err == nil {
			_ = windows.TerminateProcess(h, 0)
			windows.CloseHandle(h)
		}
	}()

	chain := tr.Ancestry(ping.PID)
	if len(chain) < 3 || chain[1].PID != cmdPID || chain[2].PID != uint32(os.Getpid()) {
		t.Fatalf("chain %v, want ping, the exited cmd %d, then this process %d", pids(chain), cmdPID, os.Getpid())
	}
}
