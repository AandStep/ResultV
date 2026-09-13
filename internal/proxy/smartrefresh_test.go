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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"resultproxy-wails/internal/verdict"
)

// A learned verdict used to simply expire, and the next request for that name
// paid for a race it had already paid for once. A day is the whole life of a
// Direct verdict, so on a name the user visits daily that was one slow load
// every single day, forever.
func TestVerdictNeedsRefreshNearTheEndOfItsLife(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

	learned := func(d verdict.Decision, remaining time.Duration) verdict.Record {
		return verdict.Record{
			Decision:  d,
			Source:    verdict.SourceLearned,
			Observed:  now,
			ExpiresAt: now.Add(remaining),
		}
	}

	// TTLDirect is a day; the last eighth of it is the last three hours.
	if !verdictNeedsRefresh(learned(verdict.Direct, time.Hour), now) {
		t.Error("a Direct verdict with an hour left was not renewed")
	}
	if verdictNeedsRefresh(learned(verdict.Direct, 12*time.Hour), now) {
		t.Error("a Direct verdict with half its life left was renewed anyway")
	}
	// TTLProxy is a week, so its last eighth is the last twenty-one hours.
	if !verdictNeedsRefresh(learned(verdict.Proxy, 10*time.Hour), now) {
		t.Error("a Proxy verdict with ten hours left was not renewed")
	}
	if verdictNeedsRefresh(learned(verdict.Proxy, 3*24*time.Hour), now) {
		t.Error("a Proxy verdict with three days left was renewed anyway")
	}
}

// Records that never expire must never be probed: a user rule is not a guess
// to be second-guessed, and re-deriving a floor entry would put the data plane
// above the hand-proven answer the source order exists to protect.
func TestVerdictThatNeverExpiresIsNeverRefreshed(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	for _, src := range []verdict.Source{verdict.SourceUser, verdict.SourceFloor, verdict.SourceList} {
		rec := verdict.Record{Decision: verdict.Proxy, Source: src, Observed: now}
		if verdictNeedsRefresh(rec, now) {
			t.Errorf("a record from source %d was scheduled for a probe", src)
		}
	}
}

// refreshOutbound is a smart outbound whose prober is a counter instead of the
// network.
func refreshOutbound(t *testing.T, answer verdict.Decision) (*smartOutbound, *atomic.Int32, *sync.WaitGroup) {
	t.Helper()
	var calls atomic.Int32
	var wg sync.WaitGroup
	s := &smartOutbound{
		store:  choiceStore(t),
		health: newDirectHealth(nil),
		probes: newProbeGate(nil),
		probe: func(ctx context.Context, host string, gate *probeGate) verdict.Decision {
			calls.Add(1)
			wg.Done()
			return answer
		},
	}
	return s, &calls, &wg
}

// The point of renewing early is that the user never waits for it: the record
// still in force answers this connection, and the probe replaces it behind the
// scenes.
func TestExpiringVerdictIsRenewedInTheBackground(t *testing.T) {
	s, calls, wg := refreshOutbound(t, verdict.Proxy)
	rec := verdict.Record{
		Decision:  verdict.Direct,
		Source:    verdict.SourceLearned,
		Observed:  time.Now().Add(-23 * time.Hour),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	wg.Add(1)
	s.refreshExpiringAsync("expiring.example", rec)
	waitForProbes(t, wg)

	if calls.Load() != 1 {
		t.Fatalf("probes fired: %d, want 1", calls.Load())
	}
	// The stub reports it was called before the caller writes the answer back,
	// so the write is the thing to wait for.
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, ok := s.store.Lookup("expiring.example")
		if ok && got.Decision == verdict.Proxy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the renewed verdict was not written back: %+v (found=%v)", got, ok)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestFreshVerdictIsLeftAlone(t *testing.T) {
	s, calls, _ := refreshOutbound(t, verdict.Proxy)
	rec := verdict.Record{
		Decision:  verdict.Direct,
		Source:    verdict.SourceLearned,
		Observed:  time.Now(),
		ExpiresAt: time.Now().Add(20 * time.Hour),
	}

	s.refreshExpiringAsync("fresh.example", rec)
	time.Sleep(50 * time.Millisecond)

	if calls.Load() != 0 {
		t.Fatalf("a verdict with most of its life left was probed %d times", calls.Load())
	}
}

// A page load opens dozens of connections to the same host at once. Without a
// guard each one would start its own probe, and the twenty-per-minute budget —
// the thing standing between this feature and the retry storm that once got a
// source banned — would be gone on a single page.
func TestConcurrentRechecksOfOneHostProbeOnce(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	s := &smartOutbound{
		store:  choiceStore(t),
		health: newDirectHealth(nil),
		probes: newProbeGate(nil),
		probe: func(ctx context.Context, host string, gate *probeGate) verdict.Decision {
			calls.Add(1)
			wg.Done()
			<-release
			return verdict.Direct
		},
	}

	for i := 0; i < 20; i++ {
		s.recheckAsync("busy.example")
	}
	waitForProbes(t, &wg)
	got := calls.Load()
	close(release)

	if got != 1 {
		t.Fatalf("twenty connections to one host started %d probes, want 1", got)
	}
}

// And once the probe is done the host is probeable again — the guard is a
// dedup, not a one-shot.
func TestRecheckIsPossibleAgainAfterTheProbeFinishes(t *testing.T) {
	s, calls, wg := refreshOutbound(t, verdict.Direct)

	wg.Add(1)
	s.recheckAsync("twice.example")
	waitForProbes(t, wg)
	// The guard is released when the probe goroutine returns, which is after
	// the stub has reported itself — so that, not the report, is what the
	// second attempt has to wait for.
	waitNotInflight(t, s, "twice.example")

	wg.Add(1)
	s.recheckAsync("twice.example")
	waitForProbes(t, wg)

	if calls.Load() != 2 {
		t.Fatalf("the second probe never ran: %d calls", calls.Load())
	}
}

func waitForProbes(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the probe never ran")
	}
}

// The trigger has to sit on the path a connection actually takes, and it has
// to hand back the verdict still in force: the connection that noticed the
// record was ageing must be served by it, not made to wait for the renewal.
func TestConnectionPathRenewsAnAgeingVerdictAndStillUsesIt(t *testing.T) {
	var calls atomic.Int32
	var wg sync.WaitGroup
	wg.Add(1)

	// A store whose clock sits 23 hours in the past: what it learns now is
	// live for it, and an hour from expiry by the wall clock the trigger uses.
	store := verdict.New([]byte("refresh-salt"), func() time.Time {
		return time.Now().Add(-23 * time.Hour)
	})
	store.SetNamespace("test")
	store.Learn("ageing.example", verdict.Direct)

	s := &smartOutbound{
		store:  store,
		health: newDirectHealth(nil),
		probes: newProbeGate(nil),
		probe: func(ctx context.Context, host string, gate *probeGate) verdict.Decision {
			calls.Add(1)
			wg.Done()
			return verdict.Proxy
		},
	}

	meta := udpMetadata("ageing.example", 443)
	rec, known := s.lookupAndRefresh(&meta)

	if !known || rec.Decision != verdict.Direct {
		t.Fatalf("the record in force was not returned: %+v (found=%v)", rec, known)
	}
	waitForProbes(t, &wg)
	if calls.Load() != 1 {
		t.Fatalf("probes fired: %d, want 1", calls.Load())
	}
}

func waitNotInflight(t *testing.T, s *smartOutbound, host string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, busy := s.inflight.Load(host); !busy {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never left the in-flight set", host)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
