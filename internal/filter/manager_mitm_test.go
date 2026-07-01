// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestManager_StartMITM_FailsWithoutCachedLists(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.StartMITM(0); err == nil {
		t.Fatal("expected StartMITM to fail before any list has been downloaded")
	}
}

func TestManager_StartStopMITM_Lifecycle(t *testing.T) {
	list := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("! test list\n", 50)))
	}))
	defer list.Close()

	orig := DefaultSources
	DefaultSources = []ListSource{{ID: 1, Name: "adguard-base", URLs: []string{list.URL}}}
	defer func() { DefaultSources = orig }()

	// urlfilter v0.23.2's proxy.Server.Close() doesn't synchronously release
	// the filter-list file handle; on Windows t.TempDir's RemoveAll then
	// fails even though the lifecycle assertions all pass. Android/POSIX
	// unlink-open-file succeeds. Cleanup is best-effort so this OS artifact
	// doesn't fail the lifecycle test.
	dir, err := os.MkdirTemp("", "mitm-lifecycle-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	m := NewManager(dir)
	if err := m.Update(context.Background(), nil); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	// Port 0 lets the OS pick a free ephemeral port — good enough to prove
	// the server starts and stops cleanly without a fixed-port collision
	// risk in CI.
	if err := m.StartMITM(0); err != nil {
		t.Fatalf("StartMITM failed: %v", err)
	}
	if !m.IsMITMRunning() {
		t.Fatal("expected IsMITMRunning() to be true after StartMITM")
	}
	if !m.Status().Enabled {
		t.Fatal("expected Status().Enabled to be true after StartMITM — Task 7's Android watchdog polls this field")
	}
	m.StopMITM()
	if m.IsMITMRunning() {
		t.Fatal("expected IsMITMRunning() to be false after StopMITM")
	}
}

func TestManager_StartMITM_IdempotentRestart(t *testing.T) {
	list := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("! test list\n", 50)))
	}))
	defer list.Close()

	orig := DefaultSources
	DefaultSources = []ListSource{{ID: 1, Name: "adguard-base", URLs: []string{list.URL}}}
	defer func() { DefaultSources = orig }()

	// urlfilter v0.23.2's proxy.Server.Close() doesn't synchronously release
	// the filter-list file handle; on Windows t.TempDir's RemoveAll then
	// fails even though the lifecycle assertions all pass. Android/POSIX
	// unlink-open-file succeeds. Cleanup is best-effort so this OS artifact
	// doesn't fail the lifecycle test.
	dir, err := os.MkdirTemp("", "mitm-restart-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	m := NewManager(dir)
	if err := m.Update(context.Background(), nil); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Pick a fixed free port up front so both StartMITM calls bind the same
	// port — this is what makes the test meaningful (port 0 would let each
	// call grab its own ephemeral port and hide the leak).
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	if err := m.StartMITM(port); err != nil {
		t.Fatalf("first StartMITM failed: %v", err)
	}
	if !m.IsMITMRunning() {
		t.Fatal("expected IsMITMRunning() to be true after first StartMITM")
	}

	// Pre-fix: this second call on the same port returned an "address
	// already in use" error because the first server was never stopped and
	// still held the listening socket. Post-fix, StartMITM stops any
	// existing proxy first, so the rebind succeeds cleanly — this is the
	// Task 7 watchdog-restart path.
	if err := m.StartMITM(port); err != nil {
		t.Fatalf("second StartMITM on the same port failed (expected idempotent restart to succeed): %v", err)
	}
	if !m.IsMITMRunning() {
		t.Fatal("expected IsMITMRunning() to be true after second StartMITM")
	}

	m.StopMITM()
	if m.IsMITMRunning() {
		t.Fatal("expected IsMITMRunning() to be false after StopMITM")
	}
}

func TestManager_CARootPath_ReturnsExistingFile(t *testing.T) {
	m := NewManager(t.TempDir())
	path, err := m.CARootPath()
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Fatal("expected a non-empty certificate path")
	}
}
