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
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
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

// autoSweepCacheTTL is how long a completed ranking stays reusable. Long enough
// that toggling AUTO off and on does not re-pay for a full sweep, short enough
// that a node dying mid-session is noticed on the next attempt.
const autoSweepCacheTTL = 90 * time.Second

type autoSweepCacheEntry struct {
	at         time.Time
	candidates []config.ProxyEntry
	diag       AutoProbeDiagnostics
}

// autoSweepCache is a map, not a single slot: a user with more than one AUTO
// group alternately selected would otherwise produce two different keys that
// evict each other on every call, giving that user a 0% hit rate and defeating
// the point of this cache entirely.
var (
	autoSweepMu    sync.Mutex
	autoSweepCache = map[string]autoSweepCacheEntry{}
)

// autoSweepCacheKey folds everything that could change the answer into one
// string: the group's membership, the node currently in use (it gets the
// tolerance head start, so it changes the order), and the physical bind address.
//
// The bind address folds most network changes into the key: roam to another
// Wi-Fi or drop to tethering and pickLANBindIPv4 eventually returns a
// different address, so the key changes and the next call re-sweeps instead
// of serving measurements from a link that no longer exists. This is NOT
// instantaneous, though — pickLANBindIPv4 is cachedPreferLANBindIPv4
// (ping_lan_bind.go), which memoizes the bind IP for up to lanBindCacheTTL
// (30s) with no invalidation hook wired to network-change events
// (invalidateLANBindCache exists but nothing calls it repo-wide). So for up
// to ~30s after roaming this key can still compute the OLD bind address, on
// top of the up-to-90s autoSweepCacheTTL itself holds a ranking once cached.
// A real connect failure on the current pick busts the whole cache
// separately (see RecordConnectOutcome in nodestats.go), which is the other
// half of "stale measurements do not linger" — this key alone does not cover
// every staleness path, only the network-identity one.
func autoSweepCacheKey(members []config.ProxyEntry, previousKey string) string {
	keys := make([]string, 0, len(members))
	for _, m := range members {
		if isProbeableNode(m) {
			keys = append(keys, AutoNodeKey(m))
		}
	}
	sort.Strings(keys)

	bind := "none"
	if ip, err := pickLANBindIPv4(); err == nil && ip != nil {
		bind = ip.String()
	}

	h := sha1.New()
	write := func(s string) { fmt.Fprintf(h, "%d:%s", len(s), s) }
	write(bind)
	write(previousKey)
	for _, k := range keys {
		write(k)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ResetAutoSweepCache drops every cached ranking, for every group. Tests use
// it for isolation between fixtures. Production calls it too now:
// RecordConnectOutcome (nodestats.go) calls it whenever a real connect fails,
// because a failed connect changes ConsecFails/LastFailAt — fields scoreNode
// weighs heavily — without changing anything in the cache key, so without
// this call a node that just failed to connect would keep its cached top
// rank for up to autoSweepCacheTTL.
func ResetAutoSweepCache() {
	autoSweepMu.Lock()
	autoSweepCache = map[string]autoSweepCacheEntry{}
	autoSweepMu.Unlock()
}

func lookupAutoSweepCache(key string) ([]config.ProxyEntry, AutoProbeDiagnostics, bool) {
	autoSweepMu.Lock()
	defer autoSweepMu.Unlock()
	entry, ok := autoSweepCache[key]
	if !ok || entry.at.IsZero() {
		return nil, AutoProbeDiagnostics{}, false
	}
	if time.Since(entry.at) >= autoSweepCacheTTL {
		return nil, AutoProbeDiagnostics{}, false
	}
	// Copy candidates AND both diagnostic slices before handing them out.
	// app.go's ResolveAutoCandidates keeps the returned slice for the AUTO
	// group's lifetime (a.autoCandidates[head.ID] = ranked), and a second
	// call — a different group hitting a different map entry, or the same
	// group hit again — must get its own independent backing array, or two
	// unrelated owners (two cache hits, possibly for two different groups)
	// would share one array underneath both of them: a future in-place edit
	// through one owner's slice (or Task 9 sorting diag for a table) would
	// silently corrupt the other's data and the cache's own stored rows.
	out := append([]config.ProxyEntry(nil), entry.candidates...)
	diag := entry.diag
	diag.Phase1 = append([]AutoProbeResult(nil), diag.Phase1...)
	diag.Phase2 = append([]AutoProbeResult(nil), diag.Phase2...)
	diag.FromCache = true
	return out, diag, true
}

func storeAutoSweepCache(key string, candidates []config.ProxyEntry, diag AutoProbeDiagnostics) {
	// A sweep that found nothing is not a fact worth keeping: "nobody answered"
	// is exactly the state the next click must re-check rather than inherit.
	if len(candidates) == 0 {
		return
	}
	now := time.Now()
	autoSweepMu.Lock()
	defer autoSweepMu.Unlock()
	// Prune expired entries on every store, not just this key's own slot: a
	// user who keeps toggling between AUTO groups (or refreshes a
	// subscription that changes group membership) would otherwise grow this
	// map without bound over a long session, since nothing else ever removes
	// a stale entry for a key that is no longer being asked for.
	for k, e := range autoSweepCache {
		if now.Sub(e.at) >= autoSweepCacheTTL {
			delete(autoSweepCache, k)
		}
	}
	autoSweepCache[key] = autoSweepCacheEntry{
		at:         now,
		candidates: append([]config.ProxyEntry(nil), candidates...),
		diag:       diag,
	}
}

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
	// The transport is stored under the "network" key, not "type" — "type" is
	// only the URI *query parameter* name (see parseVLESSURI); every consumer
	// (outbound.go, the JSON-subscription parser) reads it back as "network".
	network := strings.ToLower(getStringField(extra, "network", ""))

	// Hysteria2 obfuscation is always a flat string under "obfs_type"
	// (parseHysteria2URI writes it, outbound.go reads it the same way, with
	// "obfsType" as the camelCase alias some producers use) — there is no
	// object-shaped "obfs" key anywhere a real node populates.
	obfsType := getStringField(extra, "obfs_type", getStringField(extra, "obfsType", ""))

	// pbk mirrors outbound.go's own Reality auto-detection: the engine builds
	// a node as Reality whenever a public key is present, even without an
	// explicit security=reality, so nodeClass must agree or such a node
	// classifies as plain. Deliberately NOT falling back to the
	// publicKey/public_key aliases outbound.go also checks there: those are
	// WireGuard's own key field, and every VLESS/VMESS Reality producer in
	// this repo writes the key under "pbk" specifically — falling back would
	// misclassify every WireGuard node as obfs.
	pbk := getStringField(extra, "pbk", "")

	if security == "reality" || obfsType != "" || pbk != "" {
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

// AutoProbeDiagnostics carries both probe phases back to the caller's
// diagnostic table. Phase1 is the wide, cheap sweep (TCP/UDP/QUIC connect,
// every member); Phase2 is the narrow, accurate one (TLS handshake with real
// SNI/ALPN, shortlist only) — the only phase that can see SNI-based
// blocking. A member phase 2 re-probed can look healthy in Phase1 and still
// be missing from the final `candidates`, so callers building a per-member
// table need both, not just Phase1, or that member silently shows as a
// healthy "NNms" row instead of the failure it actually is.
type AutoProbeDiagnostics struct {
	Phase1 []AutoProbeResult
	Phase2 []AutoProbeResult

	// FromCache marks a result served from the sweep cache rather than measured
	// on this call. Callers that log timings need to say so, or a 0ms sweep
	// reads as a broken measurement.
	FromCache bool
}

// dropEmptyKeyResults removes probe slots ProbeAutoNodes left at their zero
// value because ctx was cancelled before a worker reached them (see
// ProbeAutoNodes: a cancelled slot is never assigned, so it keeps out[i]'s
// zero value, whose Key is ""). AutoNodeKey never returns "" for an actual
// probe, so an empty Key unambiguously marks a cancelled slot, not a real
// result. Recording one would write a permanent junk entry to
// node_stats.json under the empty key, and returning it in the diagnostics
// would show a meaningless "— :0" row in the table — every OK on such a
// slot is already false, so filtering them out never changes which nodes are
// considered alive.
func dropEmptyKeyResults(in []AutoProbeResult) []AutoProbeResult {
	out := in[:0]
	for _, r := range in {
		if r.Key == "" {
			continue
		}
		out = append(out, r)
	}
	return out
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
// AUTO head. diag carries the raw probe rows from both phases and is returned
// on EVERY exit path (including the nil-candidates ones) instead of being
// stashed in package state: this function is called concurrently from
// independent callers (tray clicks each run on their own goroutine — see
// tray.go), and a shared package-level "last snapshot" would let one caller's
// diagnostic table silently show another caller's rows. Returning diag as an
// ordinary value ties it to the call that produced it, so there is nothing
// left to race.
func RankAutoCandidates(ctx context.Context, members []config.ProxyEntry, previousKey string) (candidates []config.ProxyEntry, diag AutoProbeDiagnostics) {
	cacheKey := autoSweepCacheKey(members, previousKey)
	if cached, cachedDiag, ok := lookupAutoSweepCache(cacheKey); ok {
		return cached, cachedDiag
	}

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
	fast = dropEmptyKeyResults(fast)
	diag.Phase1 = fast
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
		return nil, diag
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
	full = dropEmptyKeyResults(full)
	diag.Phase2 = full
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
		return nil, diag
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
	storeAutoSweepCache(cacheKey, out, diag)
	return out, diag
}
