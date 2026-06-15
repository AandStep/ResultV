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
	"io"
	"net"
	"testing"
)

// TestEstablishedServerIP_FindsLocalConnection exercises the real
// GetExtendedTcpTable syscall + parsing: it opens a live TCP connection from
// this process to a local listener and asserts establishedServerIP finds the
// remote IP by the listener's port. This is the end-to-end proof the pure
// pickServerIP unit tests can't give.
func TestEstablishedServerIP_FindsLocalConnection(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- c
		io.Copy(io.Discard, c) // keep it open + drain
	}()

	conn, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	srv := <-accepted
	defer srv.Close()

	if got := establishedServerIP(port); got != "127.0.0.1" {
		t.Fatalf("establishedServerIP(%d) = %q, want 127.0.0.1", port, got)
	}

	// A port nothing connects to on the remote side yields no match.
	if got := establishedServerIP(1); got != "" {
		t.Fatalf("establishedServerIP(1) = %q, want empty", got)
	}
}
