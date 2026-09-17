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
	"errors"
	"testing"
	"time"

	"resultproxy-wails/internal/verdict"
)

func withStubbedProbeFetch(t *testing.T, fn func(ctx context.Context, url string, viaNode bool) probeOutcome) {
	t.Helper()
	prev := probeFetch
	probeFetch = fn
	t.Cleanup(func() { probeFetch = prev })
}

func testGate() *probeGate {
	return newProbeGate(func() time.Time { return time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC) })
}

// The case the race gets wrong: a healthy TCP connection that answers with a
// region wall. Only a probe that reads the answer can tell them apart.
func TestProbeHostCatchesAGeoBlockTheRaceWouldMiss(t *testing.T) {
	withStubbedProbeFetch(t, func(_ context.Context, _ string, viaNode bool) probeOutcome {
		if viaNode {
			return probeOutcome{Status: 200, Bytes: 4096, Elapsed: 300 * time.Millisecond}
		}
		return probeOutcome{Status: 302, Location: "https://claude.com/app-unavailable-in-region", Elapsed: 407 * time.Millisecond}
	})
	if got := probeHost(context.Background(), "claude.ai", testGate()); got != verdict.Proxy {
		t.Fatalf("got %v, want proxy", got)
	}
}

func TestProbeHostLeavesAHealthySiteAlone(t *testing.T) {
	withStubbedProbeFetch(t, func(_ context.Context, _ string, _ bool) probeOutcome {
		return probeOutcome{Status: 200, Bytes: 4096, Elapsed: 200 * time.Millisecond}
	})
	if got := probeHost(context.Background(), "example.com", testGate()); got != verdict.Direct {
		t.Fatalf("got %v, want direct", got)
	}
}

// The muzzle is not optional: a probe storm is how this project already got a
// source banned upstream once.
func TestProbeHostObeysTheGate(t *testing.T) {
	calls := 0
	withStubbedProbeFetch(t, func(_ context.Context, _ string, _ bool) probeOutcome {
		calls++
		return probeOutcome{Err: errors.New("i/o timeout")}
	})
	gate := testGate()
	for i := 0; i < probeMaxPerMinute*2; i++ {
		probeHost(context.Background(), "flaky.example", gate)
	}
	// Two fetches per admitted probe, and the per-host backoff admits far fewer
	// than the loop asks for.
	if calls > probeMaxPerMinute*2 {
		t.Fatalf("the gate let %d fetches through", calls)
	}
	if got := probeHost(context.Background(), "flaky.example", gate); got != verdict.Unknown {
		t.Fatalf("a refused probe must teach nothing, got %v", got)
	}
}

// A failed probe is evidence about the probe, not about the site. Recording it
// would be the same mistake as recording a race run on a dead link.
func TestProbeHostTeachesNothingWhenTheNodeIsDown(t *testing.T) {
	withStubbedProbeFetch(t, func(_ context.Context, _ string, viaNode bool) probeOutcome {
		if viaNode {
			return probeOutcome{Err: errors.New("connection reset by peer")}
		}
		return probeOutcome{Status: 200, Bytes: 4096}
	})
	if got := probeHost(context.Background(), "example.com", testGate()); got != verdict.Unknown {
		t.Fatalf("got %v, want unknown", got)
	}
}
