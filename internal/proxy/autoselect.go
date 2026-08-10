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
	"encoding/json"
	"sort"
	"strings"
	"time"

	"resultproxy-wails/internal/config"
)

// AutoMaxCandidates caps the ranked list handed to callers. Matches
// AUTO_MAX_ATTEMPTS in frontend/src/hooks/useDaemonControl.js so the backend
// and the frontend retry loop never disagree on how many nodes exist.
const AutoMaxCandidates = 5

// autoShortlistSize is how many phase-1 survivors get the expensive DepthFull
// probe. Phase 1 is cheap and wide; phase 2 is accurate and narrow.
const autoShortlistSize = 5

// autoTolerance is the head start the node currently in use keeps. A rival must
// beat it by more than this to take over. Mirrors the `tolerance` knob in
// Clash's url-test and sing-box's urltest, which exists for the same reason:
// without it two nodes a few milliseconds apart swap on every evaluation.
const autoTolerance = 50.0

const (
	// autoConsecFailPenalty is applied once per consecutive real-connect
	// failure (capped, see autoConsecFailCap). It is sized in the thousands
	// because RTTs here are measured in tens of milliseconds — no amount of
	// speed should let a node that will not connect outrank one that does.
	autoConsecFailPenalty = 2000.0
	// autoRecentFailPenalty applies once while the node's last connect
	// failure is still within autoRecentFailWindow, on top of any
	// autoConsecFailPenalty — a node that just failed is worse than one whose
	// last failure has aged out even at the same ConsecFails count.
	autoRecentFailPenalty = 1500.0
	// autoRecentFailWindow bounds how long a single failure keeps hurting the
	// score. Past this, the node gets to compete on its current probe data
	// again rather than being punished forever for one past incident.
	autoRecentFailWindow = 5 * time.Minute
	// autoConsecFailCap stops the failure-count penalty from growing without
	// bound. Past 3 the node is already ranked last among reachable nodes;
	// letting the multiplier climb further would make a recovered node need
	// an implausibly long clean streak to ever climb back.
	autoConsecFailCap = 3
)

// scoreNode ranks a node; lower is better.
//
// Jitter is weighted double because a node that swings between 20ms and
// 120ms is worse to actually use than a steady 70ms one, even though its
// best sample looks better. Failure history dominates latency outright: no
// amount of speed makes a node that will not connect a good pick. classWeight
// comes from detectClassWeights and is 1.0 unless the phase-1 sweep shows
// plain-protocol nodes dying while obfuscated ones stay healthy.
func scoreNode(r AutoProbeResult, st NodeStat, isCurrent bool, classWeight float64, now time.Time) float64 {
	base := float64(r.RTTms) + 2*float64(r.JitterMs)
	score := base * classWeight

	fails := st.ConsecFails
	if fails > autoConsecFailCap {
		fails = autoConsecFailCap
	}
	score += autoConsecFailPenalty * float64(fails)

	if !st.LastFailAt.IsZero() && now.Sub(st.LastFailAt) < autoRecentFailWindow {
		score += autoRecentFailPenalty
	}
	if isCurrent {
		score -= autoTolerance
	}
	return score
}

// autoBlockedClassPenalty multiplies the score of plain-protocol nodes when the
// measurements say plain protocols are being cut right now.
const autoBlockedClassPenalty = 3.0

// autoClassMinSample is the smallest number of nodes in a class for its health
// ratio to mean anything. One dead node out of one is not evidence of anything.
const autoClassMinSample = 3

