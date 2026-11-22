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

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"resultproxy-wails/internal/system"
)

// osascript runs this launcher after the user authorizes the elevation. A
// background shell job would still inherit the authorization process group.
func launchElevatedSession(args []string) (bool, error) {
	if len(args) < 2 || args[1] != "--launch-elevated-session" {
		return false, nil
	}
	if !system.IsAdmin() {
		return true, fmt.Errorf("elevated session launcher requires administrator privileges")
	}

	executable, err := os.Executable()
	if err != nil {
		return true, fmt.Errorf("locate ResultV executable: %w", err)
	}

	child := exec.Command(executable, args[2:]...)
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		return true, fmt.Errorf("start ResultV in a separate session: %w", err)
	}
	// Start reports failures from setsid and exec. The GUI then waits for the
	// original ResultV to exit using the forwarded --elevated-from argument.
	return true, child.Process.Release()
}
