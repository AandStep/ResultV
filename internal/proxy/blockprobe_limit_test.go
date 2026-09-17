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
	"testing"
	"time"
)

func TestGateCapsProbesPerMinute(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	g := newProbeGate(func() time.Time { return now })

	allowed := 0
	for i := 0; i < probeMaxPerMinute*3; i++ {
		if g.allow("host" + string(rune('a'+i%26)) + string(rune('a'+i/26))) {
			allowed++
		}
	}
	if allowed != probeMaxPerMinute {
		t.Fatalf("gate let through %d probes, cap is %d", allowed, probeMaxPerMinute)
	}

	now = now.Add(time.Minute + time.Second)
	if !g.allow("fresh.example") {
		t.Fatal("the budget must refill after a minute")
	}
}

// A host that just failed is not asked again immediately. Backoff doubles, so
// a permanently dead name costs a handful of probes, not a probe per visit.
func TestGateBacksOffPerHost(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	g := newProbeGate(func() time.Time { return now })

	if !g.allow("flaky.example") {
		t.Fatal("first probe must be allowed")
	}
	g.record("flaky.example", false)
	if g.allow("flaky.example") {
		t.Fatal("a just-failed host must not be probed again at once")
	}

	now = now.Add(probeBackoffBase + time.Second)
	if !g.allow("flaky.example") {
		t.Fatal("backoff must expire")
	}
	g.record("flaky.example", false)

	now = now.Add(probeBackoffBase + time.Second)
	if g.allow("flaky.example") {
		t.Fatal("the second failure must double the backoff")
	}
}

func TestGateSuccessClearsBackoff(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	g := newProbeGate(func() time.Time { return now })

	g.allow("good.example")
	g.record("good.example", false)
	now = now.Add(probeBackoffBase + time.Second)
	g.allow("good.example")
	g.record("good.example", true)

	if !g.allow("good.example") {
		t.Fatal("a success must clear the per-host penalty")
	}
}

// The proven failure mode: a storm of retries got the source banned upstream,
// and it looked like the app breaking by itself. The breaker is what stops the
// prober being that storm.
func TestBreakerOpensAfterSustainedFailureAndCoolsDown(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	g := newProbeGate(func() time.Time { return now })

	for i := 0; i < probeBreakerFailures; i++ {
		g.record("h"+string(rune('a'+i)), false)
	}
	if g.open() {
		t.Fatal("the breaker must be tripped after a run of failures")
	}
	if g.allow("anything.example") {
		t.Fatal("a tripped breaker must refuse every probe")
	}

	now = now.Add(probeBreakerCooldown + time.Second)
	if !g.open() {
		t.Fatal("the breaker must close again after the cooldown")
	}
}
