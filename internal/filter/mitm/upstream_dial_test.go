// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mitm

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdguardTeam/urlfilter/rules"

	"resultproxy-wails/internal/filter/ca"
)

// TestUpstreamDial_UsedForOriginConnections is the regression test for the
// Android tunnel-bypass bug: the MITM proxy's upstream connections MUST go
// through Config.UpstreamDial (our SOCKS-into-the-tunnel dialer), not a direct
// net.Dial. Here we point UpstreamDial at a recording dialer and assert a
// plain-HTTP request routed through the proxy actually dialed via it.
func TestUpstreamDial_UsedForOriginConnections(t *testing.T) {
	// Origin server the browser wants to reach.
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "ok")
	}))
	defer origin.Close()

	root, err := ca.EnsureRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ca.EnsureRoot: %v", err)
	}

	// Minimal (empty) filter list so the engine builds. An empty rule file is
	// a valid urlfilter list — nothing is blocked, which is what we want: this
	// test is about the upstream dial path, not filtering.
	//
	// Use os.MkdirTemp + best-effort RemoveAll (not t.TempDir): urlfilter's
	// list file handle isn't synchronously released on Close under Windows, so
	// t.TempDir's mandatory cleanup would fail the test on an OS artifact
	// unrelated to what's under test (same pattern as manager_mitm_test.go).
	listDir, err := os.MkdirTemp("", "mitm-upstream-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(listDir) }()
	filterFile := filepath.Join(listDir, "list.txt")
	if err := os.WriteFile(filterFile, []byte("! empty\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var dialCount atomic.Int64
	upstream := func(network, addr string) (net.Conn, error) {
		dialCount.Add(1)
		return net.Dial(network, addr)
	}

	// Bind a free port for the MITM proxy.
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
	defer srv.Close()

	// Give the listener a moment to come up.
	proxyURL, _ := url.Parse("http://127.0.0.1:" + itoa(port))
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}

	var lastErr error
	for i := 0; i < 20; i++ {
		resp, err := client.Get(origin.URL) // plain HTTP → transport dial path
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			lastErr = nil
			break
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("request through MITM proxy failed: %v", lastErr)
	}

	if dialCount.Load() == 0 {
		t.Fatal("UpstreamDial was never invoked — MITM upstream bypassed the tunnel dialer (the bug)")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
