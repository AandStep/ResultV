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

package main

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"resultproxy-wails/internal/proxy"
)

func TestControlCenterSocket(t *testing.T) {
	// Darwin limits the complete Unix socket path to 103 bytes.
	dir, err := os.MkdirTemp("/tmp", "rvctl-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "ctl.sock")
	uid, gid := uint32(os.Getuid()), uint32(os.Getgid())
	stale, err := listenControlSocket(path, uid, gid)
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Close()
	if other, err := listenControlSocket(path, uid, gid); err == nil {
		other.Close()
		t.Fatal("replaced live listener")
	}
	if err := stale.Close(); err != nil {
		t.Fatal(err)
	}

	listener, err := listenControlSocket(path, uid, gid)
	if err != nil {
		t.Fatalf("replace stale listener: %v", err)
	}
	defer listener.Close()
	app := &App{proxy: proxy.NewManager(nil)}
	var busy atomic.Bool
	for _, tc := range []struct {
		name    string
		command byte
		peer    uint32
		reply   []byte
	}{
		{"status", 's', uid, []byte("0")},
		{"already disconnected", '0', uid, []byte("0")},
		{"unknown command", 'x', uid, []byte("e")},
		{"different UID", 's', uid + 1, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := net.DialTimeout("unix", path, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			_ = client.SetDeadline(time.Now().Add(time.Second))
			_ = listener.SetDeadline(time.Now().Add(time.Second))
			if _, err := client.Write([]byte{tc.command}); err != nil {
				t.Fatal(err)
			}
			conn, err := listener.AcceptUnix()
			if err != nil {
				t.Fatal(err)
			}
			action := app.handleVPNControl(conn, tc.peer, &busy)
			_ = conn.Close()
			if action != nil {
				t.Fatal("unexpected connection change")
			}
			reply, err := io.ReadAll(client)
			if !bytes.Equal(reply, tc.reply) || (len(tc.reply) > 0 && err != nil) {
				t.Fatalf("reply = %q, err = %v; want %q", reply, err, tc.reply)
			}
		})
	}
}
