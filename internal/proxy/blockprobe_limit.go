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

package proxy

import (
	"sync"
	"time"
)

// The prober asks the network questions on the user's behalf, which makes it a
// candidate for exactly the failure this project has already diagnosed once: a
// storm of retries that got the source banned upstream and presented itself as
// the application breaking on its own. These three limits exist so it cannot
// become that storm.
const (
	// probeMaxPerMinute is the global budget. A page load touching thirty new
	// subdomains must not become thirty probes in one second.
	probeMaxPerMinute = 20
	// probeBackoffBase is the first penalty after a host fails; it doubles.
	probeBackoffBase = 30 * time.Second
	// probeBreakerFailures is how many failures in a row trip the breaker.
	probeBreakerFailures = 8
	// probeBreakerCooldown is how long everything stays quiet afterwards.
	probeBreakerCooldown = 5 * time.Minute
)

type hostPenalty struct {
	until   time.Time
	backoff time.Duration
}

// probeGate decides whether one more probe may be sent right now.
type probeGate struct {
	mu  sync.Mutex
	now func() time.Time

	windowStart time.Time
	inWindow    int

	penalties map[string]hostPenalty

	consecutiveFailures int
	breakerUntil        time.Time
}

func newProbeGate(now func() time.Time) *probeGate {
	if now == nil {
		now = time.Now
	}
	return &probeGate{now: now, penalties: make(map[string]hostPenalty)}
}

// open reports whether the breaker is closed, i.e. probing is permitted at all.
func (g *probeGate) open() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return !g.now().Before(g.breakerUntil)
}

func (g *probeGate) allow(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()

	if now.Before(g.breakerUntil) {
		return false
	}
	if now.Sub(g.windowStart) >= time.Minute {
		g.windowStart = now
		g.inWindow = 0
	}
	if g.inWindow >= probeMaxPerMinute {
		return false
	}
	if p, ok := g.penalties[key]; ok && now.Before(p.until) {
		return false
	}
	g.inWindow++
	return true
}

func (g *probeGate) record(key string, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()

	if ok {
		delete(g.penalties, key)
		g.consecutiveFailures = 0
		return
	}

	p := g.penalties[key]
	if p.backoff == 0 {
		p.backoff = probeBackoffBase
	} else {
		p.backoff *= 2
	}
	p.until = now.Add(p.backoff)
	g.penalties[key] = p

	g.consecutiveFailures++
	if g.consecutiveFailures >= probeBreakerFailures {
		g.breakerUntil = now.Add(probeBreakerCooldown)
		g.consecutiveFailures = 0
	}
}
