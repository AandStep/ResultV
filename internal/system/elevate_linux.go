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

//go:build linux

package system

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrElevationCancelled = errors.New("elevation_cancelled")
	ErrElevationDenied    = errors.New("elevation_denied")
	ErrElevationTimeout   = errors.New("elevation_timeout")
)

var (
	elevationAuthTimeout  = 3 * time.Minute
	elevationPollInterval = 100 * time.Millisecond
	parentExitTimeout     = 20 * time.Second
	ownershipSyncInterval = 30 * time.Second
)

// pkexec authorizes its parent process, so this instance has to stay alive
// until the password is accepted: quitting earlier makes polkit reject a
// correct password and can wedge an agent that lives inside the shell
// (Cinnamon).
func restartAsAdminLinux(exePath string, args []string) error {
	if _, err := exec.LookPath("pkexec"); err != nil {
		return fmt.Errorf("pkexec not found; install PolicyKit or relaunch via `sudo %s`",
			filepath.Base(exePath))
	}
	cmd := exec.Command("pkexec", pkexecRelaunchArgs(exePath, args, os.Getpid())...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pkexec start: %w", err)
	}
	return waitElevation(cmd, elevationAuthTimeout)
}

func pkexecRelaunchArgs(exePath string, args []string, parentPID int) []string {
	pkArgs := []string{"env"}
	for _, k := range []string{"DISPLAY", "XAUTHORITY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS"} {
		if v, ok := os.LookupEnv(k); ok {
			pkArgs = append(pkArgs, k+"="+v)
		}
	}
	// pkexec resets HOME to /root; without this the elevated instance starts
	// with an empty profile.
	pkArgs = append(pkArgs, "XDG_CONFIG_HOME="+userConfigBase())
	pkArgs = append(pkArgs, exePath, ElevatedFromFlag+strconv.Itoa(parentPID))
	return append(pkArgs, args...)
}

func waitElevation(cmd *exec.Cmd, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	tick := time.NewTicker(elevationPollInterval)
	defer tick.Stop()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case err := <-done:
			return pkexecExitError(err)
		case <-tick.C:
			if uid, ok := procRealUID(cmd.Process.Pid); ok && uid == 0 {
				return nil
			}
		case <-deadline.C:
			_ = cmd.Process.Kill()
			return ErrElevationTimeout
		}
	}
}

func pkexecExitError(err error) error {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		if err == nil {
			return fmt.Errorf("pkexec exited before the app started")
		}
		return fmt.Errorf("pkexec: %w", err)
	}
	switch exitErr.ExitCode() {
	case 126:
		return ErrElevationCancelled
	case 127:
		return ErrElevationDenied
	default:
		return fmt.Errorf("pkexec: %w", err)
	}
}

// procRealUID reads the real uid of pid. pkexec is setuid, so its effective
// uid is 0 from the start; the real uid turns 0 only after authorization.
func procRealUID(pid int) (int, bool) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, false
	}
	defer f.Close()
	return parseStatusRealUID(f)
}

func parseStatusRealUID(f io.Reader) (int, bool) {
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "Uid:") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "Uid:"))
		if len(fields) == 0 {
			return 0, false
		}
		uid, err := strconv.Atoi(fields[0])
		return uid, err == nil
	}
	return 0, false
}

// WaitForElevationParent blocks until the instance that launched us through
// pkexec has exited, so the single-instance lock does not hand this launch
// back to the quitting parent.
func WaitForElevationParent(args []string) {
	pid := elevatedFromPID(args)
	if pid <= 0 {
		return
	}
	deadline := time.Now().Add(parentExitTimeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(elevationPollInterval)
	}
}

// KeepInvokerOwnership hands files the elevated instance creates in the
// shared data dir back to the user who authorized it: root-owned 0600 files
// would be unreadable to the next non-elevated launch. It repeats while we
// run because GTK can exit() from C (X connection lost at logout), skipping
// any Go-side cleanup. The returned func does a final pass.
func KeepInvokerOwnership() func() {
	uid, gid, ok := invokerIDs()
	if !ok {
		return func() {}
	}
	dir := UserDataDir()
	chownTree(dir, uid, gid)
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(ownershipSyncInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				chownTree(dir, uid, gid)
			case <-stop:
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(stop)
			chownTree(dir, uid, gid)
		})
	}
}

func invokerIDs() (uid, gid int, ok bool) {
	if os.Geteuid() != 0 {
		return 0, 0, false
	}
	uidStr := os.Getenv("PKEXEC_UID")
	uid, err := strconv.Atoi(uidStr)
	if err != nil || uid == 0 {
		return 0, 0, false
	}
	gid = uid
	if u, err := user.LookupId(uidStr); err == nil {
		if g, err := strconv.Atoi(u.Gid); err == nil {
			gid = g
		}
	}
	return uid, gid, true
}

func chownTree(root string, uid, gid int) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, err := d.Info(); err == nil {
			if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) == uid {
				return nil
			}
		}
		_ = os.Lchown(path, uid, gid)
		return nil
	})
}
