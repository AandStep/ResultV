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

const (
	// raceLossesToLearn is how many separate lost races stand in for a probe
	// that could not reach a verdict.
	raceLossesToLearn = 3
	// raceLossSpacing keeps one page load, whose connections all lose to the
	// same slow lookup at once, from counting as more than one loss.
	raceLossSpacing = 15 * time.Second
	raceLossesCap   = 4096
)

type raceLoss struct {
	count int
	last  time.Time
}

// raceLosses counts, per destination, the races the direct path lost in a row.
type raceLosses struct {
	mu sync.Mutex
	m  map[string]raceLoss
}

func (l *raceLosses) add(key string, now time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil || len(l.m) >= raceLossesCap {
		l.m = make(map[string]raceLoss)
	}
	e := l.m[key]
	if e.count > 0 && now.Sub(e.last) < raceLossSpacing {
		return e.count
	}
	e.count++
	e.last = now
	l.m[key] = e
	return e.count
}

func (l *raceLosses) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, key)
}
