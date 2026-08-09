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
	"context"
	"sort"
	"sync"

	"resultproxy-wails/internal/config"
)

// AutoMaxCandidates caps the ranked list handed to callers. Matches
// AUTO_MAX_ATTEMPTS in frontend/src/hooks/useDaemonControl.js so the backend
// and the frontend retry loop never disagree on how many nodes exist.
const AutoMaxCandidates = 5

// autoShortlistSize is how many phase-1 survivors get the expensive DepthFull
// probe. Phase 1 is cheap and wide; phase 2 is accurate and narrow.
const autoShortlistSize = 5

// lastSnapshotMu guards lastSnapshot. RankAutoCandidates is reachable from
// both the tray and the frontend, so a bare package-level slice would race.
var lastSnapshotMu sync.Mutex
var lastSnapshot []AutoProbeResult

// LastAutoProbeSnapshot returns the phase-1 results of the most recent
// RankAutoCandidates call, for diagnostics. Guarded because the ranking runs
// from both the tray and the frontend.
func LastAutoProbeSnapshot() []AutoProbeResult {
	lastSnapshotMu.Lock()
	defer lastSnapshotMu.Unlock()
	return append([]AutoProbeResult(nil), lastSnapshot...)
}

// RankAutoCandidates probes members in two phases and returns them best-first.
//
// Phase 1 sweeps every member with DepthFast to drop the dead ones. Phase 2
// re-probes the shortlist with DepthFull, which is the only stage that can see
// SNI blocking. previousKey (may be empty) is force-included in the shortlist
// so the node currently in use is always measured on equal footing — without it
// a node just outside the top-5 would flap in and out of consideration.
//
// Returns nil when nothing is reachable; callers fall back to the AUTO head.
func RankAutoCandidates(ctx context.Context, members []config.ProxyEntry, previousKey string) []config.ProxyEntry {
	byKey := make(map[string]config.ProxyEntry, len(members))
	for _, m := range members {
		byKey[AutoNodeKey(m)] = m
	}

	fast := ProbeAutoNodes(ctx, members, DepthFast)

	// Snapshot phase-1 results for diagnostics: callers (ResolveAutoCandidates)
	// build the per-member RTT/reason table from this, since after ranking they
	// only have the survivors, not the full sweep. Store a copy, not the live
	// slice — a concurrent rank must not mutate what a reader is iterating.
	lastSnapshotMu.Lock()
	lastSnapshot = append([]AutoProbeResult(nil), fast...)
	lastSnapshotMu.Unlock()

	alive := make([]AutoProbeResult, 0, len(fast))
	for _, r := range fast {
		if r.OK {
			alive = append(alive, r)
		}
	}
	if len(alive) == 0 {
		return nil
	}
	sort.SliceStable(alive, func(i, j int) bool { return alive[i].RTTms < alive[j].RTTms })

	shortlist := make([]config.ProxyEntry, 0, autoShortlistSize+1)
	seen := make(map[string]bool, autoShortlistSize+1)
	for _, r := range alive {
		if len(shortlist) >= autoShortlistSize {
			break
		}
		shortlist = append(shortlist, byKey[r.Key])
		seen[r.Key] = true
	}
	if previousKey != "" && !seen[previousKey] {
		for _, r := range alive {
			if r.Key == previousKey {
				shortlist = append(shortlist, byKey[previousKey])
				break
			}
		}
	}

	full := ProbeAutoNodes(ctx, shortlist, DepthFull)
	scored := make([]AutoProbeResult, 0, len(full))
	for _, r := range full {
		if r.OK {
			scored = append(scored, r)
		}
	}
	if len(scored) == 0 {
		return nil
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].RTTms < scored[j].RTTms })

	out := make([]config.ProxyEntry, 0, AutoMaxCandidates)
	for _, r := range scored {
		if len(out) >= AutoMaxCandidates {
			break
		}
		out = append(out, byKey[r.Key])
	}
	return out
}
