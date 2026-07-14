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

package system

import "golang.org/x/sys/windows"

// RaiseProcessPriority bumps the current process to HIGH_PRIORITY_CLASS while a
// tunnel session is active, and RestoreProcessPriority drops it back to NORMAL.
//
// This is the load-bearing fix for the "all connections die at once when the dev
// machine runs `go test ./...` / a full build" collapse. The sing-box engine
// runs in-process; under sustained 100%-on-every-core load its goroutines starve
// for scheduler time, stop draining the TUN/listener sockets, and the OS then
// aborts every tunneled TCP connection at once (WSAECONNABORTED). Empirically
// (2026-07-07, controlled CPU-burner runs) a Normal-priority engine collapsed in
// ~70s of full-core load, while at HIGH the same burner left the tunnel fully
// healthy for the entire run. HIGH (not ABOVE_NORMAL) is what was proven to
// win against a full field of Normal-priority compiler/test threads.
//
// Scope is deliberately narrow: only elevated while connected, restored on
// disconnect, so we never leave a background GUI app permanently starving the
// system. Errors are returned for logging but are non-fatal — a failed priority
// bump only forfeits the hardening, it must never block a connect.
func RaiseProcessPriority() error {
	return windows.SetPriorityClass(windows.CurrentProcess(), windows.HIGH_PRIORITY_CLASS)
}

// RestoreProcessPriority returns the process to the normal scheduling class.
func RestoreProcessPriority() error {
	return windows.SetPriorityClass(windows.CurrentProcess(), windows.NORMAL_PRIORITY_CLASS)
}
