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

package processtree

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizeRootsDedupesAndStrips(t *testing.T) {
	got := normalizeRoots([]string{"Steam.exe", "STEAM.EXE", "", `C:\Foo\Bar\app.EXE`, "  ", "discord"})
	want := []string{"steam.exe", "app.exe", "discord"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeRoots: got %v, want %v", got, want)
	}
}

func TestSnapshotEqualIgnoresRoots(t *testing.T) {
	a := Snapshot{Roots: []string{"steam.exe"}, Descendants: []string{"a", "b"}}
	b := Snapshot{Roots: []string{"chrome.exe"}, Descendants: []string{"a", "b"}}
	if !a.Equal(b) {
		t.Fatal("snapshots with same descendants should be equal regardless of roots")
	}
	c := Snapshot{Roots: []string{"steam.exe"}, Descendants: []string{"a"}}
	if a.Equal(c) {
		t.Fatal("snapshots with different descendants must not be equal")
	}
}

func TestMonitorEmitsOnChangeAndDedupes(t *testing.T) {
	m := New(nil)

	var counter atomic.Int32
	var lastSnap Snapshot
	var mu sync.Mutex

	m.OnChange(func(s Snapshot) {
		counter.Add(1)
		mu.Lock()
		lastSnap = s
		mu.Unlock()
	})

	state := []string{"a.exe", "b.exe"}
	var stateMu sync.Mutex
	m.scan = func(roots []string) Snapshot {
		stateMu.Lock()
		defer stateMu.Unlock()
		out := append([]string(nil), state...)
		sort.Strings(out)
		return Snapshot{Roots: roots, Descendants: out}
	}

	m.SetRoots([]string{"steam.exe"})
	m.Trigger()

	if got := counter.Load(); got != 1 {
		t.Fatalf("expected 1 emit after first scan, got %d", got)
	}
	mu.Lock()
	if !reflect.DeepEqual(lastSnap.Descendants, []string{"a.exe", "b.exe"}) {
		t.Fatalf("unexpected descendants: %v", lastSnap.Descendants)
	}
	mu.Unlock()

	// Same scan result -> no emit.
	m.Trigger()
	if got := counter.Load(); got != 1 {
		t.Fatalf("expected no second emit when scan unchanged, got %d", got)
	}

	// Add a child -> emit.
	stateMu.Lock()
	state = append(state, "c.exe")
	stateMu.Unlock()
	m.Trigger()
	if got := counter.Load(); got != 2 {
		t.Fatalf("expected 2nd emit after change, got %d", got)
	}

	// Remove child -> emit.
	stateMu.Lock()
	state = []string{"a.exe"}
	stateMu.Unlock()
	m.Trigger()
	if got := counter.Load(); got != 3 {
		t.Fatalf("expected 3rd emit after removal, got %d", got)
	}
}

func TestMonitorStartStopGoroutine(t *testing.T) {
	m := New(nil)
	m.scan = func(roots []string) Snapshot {
		return Snapshot{Roots: roots}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)
	// Calling Start a second time must not spawn another goroutine.
	m.Start(ctx)
	m.Stop()

	// After Stop, a fresh Start should work again.
	m.Start(context.Background())
	m.Stop()
}

func TestMonitorEmptyRootsEmitsEmptyDescendants(t *testing.T) {
	m := New(nil)
	var snap Snapshot
	var got atomic.Bool
	m.OnChange(func(s Snapshot) {
		snap = s
		got.Store(true)
	})
	m.scan = func(roots []string) Snapshot {
		// platformScan returns empty descendants for empty roots; emulate that.
		return Snapshot{Roots: roots}
	}
	m.SetRoots(nil)
	m.Trigger()
	// First scan against zero-value last is a change (empty Descendants vs
	// empty), but Equal compares descendant slices and both are nil so no emit.
	if got.Load() {
		t.Fatalf("did not expect emit on initial empty-equal-empty scan, snap=%+v", snap)
	}
}

func TestScanIntervalIsReasonable(t *testing.T) {
	if scanInterval <= 0 || scanInterval > 2*time.Second {
		t.Fatalf("scanInterval out of expected range: %v", scanInterval)
	}
}
