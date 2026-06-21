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

package proxy

type SystemProxy interface {
	Set(addr string, bypass []string) error

	Disable() error

	DisableSync()

	ApplyKillSwitch() error

	// LeftoverActive reports whether a system proxy set by a previous run is
	// still in effect — i.e. the process exited (crash / force-kill) without
	// calling Disable, leaving the OS pointed at our now-dead local port.
	// Detection is marker-based (a file written on Set, removed on Disable),
	// so it is independent of in-memory state. Non-Windows platforms return
	// false: the force-kill leftover problem this guards against is specific
	// to the Windows Internet Settings registry proxy.
	LeftoverActive() bool
}
