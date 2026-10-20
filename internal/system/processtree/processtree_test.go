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
	"sync"
	"testing"
	"time"
)

type fakeSource struct {
	mu    sync.Mutex
	procs map[uint32]Process
	clock int64
}

func newFakeSource(procs ...Process) *fakeSource {
	s := &fakeSource{procs: map[uint32]Process{}, clock: 1000}
	for _, p := range procs {
		s.procs[p.PID] = p
	}
	return s
}

func (s *fakeSource) live(pid uint32) (Process, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.procs[pid]
	return p, ok
}

func (s *fakeSource) now() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clock
}

func (s *fakeSource) young(created int64, d time.Duration) bool {
	return s.now()-created < 100
}

func (s *fakeSource) exit(pid uint32) {
	s.mu.Lock()
	delete(s.procs, pid)
	s.mu.Unlock()
}

func pids(chain []Process) []uint32 {
	out := make([]uint32, len(chain))
	for i, p := range chain {
		out[i] = p.PID
	}
	return out
}

func assertChain(t *testing.T, got []Process, want ...uint32) {
	t.Helper()
	ids := pids(got)
	if len(ids) != len(want) {
		t.Fatalf("chain %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("chain %v, want %v", ids, want)
		}
	}
}

var (
	explorer = Process{PID: 100, ParentPID: 50, Created: 10, Image: `C:\Windows\explorer.exe`}
	steam    = Process{PID: 200, ParentPID: 100, Created: 20, Image: `C:\Steam\steam.exe`}
	launcher = Process{PID: 300, ParentPID: 200, Created: 30, Image: `C:\Games\launcher.exe`}
	game     = Process{PID: 400, ParentPID: 300, Created: 40, Image: `C:\Games\game.exe`}
)

func TestAncestryWalksLiveParents(t *testing.T) {
	src := newFakeSource(explorer, steam, launcher, game)
	tr := newTracker(src)
	assertChain(t, tr.Ancestry(game.PID), 400, 300, 200, 100)
}

func TestAncestryUnknownPID(t *testing.T) {
	tr := newTracker(newFakeSource())
	if got := tr.Ancestry(999); got != nil {
		t.Fatalf("unknown pid gave %v", pids(got))
	}
}

// A newer process now holding the parent's PID is not the parent.
func TestAncestryRejectsReusedParentPID(t *testing.T) {
	impostor := Process{PID: 300, ParentPID: 100, Created: 50, Image: `C:\Other\other.exe`}
	src := newFakeSource(explorer, steam, impostor, game)
	tr := newTracker(src)
	assertChain(t, tr.Ancestry(game.PID), 400)
}

// The launcher exited after starting the game; the start events recorded it.
func TestAncestryKeepsExitedLauncher(t *testing.T) {
	src := newFakeSource(explorer, steam, game)
	tr := newTracker(src)
	tr.Started(launcher)
	tr.Started(game)
	tr.Exited(launcher.PID, launcher.Created, 45)
	assertChain(t, tr.Ancestry(game.PID), 400, 300, 200, 100)
}

// The launcher exited and its PID went to another process; the history still
// knows which generation started the game.
func TestAncestryKeepsExitedLauncherAfterReuse(t *testing.T) {
	impostor := Process{PID: 300, ParentPID: 100, Created: 50}
	src := newFakeSource(explorer, steam, game, impostor)
	tr := newTracker(src)
	tr.Started(launcher)
	tr.Started(game)
	tr.Exited(launcher.PID, launcher.Created, 45)
	tr.Started(impostor)
	assertChain(t, tr.Ancestry(game.PID), 400, 300, 200, 100)
}

// A generation that exited before the child started cannot be its parent,
// even when the real parent was never recorded.
func TestAncestryRejectsGenerationDeadBeforeChild(t *testing.T) {
	old := Process{PID: 300, ParentPID: 200, Created: 5}
	src := newFakeSource(explorer, steam, game)
	tr := newTracker(src)
	tr.Started(old)
	tr.Exited(old.PID, old.Created, 8)
	assertChain(t, tr.Ancestry(game.PID), 400)
}

// Seen alive by a scan, then gone without an event: still the parent, since
// nothing says the PID changed hands.
func TestAncestryTrustsScannedParentNotYetSwept(t *testing.T) {
	src := newFakeSource(explorer, steam, launcher, game)
	tr := newTracker(src)
	tr.Observe(launcher, 35)
	src.exit(launcher.PID)
	assertChain(t, tr.Ancestry(game.PID), 400, 300, 200, 100)
}

// Once a scan finds the PID missing, a parent seen only before the child
// started proves nothing.
func TestAncestryDropsSweptParentSeenBeforeChild(t *testing.T) {
	src := newFakeSource(explorer, steam, launcher, game)
	tr := newTracker(src)
	tr.Observe(launcher, 35)
	src.exit(launcher.PID)
	tr.rescan(nil, map[uint32]bool{explorer.PID: true, steam.PID: true, game.PID: true}, 60)
	assertChain(t, tr.Ancestry(game.PID), 400)
}

// With an event feed running, a walk waits for a parent the feed has not
// delivered yet.
func TestAncestryWaitsForLaggingFeed(t *testing.T) {
	src := newFakeSource(explorer, steam, game)
	src.clock = 50
	tr := newTracker(src)
	tr.setEvents(true)
	var once sync.Once
	tr.sleep = func(time.Duration) {
		once.Do(func() {
			tr.Started(launcher)
			tr.Exited(launcher.PID, launcher.Created, 45)
		})
	}
	assertChain(t, tr.Ancestry(game.PID), 400, 300, 200, 100)
}

func TestAncestryDoesNotWaitWithoutFeed(t *testing.T) {
	src := newFakeSource(explorer, steam, game)
	tr := newTracker(src)
	tr.sleep = func(time.Duration) { t.Fatal("waited without an event feed") }
	assertChain(t, tr.Ancestry(game.PID), 400)
}

func TestAncestryStopsAtDepthLimit(t *testing.T) {
	src := newFakeSource()
	for i := uint32(1); i <= 2*maxDepth; i++ {
		src.procs[i] = Process{PID: i, ParentPID: i + 1, Created: int64(1000 - i)}
	}
	tr := newTracker(src)
	if got := len(tr.Ancestry(1)); got != maxDepth {
		t.Fatalf("chain length %d, want %d", got, maxDepth)
	}
}

func TestLiveSkipsExited(t *testing.T) {
	tr := newTracker(newFakeSource())
	tr.Started(steam)
	tr.Started(launcher)
	tr.Exited(launcher.PID, launcher.Created, 45)
	live := tr.Live()
	if len(live) != 1 || live[0].PID != steam.PID {
		t.Fatalf("live = %v", pids(live))
	}
}

func TestEvictionKeepsTheLiving(t *testing.T) {
	tr := newTracker(newFakeSource())
	tr.Started(steam)
	for i := uint32(0); i < maxRecords+10; i++ {
		p := Process{PID: 10000 + i, ParentPID: steam.PID, Created: int64(100 + i)}
		tr.Started(p)
		tr.Exited(p.PID, p.Created, int64(101+i))
	}
	if tr.count > maxRecords {
		t.Fatalf("count %d over the cap", tr.count)
	}
	live := tr.Live()
	if len(live) != 1 || live[0].PID != steam.PID {
		t.Fatalf("eviction dropped a living process, live = %v", pids(live))
	}
}
