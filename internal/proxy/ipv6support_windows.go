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

import "golang.org/x/sys/windows/registry"

// disabledComponentsNonTunnel is the DisabledComponents bit that switches IPv6
// off on non-tunnel interfaces. Wintun reports IF_TYPE_PROP_VIRTUAL, which
// Windows counts as non-tunnel.
const disabledComponentsNonTunnel = 0x10

// hostSupportsIPv6 reports whether the TUN adapter can take an IPv6 address.
// IPv6 unticked on the physical adapters does not matter: the Wintun adapter
// has its own binding. Only the system-wide switch does.
func hostSupportsIPv6() bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Services\Tcpip6\Parameters`, registry.QUERY_VALUE)
	if err != nil {
		return true
	}
	defer key.Close()
	v, _, err := key.GetIntegerValue("DisabledComponents")
	if err != nil {
		return true
	}
	return ipv6EnabledForTun(v)
}

func ipv6EnabledForTun(disabledComponents uint64) bool {
	return disabledComponents&disabledComponentsNonTunnel == 0
}
