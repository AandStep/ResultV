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

// Package processtree answers which processes started a given one, including
// ancestors that have already exited. The routing engine uses it to extend an
// app exclusion to everything the app launched.
package processtree

import (
	"sort"
	"sync"
	"time"
)

// Process is one generation of a PID. Windows hands a freed PID to the next
// process, so a PID alone does not name a process; PID plus creation time does.
type Process struct {
	PID       uint32
	ParentPID uint32
	// Created is in the platform's own clock and is only ever compared with
	// other values from the same Tracker. Zero means the platform cannot tell.
	Created int64
	// Image is the executable path as the OS reports it: a DOS path from a live
	// query, an NT device path from an ETW event, a bare name on some platforms.
	Image string
}

type record struct {
	Process
	// seen is the latest moment the process is known to have been alive.
	seen int64
	// exited is the exact exit time when an event reported it.
	exited int64
	// gone means the process is known to be dead, exit time or not.
	gone bool
}

type source interface {
	// live describes the process currently holding pid.
	live(pid uint32) (Process, bool)
	now() int64
	// young reports whether a creation time lies within d of now.
	young(created int64, d time.Duration) bool
}

const (
	maxGenerations = 4
	maxRecords     = 8192
	maxDepth       = 64
)

// lagWait bounds how long an ancestry walk waits for the event feed to report
// a parent that started and exited moments ago.
const lagWait = 300 * time.Millisecond

// Tracker keeps every generation of every PID it has seen, so a chain survives
// a launcher that exits right after starting the game.
type Tracker struct {
	mu    sync.Mutex
	src   source
	gens  map[uint32][]*record
	count int
	// events is set while a feed reports starts and exits as they happen. A
	// walk that hits an unknown parent then waits for the feed to catch up.
	events bool
	sleep  func(time.Duration)
}

func newTracker(src source) *Tracker {
	return &Tracker{src: src, gens: make(map[uint32][]*record), sleep: time.Sleep}
}

// Observe records a process seen alive at time seen.
func (t *Tracker) Observe(p Process, seen int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.observeLocked(p, seen)
}

// Started records a process start reported by an event feed. Its parent was
// alive at that moment, which is what lets the chain outlive the parent.
func (t *Tracker) Started(p Process) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.observeLocked(p, p.Created)
	if r := t.generationAtLocked(p.ParentPID, p.Created); r != nil && r.seen < p.Created {
		r.seen = p.Created
	}
}

// Exited records an exit reported by an event feed.
func (t *Tracker) Exited(pid uint32, created, at int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, r := range t.gens[pid] {
		if r.Created == created {
			r.exited = at
			r.gone = true
			return
		}
	}
}

// rescan applies a full read of the process table: procs are the processes it
// could describe, present every PID it listed, described or not. A PID missing
// from present has exited.
func (t *Tracker) rescan(procs []Process, present map[uint32]bool, now int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, p := range procs {
		t.observeLocked(p, now)
	}
	for pid, gs := range t.gens {
		if present[pid] {
			continue
		}
		for _, r := range gs {
			r.gone = true
		}
	}
}

func (t *Tracker) setEvents(on bool) {
	t.mu.Lock()
	t.events = on
	t.mu.Unlock()
}

func (t *Tracker) observeLocked(p Process, seen int64) *record {
	gs := t.gens[p.PID]
	for _, r := range gs {
		if r.Created == p.Created {
			if seen > r.seen {
				r.seen = seen
			}
			if r.Image == "" {
				r.Image = p.Image
			}
			if r.ParentPID == 0 {
				r.ParentPID = p.ParentPID
			}
			return r
		}
	}
	r := &record{Process: p, seen: seen}
	gs = append(gs, r)
	sort.Slice(gs, func(i, j int) bool { return gs[i].Created < gs[j].Created })
	// A PID is reused only after its holder exits, so every older generation
	// is dead.
	for _, older := range gs {
		if older.Created < p.Created {
			older.gone = true
		}
	}
	if len(gs) > maxGenerations {
		gs = gs[len(gs)-maxGenerations:]
	}
	t.gens[p.PID] = gs
	t.count++
	if t.count > maxRecords {
		t.evictLocked()
	}
	return r
}

// generationAtLocked returns the generation of pid that was alive at time at,
// or nil when nothing proves one was.
func (t *Tracker) generationAtLocked(pid uint32, at int64) *record {
	gs := t.gens[pid]
	for i := len(gs) - 1; i >= 0; i-- {
		r := gs[i]
		if r.Created > at {
			continue
		}
		// Only the newest generation started before at can have held the PID
		// then: every older one exited before this one started.
		switch {
		case r.exited != 0:
			if r.exited >= at {
				return r
			}
		case r.seen >= at, !r.gone:
			return r
		}
		return nil
	}
	return nil
}

