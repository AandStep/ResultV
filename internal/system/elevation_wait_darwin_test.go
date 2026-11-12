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
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestWaitForElevationParentWaitsForExit(t *testing.T) {
	parent := exec.Command("/bin/sleep", "60")
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	parentExited := make(chan struct{})
	go func() {
		_ = parent.Wait()
		close(parentExited)
	}()
	t.Cleanup(func() {
		_ = parent.Process.Kill()
		<-parentExited
	})

	waited := make(chan struct{})
	go func() {
		WaitForElevationParent([]string{ElevatedFromFlag + strconv.Itoa(parent.Process.Pid)})
		close(waited)
	}()

	select {
	case <-waited:
		t.Fatal("elevated instance did not wait for the parent")
	case <-time.After(150 * time.Millisecond):
	}

	if err := parent.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waited:
	case <-time.After(3 * time.Second):
		t.Fatal("elevated instance did not resume after the parent exited")
	}
}
