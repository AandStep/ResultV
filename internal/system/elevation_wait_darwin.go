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

package system

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// WaitForElevationParent lets the old GUI finish network cleanup before the
// elevated instance opens the user profile and attempts a connection.
func WaitForElevationParent(args []string) {
	pid := elevatedFromPID(args)
	if pid <= 0 || pid == os.Getpid() {
		return
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}
