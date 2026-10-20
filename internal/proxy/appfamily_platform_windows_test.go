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
	"net"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"github.com/sagernet/sing-box/adapter"

	"resultproxy-wails/internal/system/processtree"
)

// The core hands the platform a connection's addresses and gets the owner
// back; the family tag has to ride along on a real socket.
func TestFamilyPlatform_TagsTheConnectionOwner(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		if c, err := ln.Accept(); err == nil {
			defer c.Close()
			_, _ = c.Read(make([]byte, 1))
		}
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	saved := selfPID
	selfPID = 0
	defer func() { selfPID = saved }()

	exe, _ := os.Executable()
	platform := newFamilyPlatform(newFamilyResolver(newAppLists([]string{filepath.Base(exe)}, nil), processtree.NewLive()))
	local := conn.LocalAddr().(*net.TCPAddr)
	remote := conn.RemoteAddr().(*net.TCPAddr)
	owner, err := platform.FindConnectionOwner(&adapter.FindConnectionOwnerRequest{
		IpProtocol:         syscall.IPPROTO_TCP,
		SourceAddress:      local.IP.String(),
		SourcePort:         int32(local.Port),
		DestinationAddress: remote.IP.String(),
		DestinationPort:    int32(remote.Port),
	})
	if err != nil {
		t.Fatal(err)
	}
	if owner.ProcessID != uint32(os.Getpid()) {
		t.Fatalf("owner pid %d, want %d", owner.ProcessID, os.Getpid())
	}
	if !slices.Equal(owner.AndroidPackageNames, []string{familyTagDirect}) {
		t.Fatalf("owner tags %v, want the excluded family", owner.AndroidPackageNames)
	}
}
