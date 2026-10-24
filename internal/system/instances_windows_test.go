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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

const instancesHelperEnv = "RV_INSTANCES_HELPER_SLEEP"

// TestInstancesHelperProcess is not a test: it is the lingering instance the
// tests below start under a name of their own.
func TestInstancesHelperProcess(t *testing.T) {
	d, err := time.ParseDuration(os.Getenv(instancesHelperEnv))
	if err != nil {
		return
	}
	time.Sleep(d)
}

// startLingering runs a copy of this test binary under a unique name, so the
// search can only ever find processes this test started.
func startLingering(t *testing.T, sleep time.Duration) (string, *exec.Cmd) {
	t.Helper()
	name := fmt.Sprintf("rvwait-%d-%d.exe", os.Getpid(), time.Now().UnixNano())
	path := filepath.Join(t.TempDir(), name)
	src, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dst, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	dst.Close()

	cmd := exec.Command(path, "-test.run", "^TestInstancesHelperProcess$")
	cmd.Env = append(os.Environ(), instancesHelperEnv+"="+sleep.String())
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	return name, cmd
}

func TestAwaitProcessesNamedWaitsForAnInstanceThatLeaves(t *testing.T) {
	name, cmd := startLingering(t, 500*time.Millisecond)

	rep := awaitProcessesNamed(name, 20*time.Second, 2*time.Second)
	if !slices.Contains(rep.Found, uint32(cmd.Process.Pid)) {
		t.Fatalf("the instance was not found: %+v", rep)
	}
	if len(rep.Killed) != 0 || len(rep.Stuck) != 0 {
		t.Fatalf("an instance that leaves on its own must not be killed: %+v", rep)
	}
}

func TestAwaitProcessesNamedEndsALingeringInstance(t *testing.T) {
	name, cmd := startLingering(t, time.Minute)

	start := time.Now()
	rep := awaitProcessesNamed(name, 300*time.Millisecond, 10*time.Second)
	if !slices.Contains(rep.Killed, uint32(cmd.Process.Pid)) || len(rep.Stuck) != 0 {
		t.Fatalf("a lingering instance must be ended: %+v", rep)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("took %s", took)
	}
	if _, err := cmd.Process.Wait(); err != nil {
		t.Fatalf("process still there: %v", err)
	}
}

func TestAwaitProcessesNamedSkipsItself(t *testing.T) {
	self := filepath.Base(os.Args[0])
	rep := awaitProcessesNamed(self, 0, 0)
	if slices.Contains(rep.Found, uint32(os.Getpid())) {
		t.Fatalf("found itself: %+v", rep)
	}
}
