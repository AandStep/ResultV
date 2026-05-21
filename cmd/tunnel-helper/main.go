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

// Command tunnel-helper is a privileged macOS helper that owns the sing-box
// engine when the main ResultV app runs in tunnel mode. Splitting the TUN
// device creation into a separate root process lets the main GUI process stay
// under the user's UID — which is required for the macOS Dock context menu
// to function (root processes in a user-session Dock get only the system
// "Force Quit" fallback because cross-UID Apple Events are sandboxed).
//
// Lifecycle (see also internal/tunnelipc/protocol.go):
//   1. Main app builds an internal/proxy.SingBoxConfig for tunnel mode.
//   2. Main app launches this binary via system.RunPrivileged (one osascript
//      password prompt) with --socket, --owner-uid, --main-pid.
//   3. Helper opens a unix-domain socket, chowns it to --owner-uid, accepts a
//      single connection from main, then drives sing-box start/stop based on
//      newline-delimited JSON commands.
//   4. Helper exits cleanly when main disconnects, when main dies (PID
//      probe), or when it receives a "stop" + "shutdown" command pair.
package main

func main() {
	run()
}
