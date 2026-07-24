// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mitm

import (
	"bufio"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AdguardTeam/urlfilter/rules"

	"resultproxy-wails/internal/filter/ca"
)

// TestClose_ReturnsPromptlyWithActiveTunnel is the regression test for the
// Android "disconnect hangs" bug: gomitmproxy's Close() waits on conns.Wait(),
// and a plain (non-MITM'd) CONNECT tunnel sits in a pair of io.Copy calls that
// never observe the closing signal — their only exit is the absolute 5-minute
// read deadline. With Chrome keeping tunnels open, StopMITM blocked for
// minutes, BoxModule.stop() queued behind it never ran, the TUN (with
// setHttpProxy) stayed up pointing at a dead proxy, and reconnect was ignored
// by the still-"running" BoxModule.
//
// Here we hold a live CONNECT tunnel to a MITM-exception host (google.com,
// upstream dial diverted to a local stub) and require Close() to return
// within seconds, not minutes.
func TestClose_ReturnsPromptlyWithActiveTunnel(t *testing.T) {
	root, err := ca.EnsureRoot(t.TempDir(), "")
	if err != nil {
		t.Fatalf("ca.EnsureRoot: %v", err)
	}

	// Empty filter list — filtering is irrelevant here. os.MkdirTemp instead of
	// t.TempDir for the same Windows file-handle reason as the other MITM tests.
	listDir, err := os.MkdirTemp("", "mitm-close-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(listDir) }()
	filterFile := filepath.Join(listDir, "list.txt")
	if err := os.WriteFile(filterFile, []byte("! empty\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Stub origin: accepts the upstream side of the tunnel, proves a byte made
	// it through, then idles (like a real keep-alive TLS connection would).
	stub, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer stub.Close()
	gotByte := make(chan struct{})
	go func() {
		conn, err := stub.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 1)
		if _, err := conn.Read(buf); err == nil {
			close(gotByte)
		}
		// Idle: hold the connection open until the listener/test tears down.
		buf2 := make([]byte, 1)
		_, _ = conn.Read(buf2)
	}()

	upstream := func(network, addr string) (net.Conn, error) {
		return net.Dial("tcp", stub.Addr().String())
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	srv, err := NewServer(Config{
		ListenPort:   port,
		RootCert:     root.Certificate,
		RootKey:      root.PrivateKey,
		FilterPaths:  map[rules.ListID]string{1: filterFile},
		UpstreamDial: upstream,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Open a CONNECT tunnel to a MITM-exception host so the proxy takes the
	// plain passthrough path (the one that blocks Close). Retry the dial while
	// the listener comes up.
	var client net.Conn
	for i := 0; i < 20; i++ {
		client, err = net.Dial("tcp", "127.0.0.1:"+itoa(port))
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("dialing proxy: %v", err)
	}
	defer client.Close()

	if _, err := client.Write([]byte("CONNECT google.com:443 HTTP/1.1\r\nHost: google.com:443\r\n\r\n")); err != nil {
		t.Fatalf("writing CONNECT: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatalf("reading CONNECT response: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status = %d, want 200", resp.StatusCode)
	}

	// Push one byte through and wait for the stub to see it — proves the
	// tunnel's copy goroutines are live before we ask the proxy to close.
	if _, err := client.Write([]byte{'x'}); err != nil {
		t.Fatalf("writing through tunnel: %v", err)
	}
	select {
	case <-gotByte:
	case <-time.After(5 * time.Second):
		t.Fatal("tunnel byte never reached the stub origin")
	}

	// The client deliberately stays open: Close() must not need the browser's
	// cooperation to finish.
	closed := make(chan struct{})
	go func() {
		srv.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Server.Close() did not return within 5s while a CONNECT tunnel was active — shutdown would hang the VPN disconnect")
	}

	// After Close the tunnel must actually be dead: the client side should see
	// EOF/reset promptly instead of hanging until its own deadline.
	_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 1)
	if _, err := client.Read(buf); err == nil || strings.Contains(err.Error(), "timeout") {
		t.Fatalf("tunnel client conn still alive after Close (read err = %v)", err)
	}
}
