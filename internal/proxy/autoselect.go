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

	"resultproxy-wails/internal/config"
)

// AutoMaxCandidates caps the ranked list handed to callers. Matches
// AUTO_MAX_ATTEMPTS in frontend/src/hooks/useDaemonControl.js so the backend
// and the frontend retry loop never disagree on how many nodes exist.
const AutoMaxCandidates = 5

// autoShortlistSize is how many phase-1 survivors get the expensive DepthFull
// probe. Phase 1 is cheap and wide; phase 2 is accurate and narrow.
const autoShortlistSize = 5

// RankAutoCandidates probes members in two phases and returns them best-first.
//
// Phase 1 sweeps every member with DepthFast to drop the dead ones. Phase 2
// re-probes the shortlist with DepthFull, which is the only stage that can see
// SNI blocking. previousKey (may be empty) is force-included in the shortlist
// so the node currently in use is always measured on equal footing — without it
// a node just outside the top-5 would flap in and out of consideration.
//
// Returns candidates nil when nothing is reachable; callers fall back to the
// AUTO head. phase1 carries the raw phase-1 probe rows and is returned on
// EVERY exit path (including the nil-candidates ones) instead of being stashed
// in package state: this function is called concurrently from independent
// callers (tray clicks each run on their own goroutine — see tray.go), and a
// shared package-level "last snapshot" would let one caller's diagnostic table
// silently show another caller's rows. Returning phase1 as an ordinary value
// ties it to the call that produced it, so there is nothing left to race.
func RankAutoCandidates(ctx context.Context, members []config.ProxyEntry, previousKey string) (candidates []config.ProxyEntry, phase1 []AutoProbeResult) {
	// Deferred rather than a single call before the last return: this function
	// has early returns (no reachable node in phase 1 or phase 2) and those
	// probe results are exactly the diagnostic data worth keeping across a
	// restart, not just the happy path's.
	defer func() { _ = nodeStats().Flush() }()

	byKey := make(map[string]config.ProxyEntry, len(members))
	for _, m := range members {
		byKey[AutoNodeKey(m)] = m
	}

	fast := ProbeAutoNodes(ctx, members, DepthFast)
	phase1 = fast
	for _, r := range fast {
		nodeStats().RecordProbe(r.Key, r.RTTms, r.JitterMs, r.OK, r.Reason)
	}

	alive := make([]AutoProbeResult, 0, len(fast))
	for _, r := range fast {
		if r.OK {
			alive = append(alive, r)
		}
	}
	if len(alive) == 0 {
		return nil, phase1
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
	for _, r := range full {
		nodeStats().RecordProbe(r.Key, r.RTTms, r.JitterMs, r.OK, r.Reason)
	}
	scored := make([]AutoProbeResult, 0, len(full))
	for _, r := range full {
		if r.OK {
			scored = append(scored, r)
		}
	}
	if len(scored) == 0 {
		return nil, phase1
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].RTTms < scored[j].RTTms })

	out := make([]config.ProxyEntry, 0, AutoMaxCandidates)
	for _, r := range scored {
		if len(out) >= AutoMaxCandidates {
			break
		}
		out = append(out, byKey[r.Key])
	}
	return out, phase1
}
