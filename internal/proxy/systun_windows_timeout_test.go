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

//go:build windows

package proxy

import (
	"strings"
	"testing"
	"time"
)

// A wedged `pnputil /remove-device` used to hang removeStaleTunAdapter forever:
// exec.Command + cmd.Run() carry no deadline, and connectCtx is not threaded
// into this path, so the whole connect stalled with no further log line and no
// way to cancel. Both helpers must come back on their own.
func TestRunCommandHidden_HonoursTimeout(t *testing.T) {
	withShortTunCommandTimeout(t)

	start := time.Now()
	err := runCommandHidden("powershell", "-NoProfile", "-NonInteractive", "-Command", "Start-Sleep -Seconds 60")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a command that outlives the timeout must return an error")
	}
	if !strings.Contains(err.Error(), tunCommandTimeout.String()) {
		t.Fatalf("the error must name the timeout that fired, got %v", err)
	}
	if elapsed > 30*time.Second {
		t.Fatalf("the call must not outlive the timeout by much, took %s", elapsed)
	}
}

func TestRunCommandHiddenOut_HonoursTimeout(t *testing.T) {
	withShortTunCommandTimeout(t)

	start := time.Now()
	_, err := runCommandHiddenOut("powershell", "-NoProfile", "-NonInteractive", "-Command", "Start-Sleep -Seconds 60")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a command that outlives the timeout must return an error")
	}
	if elapsed > 30*time.Second {
		t.Fatalf("the call must not outlive the timeout by much, took %s", elapsed)
	}
}

// A command that finishes inside the budget must be unaffected.
func TestRunCommandHiddenOut_ReturnsOutputWithinTimeout(t *testing.T) {
	out, err := runCommandHiddenOut("powershell", "-NoProfile", "-NonInteractive", "-Command", "Write-Output rvtun-ok")
	if err != nil {
		t.Fatalf("a fast command must succeed, got %v", err)
	}
	if !strings.Contains(string(out), "rvtun-ok") {
		t.Fatalf("stdout must still be captured, got %q", string(out))
	}
}

// powershell.exe itself costs a few hundred ms to start, so the budget has to be
// long enough that the timeout — not the spawn — is what the test measures.
func withShortTunCommandTimeout(t *testing.T) {
	t.Helper()
	prev := tunCommandTimeout
	tunCommandTimeout = 3 * time.Second
	t.Cleanup(func() { tunCommandTimeout = prev })
}
