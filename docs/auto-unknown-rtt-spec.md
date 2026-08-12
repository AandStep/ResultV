# AUTO ranking prefers the node it could not measure

**Status:** fixed on `android`, open on `dev`.
**Audience:** whoever maintains AUTO selection on the desktop branch.
**Written:** 2026-08-12, from the android port of `autoprobe.go` / `autoselect.go` / `nodestats.go`.

## What goes wrong

A user with an AUTO group gets connected to the worst node in it — specifically, to whichever node the latency probe failed to time — and stays there. Reconnecting does not help: the choice is stable, because it is not random. The node that answers a probe but cannot be timed outranks every node that answered *and* reported a good figure.

It gets worse over time rather than better. The per-node statistics that exist to correct a bad pick record a negative rolling average for that node, so on the next launch it starts out looking even faster than it did before.

WireGuard and AmneziaWG groups are where this bites, because those transports are the ones whose latency routinely cannot be measured.

## Why

`PingProxyUDP` returns `-1, true, ""` when its UDP read times out, and again for any read error that is not an explicit refusal — `internal/proxy/engine.go:1350` and `:1357`.

**That `-1` is not a bug.** It is an intentional sentinel meaning *reachable, but we could not time it*, written for a UI that renders a negative number as a dash. The doc comment on `PingWireGuard` (`internal/proxy/engine.go:1370-1375`) says so: "returns -1ms → shown as '—'". Do not go fix the probe. The bug is that the ranking performs arithmetic on a value that was never a duration.

`PingWireGuard` reaches that sentinel whenever ICMP is unavailable — it tries `pingICMPHost` first and falls through to `PingProxyUDP` (`engine.go:1370-1375`).

Three consumers then treat it as a number:

| Site | What it does with `-1` |
|---|---|
| `internal/proxy/autoselect.go:74` | `base := float64(r.RTTms) + 2*float64(r.JitterMs)` → the node scores `-1` and beats a measured 20 ms node, whose score is 20. Lower is better. |
| `internal/proxy/autoselect.go:256` | `sort.SliceStable(alive, …RTTms < …RTTms)` → it sorts first into the phase-2 shortlist. |
| `internal/proxy/nodestats.go:125-138` | `RecordProbe` blends it into `EWMARTTms`, which is persisted to `node_stats.json` and survives the process. |

Nothing downstream repairs it. WireGuard and AmneziaWG never reach the phase-2 TLS stage — `autoProbeTLSParams` returns `wantTLS=false` for them — so the sentinel is still in the result when `scoreNode` runs.

`app.go:2705` also formats `r.RTTms` directly into the AUTO diagnostic table, so the row for such a node reads `-1ms`.

## How `dev` differs from `android`

Same defect, same code, different frequency.

`android` added a gate that forbids the keyed WireGuard handshake probe while our own tunnel is up, because the keyed probe completes a real handshake and the server may treat it as a session reset. That gate was originally keyed off the UI-facing "tunnel active" flag, which `ResultVpnService.kt` sets to true before the coroutine that resolves an AUTO group even starts — so the gate was closed for every AUTO resolve, including the very first connect before any tunnel existed, not only a re-rank after a node failure as the phrasing here used to suggest. WireGuard nodes therefore took the keyless path — which is `PingWireGuard`, which is the sentinel path — on essentially every resolve. (A later fix rebased the gate on whether the engine is actually running rather than that UI flag; it corrects which resolves are gated, but does not change the point made here — on `android`, this sentinel path was the common case for WireGuard/AmneziaWG groups, not an edge case.)

`dev` has no such gate. It reaches the same sentinel through `PingWireGuard`'s ICMP fallback: any environment where ICMP is blocked or unprivileged ICMP is unavailable. The consequence is identical; it just fires less predictably.

## The fix

Carry "measured" and "not measured" as distinct states instead of encoding the second as a negative number. Five edits, all in `internal/proxy`.

**1. `autoprobe.go`, `AutoProbeResult` (`:52`)** — add a field:

```go
	// RTTKnown distinguishes "reachable, and this is how long it took" from
	// "reachable, but we could not time it". PingProxyUDP reports the second
	// case as RTTms = -1 (engine.go:1350), a sentinel meant for a UI that
	// renders it as a dash — arithmetic on it makes the least-known node the
	// best-scoring one.
	RTTKnown bool
```

**2. `autoprobe.go`, `probeOne`** — set it from the transport probe, right after the existing assignment of `RTTms`/`OK`/`Stage`/`Reason`:

```go
	res.RTTKnown = ok && rtt >= 0
```

and in the TLS branch, next to `res.RTTms, res.JitterMs = medianAndJitter(samples)`, add `res.RTTKnown = true` — a completed handshake is always a real measurement.

**3. `autoselect.go`, above `scoreNode`** — a stand-in value and the single accessor every comparison goes through:

