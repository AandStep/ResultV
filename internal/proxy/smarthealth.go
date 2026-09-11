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

	"resultproxy-wails/internal/verdict"
)

const (
	// directHealthDistinctFailures is how many unrelated sites have to fail
	// before the direct path is presumed broken rather than censored. Five,
	// counted by parent domain: one site failing is a site, five unrelated ones
	// at once is a link.
	directHealthDistinctFailures = 5
	// directHealthWindow is how long one failure stays evidence. An outage is a
	// burst; failures spread over minutes are just the internet.
	directHealthWindow = 30 * time.Second
	// directHealthCooldown is how long the engine stops learning and stops
	// hedging once it has decided the link is broken.
	directHealthCooldown = 2 * time.Minute
)

// directHealth watches whether the direct path is working at all.
//
// It exists because the cost of being wrong is asymmetric: a blocked verdict
// lives for seven days, so a two-minute outage recorded as censorship follows
// the user around for a week.
type directHealth struct {
	mu  sync.Mutex
	now func() time.Time

	failures     map[string]time.Time
	trippedUntil time.Time
}

func newDirectHealth(now func() time.Time) *directHealth {
	if now == nil {
		now = time.Now
	}
	return &directHealth{now: now, failures: make(map[string]time.Time)}
}

// healthy reports whether the engine may still trust what it measures.
func (h *directHealth) healthy() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.now().Before(h.trippedUntil)
}

// record files one direct-path outcome, keyed by parent domain so that a site
// and its subdomains count once.
func (h *directHealth) record(host string, ok bool) {
	key := healthKey(host)
	if key == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()

	if ok {
		// The link demonstrably works. Everything filed against it has stopped
		// being evidence of anything.
		h.failures = make(map[string]time.Time)
		return
	}

	h.failures[key] = now
	for k, at := range h.failures {
		if now.Sub(at) >= directHealthWindow {
			delete(h.failures, k)
		}
	}
	if len(h.failures) >= directHealthDistinctFailures {
		h.trippedUntil = now.Add(directHealthCooldown)
		h.failures = make(map[string]time.Time)
	}
}

// healthKey collapses a host to the site it belongs to.
func healthKey(host string) string {
	key := verdict.NormalizeHost(host)
	if key == "" {
		return ""
	}
	if parent := verdict.ParentDomain(key); parent != "" {
		return parent
	}
	return key
}
