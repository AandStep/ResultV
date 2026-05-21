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

//go:build darwin

package main

// ensureAdmin is intentionally a no-op on macOS.
//
// Previously we relaunched the entire process under root via osascript "do
// shell script ... with administrator privileges". That made every operation
// available, but it also broke the macOS Dock context menu: a root process
// living inside a user-session WindowServer can't fully participate in the
// cross-UID Apple Events Dock relies on, so right-click reverted to a
// hard-coded "Force Quit" fallback (cmd+q still worked because that path is
// in-process and doesn't cross UID boundaries).
//
// Privilege is now requested per-operation:
//   - networksetup (system proxy)  -> system.RunPrivileged
//   - pfctl (kill switch)          -> system.RunPrivileged
//   - sing-box TUN device          -> runs in a dedicated root helper
//                                     (cmd/tunnel-helper), launched on demand
//                                     when tunnel mode is enabled.
//
// The first time the user does any of those, macOS pops a single osascript
// password prompt; everything else stays unprivileged and the Dock menu
// works normally.
func ensureAdmin() {}
