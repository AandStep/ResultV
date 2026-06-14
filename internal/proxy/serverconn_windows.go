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
	"encoding/binary"
	"net"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// GetExtendedTcpTable table class + family + state constants we need.
const (
	tcpTableOwnerPIDAll = 5 // TCP_TABLE_OWNER_PID_ALL
	mibTCPStateEstab    = 5 // MIB_TCP_STATE_ESTAB
)

var (
	modIPHlpAPI             = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTCPTable = modIPHlpAPI.NewProc("GetExtendedTcpTable")
)

// mibTCPRowOwnerPID mirrors the Win32 MIB_TCPROW_OWNER_PID (IPv4). All fields
// are DWORDs; addr is network byte order in_addr, port is network byte order in
// the low 16 bits.
type mibTCPRowOwnerPID struct {
	state      uint32
	localAddr  uint32
	localPort  uint32
	remoteAddr uint32
	remotePort uint32
	owningPid  uint32
}

// mibTCP6RowOwnerPID mirrors MIB_TCP6ROW_OWNER_PID (IPv6).
type mibTCP6RowOwnerPID struct {
	localAddr     [16]byte
	localScopeID  uint32
	localPort     uint32
	remoteAddr    [16]byte
	remoteScopeID uint32
	remotePort    uint32
	state         uint32
	owningPid     uint32
}

// netPort decodes a MIB row port DWORD (network byte order in the low word).
func netPort(dw uint32) int {
	b := (*[4]byte)(unsafe.Pointer(&dw))
	return int(binary.BigEndian.Uint16(b[:2]))
}

// establishedServerIP returns the remote IP of an ESTABLISHED TCP connection
// owned by this process whose remote port is serverPort, or "" when none is
// found. See serverconn.go for why this beats re-resolving.
func establishedServerIP(serverPort int) string {
	pid := uint32(os.Getpid())
	rows := queryTCPTable(windows.AF_INET)
	rows = append(rows, queryTCPTable(windows.AF_INET6)...)
	return pickServerIP(rows, pid, serverPort)
}

// queryTCPTable snapshots the OS TCP connection table for one address family
// and projects it into platform-independent rows. Returns nil on any failure
// (best-effort — callers fall back to the existing domain behaviour).
func queryTCPTable(family uint32) []tcpConnRow {
	var size uint32
	// First call sizes the buffer; ERROR_INSUFFICIENT_BUFFER (122) is expected
	// and the return is ignored — we only need the size it writes back.
	procGetExtendedTCPTable.Call(
		0,
		uintptr(unsafe.Pointer(&size)),
		0,
		uintptr(family),
		uintptr(tcpTableOwnerPIDAll),
		0,
	)
	if size == 0 {
		return nil
	}
	buf := make([]byte, size)
	r, _, _ := procGetExtendedTCPTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		0,
		uintptr(family),
		uintptr(tcpTableOwnerPIDAll),
		0,
	)
	if r != 0 { // NO_ERROR == 0
		return nil
	}
	// Layout: DWORD dwNumEntries; ROW table[dwNumEntries]. The row structs are
	// 4-byte aligned, so the table starts at offset 4.
	n := *(*uint32)(unsafe.Pointer(&buf[0]))
	if n == 0 {
		return nil
	}
	out := make([]tcpConnRow, 0, n)
	if family == windows.AF_INET {
		rows := unsafe.Slice((*mibTCPRowOwnerPID)(unsafe.Pointer(&buf[4])), int(n))
		for i := range rows {
			row := &rows[i]
			ipBytes := (*[4]byte)(unsafe.Pointer(&row.remoteAddr))
			out = append(out, tcpConnRow{
				remoteIP:    net.IP(ipBytes[:]).String(),
				remotePort:  netPort(row.remotePort),
				pid:         row.owningPid,
				established: row.state == mibTCPStateEstab,
			})
		}
	} else {
		rows := unsafe.Slice((*mibTCP6RowOwnerPID)(unsafe.Pointer(&buf[4])), int(n))
		for i := range rows {
			row := &rows[i]
			ip := make(net.IP, 16)
			copy(ip, row.remoteAddr[:])
			out = append(out, tcpConnRow{
				remoteIP:    ip.String(),
				remotePort:  netPort(row.remotePort),
				pid:         row.owningPid,
				established: row.state == mibTCPStateEstab,
			})
		}
	}
	return out
}