// nodeClass splits nodes into censorship-resistant and plain transports.
// Providers encode the same split in section headers ("when they throttle" /
// "when they don't"), but header text is unreliable — the protocol is not.
func nodeClass(e config.ProxyEntry) string {
	extra := map[string]any{}
	if len(e.Extra) > 0 {
		_ = json.Unmarshal(e.Extra, &extra)
	}
	security := strings.ToLower(getStringField(extra, "security", ""))
	network := strings.ToLower(getStringField(extra, "type", ""))

	// "obfs" arrives either as a bare string or as an object (SBHysteria2Obfs
	// has type/password), so test for a non-empty presence rather than a string
	// value — getStringField would return "" for the object form.
	hasObfs := false
	switch v := extra["obfs"].(type) {
	case string:
		hasObfs = strings.TrimSpace(v) != ""
	case map[string]any:
		hasObfs = len(v) > 0
	}

	if security == "reality" || hasObfs {
		return "obfs"
	}
	switch network {
	case "xhttp", "grpc":
		return "obfs"
	}
	return "plain"
}

// detectClassWeights infers whether plain protocols are currently being blocked
// by comparing per-class survival in the phase-1 sweep. Only measurement decides
// this — a class is penalised only when its own nodes are dying while the other
// class's are fine, and only when the sample is big enough to mean something.
func detectClassWeights(members []config.ProxyEntry, fast []AutoProbeResult) map[string]float64 {
	weights := map[string]float64{"plain": 1.0, "obfs": 1.0}

	okByKey := make(map[string]bool, len(fast))
	for _, r := range fast {
		okByKey[r.Key] = r.OK
	}

	total := map[string]int{}
	alive := map[string]int{}
	for _, m := range members {
		if !isProbeableNode(m) {
			continue
		}
		c := nodeClass(m)
		total[c]++
		if okByKey[AutoNodeKey(m)] {
			alive[c]++
		}
	}

	if total["plain"] < autoClassMinSample || total["obfs"] < 1 {
		return weights
	}
	plainRatio := float64(alive["plain"]) / float64(total["plain"])
	obfsRatio := float64(alive["obfs"]) / float64(total["obfs"])

	if plainRatio < 0.5 && obfsRatio >= 0.5 {
		weights["plain"] = autoBlockedClassPenalty
	}
	return weights
}

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
	// Score every candidate exactly once, into a parallel slice, before sorting.
	// nodeStats() is a live store that other goroutines mutate concurrently —
	// tray clicks each run on their own goroutine (see tray.go), and
	// connectFromTray -> RecordConnectOutcome -> NodeStatStore.RecordConnect
	// writes the very ConsecFails/LastFailAt fields scoreNode reads, for the
	// same keys a concurrent ranking could be scoring. A comparator that called
	// nodeStats().Get(...) and scoreNode(...) inline would re-read that
	// mutating store up to O(n log n) times per sort, so two comparisons of the
	// same element could legally disagree mid-sort, producing a quietly
	// self-inconsistent order for one ranking cycle. Scoring once up front
	// against a single now snapshot removes that inconsistency and does N
	// mutex-guarded store lookups instead of N*logN.
	now := time.Now()
	// Class weights come from the phase-1 sweep (the wide, cheap one) so a
	// handful of shortlist members never gets mistaken for the whole group's
	// health signal. Looked up once per candidate here, in the same pass that
	// computes score — not inside the comparator, which must stay a pure read
	// of precomputed values (see the comment above on why scores are
	// precomputed at all).
	weights := detectClassWeights(members, fast)
	classByKey := make(map[string]string, len(members))
	for _, m := range members {
		classByKey[AutoNodeKey(m)] = nodeClass(m)
	}
	type rankedCandidate struct {
		result AutoProbeResult
		score  float64
	}
	ranked := make([]rankedCandidate, len(scored))
	for i, r := range scored {
		ranked[i] = rankedCandidate{
			result: r,
			score:  scoreNode(r, nodeStats().Get(r.Key), r.Key == previousKey, weights[classByKey[r.Key]], now),
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score < ranked[j].score })

	out := make([]config.ProxyEntry, 0, AutoMaxCandidates)
	for _, r := range ranked {
		if len(out) >= AutoMaxCandidates {
			break
		}
		out = append(out, byKey[r.result.Key])
	}
	return out, phase1
}
