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

// Package processtree watches the OS process tree and reports the set of
// descendant executables of a user-specified set of "root" applications.
// Used by the routing engine to keep the app whitelist in sync with the
// current process tree (e.g., adding steamwebhelper.exe automatically when
// steam.exe is in the user's exclusion list).
package processtree

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// Snapshot is the result of a tree scan: the user-supplied roots and the
// discovered descendants. Both lists contain lowercase basenames.
type Snapshot struct {
	Roots       []string
	Descendants []string
}

// Equal reports whether two snapshots have the same descendant set.
// Roots are user-controlled and ignored in equality.
func (s Snapshot) Equal(other Snapshot) bool {
	if len(s.Descendants) != len(other.Descendants) {
		return false
	}
	for i := range s.Descendants {
		if s.Descendants[i] != other.Descendants[i] {
			return false
		}
	}
	return true
}

// Logger is the minimal logging surface used by the monitor.
// Pass nil to silence diagnostic output.
type Logger interface {
	Warning(string)
	Info(string)
}

// Monitor watches the OS process tree on a background goroutine.
// Zero value is not usable — construct via New.
type Monitor struct {
	mu       sync.Mutex
	roots    []string
	onChange func(Snapshot)
	last     Snapshot
	cancel   context.CancelFunc
	log      Logger

	scan func(roots []string) Snapshot
}

// New constructs a platform-specific monitor. Passing a nil logger silences
// diagnostics. Returns a stub that emits empty snapshots on platforms where
// process-tree scanning is not implemented.
func New(log Logger) *Monitor {
	return &Monitor{log: log, scan: platformScan}
}

// SetRoots replaces the set of root applications to watch.
// Roots are normalized to lowercase basenames; empty entries are dropped.
// Safe to call before or after Start.
func (m *Monitor) SetRoots(roots []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roots = normalizeRoots(roots)
}

// OnChange registers a callback invoked whenever the descendant set changes.
// The callback runs on the monitor's goroutine; keep it cheap or hand off
// to another goroutine. Pass nil to clear.
func (m *Monitor) OnChange(fn func(Snapshot)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = fn
}

// Start begins monitoring. Stop or ctx cancellation halts the goroutine.
// Calling Start while already running is a no-op.
func (m *Monitor) Start(ctx context.Context) {
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()

	go m.loop(loopCtx)
}

// Stop halts the monitor goroutine. Safe to call multiple times.
func (m *Monitor) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.last = Snapshot{}
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Trigger forces an immediate scan and emit. Useful right after SetRoots so
// the manager doesn't have to wait for the next tick before reflecting a
// new exclusion list.
func (m *Monitor) Trigger() {
	m.tick()
}

// Scan performs a single one-shot scan and returns the result without starting
// a goroutine. Used by callers that need an initial snapshot before deciding
// whether to spin up a long-running monitor (e.g., engine startup).
//
// Roots are normalized identically to SetRoots — paths are stripped to
// basenames, lowercased, and deduplicated.
func Scan(roots []string) Snapshot {
	normalized := normalizeRoots(roots)
	if len(normalized) == 0 {
		return Snapshot{}
	}
	snap := platformScan(normalized)
	sort.Strings(snap.Descendants)
	return snap
}

func (m *Monitor) loop(ctx context.Context) {
	ticker := newTicker()
	defer ticker.Stop()

	m.tick()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			m.tick()
		}
	}
}

func (m *Monitor) tick() {
	m.mu.Lock()
	roots := append([]string(nil), m.roots...)
	cb := m.onChange
	prev := m.last
	scanFn := m.scan
	m.mu.Unlock()

	if scanFn == nil {
		return
	}

	snap := scanFn(roots)
	sort.Strings(snap.Descendants)

	if snap.Equal(prev) {
		return
	}

	m.mu.Lock()
	m.last = snap
	m.mu.Unlock()

	if cb != nil {
		cb(snap)
	}
}

func normalizeRoots(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.TrimSpace(strings.ToLower(raw))
		if s == "" {
			continue
		}
		// Strip path components if any caller passed a full path.
		if i := strings.LastIndexAny(s, `/\`); i >= 0 {
			s = s[i+1:]
		}
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