```go
// autoUnknownRTTms stands in for a node that answered but could not be timed
// (AutoProbeResult.RTTKnown false). It has to sit above any latency we would
// actually be happy with and below the point where a node is unusable: the
// node is not known to be slow, but preferring it over one we measured and
// liked would be preferring ignorance. 500ms is past every RTT a node in this
// client's rotation realistically shows, and far below the failure penalties,
// so an unmeasured node still outranks one that will not connect.
const autoUnknownRTTms = 500.0

// effectiveRTTms is the latency the ranking should reason about — the measured
// one, or the stand-in when there is none. Every comparison of RTTms goes
// through here; comparing the raw field is what let the -1 sentinel win.
func effectiveRTTms(r AutoProbeResult) float64 {
	if !r.RTTKnown {
		return autoUnknownRTTms
	}
	return float64(r.RTTms)
}
```

**4. `autoselect.go:74` and `:256`** — route both comparisons through it:

```go
	base := effectiveRTTms(r) + 2*float64(r.JitterMs)
```
```go
	sort.SliceStable(alive, func(i, j int) bool { return effectiveRTTms(alive[i]) < effectiveRTTms(alive[j]) })
```

**5. `nodestats.go`, `RecordProbe` (`:131`)** — refuse the sentinel at the store, not only at the call site, because this file is what persists:

```go
	if ok && rttMs >= 0 {
		… existing EWMA update …
	} else if !ok {
		st.LastFailAt = time.Now()
	}
```

Note the shape: an untimed-but-reachable sample must fall through **both** branches and update only `LastReason`. It is not a failure, so it must not set `LastFailAt`. The naive edit — guarding the `if ok` and leaving a bare `else` — records every unmeasurable node as failing, which is a different bug with the same symptom.

**6. `app.go:2705`** — the AUTO diagnostic row currently prints `fmt.Sprintf("%dms", r.RTTms)`. Render the unknown case as a dash rather than `-1ms`, matching what the sentinel was designed for in the first place.

## Tests

Three, and they fail before the change:

```go
func TestProbeOneMarksUnknownRTT(t *testing.T) {
	prev := pingTCPProbe
	pingTCPProbe = func(ip string, port int) (int64, bool, string) { return -1, true, "" }
	t.Cleanup(func() { pingTCPProbe = prev })

	got := probeOne(config.ProxyEntry{IP: "203.0.113.9", Port: 443, Type: "VLESS"}, DepthFast)
	if !got.OK {
		t.Fatalf("an unknown-latency probe is still a reachable node: %+v", got)
	}
	if got.RTTKnown {
		t.Fatalf("RTTKnown must be false when the probe reported no latency: %+v", got)
	}
}

func TestScoreNodeRanksUnknownRTTBelowMeasured(t *testing.T) {
	now := time.Now()
	unknown := scoreNode(AutoProbeResult{Key: "u", OK: true, RTTms: -1, RTTKnown: false}, NodeStat{}, false, 1.0, now)
	measured := scoreNode(AutoProbeResult{Key: "m", OK: true, RTTms: 200, RTTKnown: true}, NodeStat{}, false, 1.0, now)
	if !(measured < unknown) {
		t.Fatalf("a measured 200ms node must beat an unmeasured one: measured=%v unknown=%v", measured, unknown)
	}
}

func TestRecordProbeIgnoresUnknownLatency(t *testing.T) {
	s := NewNodeStatStore(t.TempDir())
	s.RecordProbe("k", 40, 5, true, "")
	s.RecordProbe("k", -1, 0, true, "")
	if got := s.Get("k").EWMARTTms; got != 40 {
		t.Fatalf("an unknown-latency sample must leave the average untouched, got %v", got)
	}
}
```

Worth adding a fourth, asserting the other side of the policy — that an unmeasured node still outranks one with recorded connect failures — so a later change to `autoUnknownRTTms` cannot quietly push it past the failure penalties:

```go
func TestScoreNodeRanksUnknownRTTAboveFailingNode(t *testing.T) {
	now := time.Now()
	unknown := scoreNode(AutoProbeResult{Key: "u", OK: true, RTTms: -1, RTTKnown: false}, NodeStat{}, false, 1.0, now)
	failing := scoreNode(
		AutoProbeResult{Key: "f", OK: true, RTTms: 20, RTTKnown: true},
		NodeStat{ConsecFails: 2, LastFailAt: now.Add(-time.Minute)}, false, 1.0, now)
	if !(unknown < failing) {
		t.Fatalf("an unmeasured node must beat one that will not connect: unknown=%v failing=%v", unknown, failing)
	}
}
```

## What android did, for diffing

One commit on the `android` branch: `fix(auto): stop the unknown-latency sentinel from winning the ranking`. It contains the five Go edits above plus two things `dev` does not need in the same form — the flag is carried across the gomobile boundary as a `rttKnown` field on the candidate JSON (`mobile/libbox_auto.go`), and the Kotlin log line renders a dash instead of `-1 ms` (`AutoSelection.kt`, `ResultVpnService.kt`, both `strings.xml`). `dev`'s equivalent of that last part is item 6 above.

The android branch reached this through a port of `dev`'s own `autoprobe.go` / `autoselect.go` / `nodestats.go`, so the three Go files are otherwise the same code on both sides and the diff should apply cleanly.
