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

import "time"

// scanInterval controls how often the OS process table is rescanned. 2s is
// the sweet spot: newly-spawned children join the tunnel exclusions within
// ~2 seconds of launch (imperceptible for typical app launches), while
// keeping the scan rate low enough that platform-specific scanners (notably
// macOS, which goes through CGO/launchctl) don't drive measurable background
// CPU during long-running sessions with per-app whitelist enabled.
//
// Exposed as a var so tests can override it.
var scanInterval = 2 * time.Second

type ticker interface {
	C() <-chan time.Time
	Stop()
}

type realTicker struct{ t *time.Ticker }

func (r *realTicker) C() <-chan time.Time { return r.t.C }
func (r *realTicker) Stop()                { r.t.Stop() }

func newTicker() ticker {
	return &realTicker{t: time.NewTicker(scanInterval)}
}
