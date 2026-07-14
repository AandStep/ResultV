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

// hasLeftoverTunFn / clearLeftoverTunFn indirect through package vars so the
// Manager's recovery path can be unit-tested without a real sing-tun adapter or
// touching the OS routing table. Production wiring points at the per-platform
// implementations (systun_windows.go / systun_other.go).
var (
	hasLeftoverTunFn        = hasLeftoverTun
	clearLeftoverTunFn      = clearLeftoverTun
	removeStaleTunAdapterFn = removeStaleTunAdapter
)