// Identify describes the process holding pid now, without walking its
// ancestors.
func (t *Tracker) Identify(pid uint32) (Process, bool) {
	p, ok := t.src.live(pid)
	if !ok {
		return Process{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.observeLocked(p, t.src.now()).Process, true
}

// Ancestry returns the process holding pid followed by its ancestors, nearest
// first. Nil when pid is unknown.
func (t *Tracker) Ancestry(pid uint32) []Process {
	now := t.src.now()
	self, ok := t.src.live(pid)

	t.mu.Lock()
	defer t.mu.Unlock()
	if ok {
		self = t.observeLocked(self, now).Process
	} else if gs := t.gens[pid]; len(gs) > 0 {
		self = gs[len(gs)-1].Process
	} else {
		return nil
	}

	chain := []Process{self}
	cur := self
	for len(chain) < maxDepth {
		parent, ok := t.parentLocked(cur, now)
		if !ok {
			break
		}
		chain = append(chain, parent)
		cur = parent
	}
	return chain
}

func (t *Tracker) parentLocked(child Process, now int64) (Process, bool) {
	ppid := child.ParentPID
	if ppid == 0 || ppid == child.PID {
		return Process{}, false
	}
	if p, ok := t.src.live(ppid); ok {
		// Alive now and older than the child means it held the PID when the
		// child started: it is the parent.
		if p.Created <= child.Created {
			return t.observeLocked(p, now).Process, true
		}
		t.observeLocked(p, now)
	}
	if r := t.generationAtLocked(ppid, child.Created); r != nil {
		return r.Process, true
	}
	// The parent may have started and exited faster than the feed delivers.
	if !t.events || !t.src.young(child.Created, 10*time.Second) {
		return Process{}, false
	}
	for waited := time.Duration(0); waited < lagWait; waited += 25 * time.Millisecond {
		t.mu.Unlock()
		t.sleep(25 * time.Millisecond)
		t.mu.Lock()
		if r := t.generationAtLocked(ppid, child.Created); r != nil {
			return r.Process, true
		}
	}
	return Process{}, false
}

// Young reports whether p started within d of now.
func (t *Tracker) Young(p Process, d time.Duration) bool {
	return t.src.young(p.Created, d)
}

// Live returns every generation not known to have exited.
func (t *Tracker) Live() []Process {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []Process
	for _, gs := range t.gens {
		for _, r := range gs {
			if !r.gone {
				out = append(out, r.Process)
			}
		}
	}
	return out
}

// evictLocked drops the longest-dead records, keeping three quarters of the
// cap. The living are kept whatever their number.
func (t *Tracker) evictLocked() {
	var dead []*record
	for _, gs := range t.gens {
		for _, r := range gs {
			if r.gone {
				dead = append(dead, r)
			}
		}
	}
	sort.Slice(dead, func(i, j int) bool { return deathTime(dead[i]) < deathTime(dead[j]) })
	drop := t.count - maxRecords*3/4
	if drop > len(dead) {
		drop = len(dead)
	}
	victims := make(map[*record]struct{}, drop)
	for _, r := range dead[:drop] {
		victims[r] = struct{}{}
	}
	for pid, gs := range t.gens {
		kept := gs[:0]
		for _, r := range gs {
			if _, v := victims[r]; !v {
				kept = append(kept, r)
			}
		}
		if len(kept) == 0 {
			delete(t.gens, pid)
		} else {
			t.gens[pid] = kept
		}
	}
	t.count -= drop
}

func deathTime(r *record) int64 {
	if r.exited != 0 {
		return r.exited
	}
	return r.seen
}

var (
	defaultOnce    sync.Once
	defaultTracker *Tracker
)

// Default returns the process-wide tracker, starting it on first use. It keeps
// running for the life of the process: history recorded between sessions is
// what links a game to a launcher that exited before the user connected.
func Default() *Tracker {
	defaultOnce.Do(func() {
		defaultTracker = newTracker(platformSource{})
		startPlatform(defaultTracker)
	})
	return defaultTracker
}

// NewLive returns a tracker that only asks the OS about running processes, with
// no history behind it.
func NewLive() *Tracker {
	return newTracker(platformSource{})
}

// Shutdown stops the platform event feed, if one is running.
func Shutdown() {
	if defaultTracker != nil {
		stopPlatform()
	}
}
