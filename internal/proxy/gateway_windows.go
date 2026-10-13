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

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// ipv4GatewayMetrics maps the index of every adapter holding an IPv4 default
// route to that route's effective metric (route plus interface metric, the sum
// Windows compares). nil when the routing table cannot be read.
func ipv4GatewayMetrics() map[int]uint32 {
	var table *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &table); err != nil {
		return nil
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))

	metrics := map[int]uint32{}
	for _, row := range table.Rows() {
		if row.DestinationPrefix.PrefixLength != 0 {
			continue
		}
		iface := windows.MibIpInterfaceRow{Family: windows.AF_INET, InterfaceLuid: row.InterfaceLuid}
		if err := windows.GetIpInterfaceEntry(&iface); err != nil {
			continue
		}
		metric := row.Metric + iface.Metric
		idx := int(row.InterfaceIndex)
		if old, ok := metrics[idx]; !ok || metric < old {
			metrics[idx] = metric
		}
	}
	return metrics
}
