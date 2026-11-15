package proxy

import (
	"context"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"

	"resultproxy-wails/internal/verdict"
)

type learnRig struct {
	s      *smartOutbound
	clock  time.Time
	probes atomic.Int32
}

func newLearnRig(t *testing.T, answer verdict.Decision) *learnRig {
	t.Helper()
	r := &learnRig{clock: time.Now()}
	r.s = &smartOutbound{
		store:  choiceStore(t),
		health: newDirectHealth(nil),
		probes: newProbeGate(nil),
		now:    func() time.Time { return r.clock },
		probe: func(ctx context.Context, host string, gate *probeGate) verdict.Decision {
			r.probes.Add(1)
			return answer
		},
	}
	return r
}

func (r *learnRig) settle(t *testing.T, host string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for r.probes.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the probe never ran")
		}
		time.Sleep(2 * time.Millisecond)
	}
	waitNotInflight(t, r.s, host)
}

var nodeWon = raceResult{ViaProxy: true}
var directWon = raceResult{}

func TestNodeWinningOneRaceWritesNoVerdict(t *testing.T) {
	r := newLearnRig(t, verdict.Unknown)
	r.s.noteRace(udpMetadata("slow.example", 443), nodeWon)
	r.settle(t, "slow.example")
	if rec, ok := r.s.store.Lookup("slow.example"); ok {
		t.Fatalf("one lost race became a verdict: %+v", rec)
	}
}

func TestNodeWinIsConfirmedByTheProbe(t *testing.T) {
	r := newLearnRig(t, verdict.Proxy)
	r.s.noteRace(udpMetadata("blocked.example", 443), nodeWon)
	r.settle(t, "blocked.example")
	rec, ok := r.s.store.Lookup("blocked.example")
	if !ok || rec.Decision != verdict.Proxy || rec.Unverified {
		t.Fatalf("got %+v, ok=%v", rec, ok)
	}
}

func TestNodeWinIsOverruledByTheProbe(t *testing.T) {
	r := newLearnRig(t, verdict.Direct)
	r.s.noteRace(udpMetadata("fine.example", 443), nodeWon)
	r.settle(t, "fine.example")
	rec, ok := r.s.store.Lookup("fine.example")
	if !ok || rec.Decision != verdict.Direct {
		t.Fatalf("got %+v, ok=%v", rec, ok)
	}
}

func TestRepeatedSpacedLossesBecomeAnUnverifiedProxy(t *testing.T) {
	r := newLearnRig(t, verdict.Unknown)
	meta := udpMetadata("mute.example", 443)
	for i := 0; i < raceLossesToLearn; i++ {
		r.s.noteRace(meta, nodeWon)
		r.clock = r.clock.Add(raceLossSpacing + time.Second)
	}
	rec, ok := r.s.store.Lookup("mute.example")
	if !ok || rec.Decision != verdict.Proxy || !rec.Unverified {
		t.Fatalf("got %+v, ok=%v", rec, ok)
	}
}

func TestABurstOfLossesCountsOnce(t *testing.T) {
	r := newLearnRig(t, verdict.Unknown)
	meta := udpMetadata("burst.example", 443)
	for i := 0; i < raceLossesToLearn*4; i++ {
		r.s.noteRace(meta, nodeWon)
	}
	if rec, ok := r.s.store.Lookup("burst.example"); ok {
		t.Fatalf("simultaneous losses became a verdict: %+v", rec)
	}
}

func TestDirectWinResetsTheLossCount(t *testing.T) {
	r := newLearnRig(t, verdict.Unknown)
	meta := udpMetadata("flaky.example", 443)
	for i := 0; i < raceLossesToLearn-1; i++ {
		r.s.noteRace(meta, nodeWon)
		r.clock = r.clock.Add(raceLossSpacing + time.Second)
	}
	r.s.noteRace(meta, directWon)
	r.s.store.Forget("flaky.example")
	r.s.noteRace(meta, nodeWon)
	if rec, ok := r.s.store.Lookup("flaky.example"); ok {
		t.Fatalf("losses before a direct win still counted: %+v", rec)
	}
}

func TestBareAddressLearnsFromSpacedLossesOnly(t *testing.T) {
	r := newLearnRig(t, verdict.Proxy)
	addr := netip.MustParseAddr("203.0.113.9")
	meta := adapter.InboundContext{Destination: M.SocksaddrFrom(addr, 443)}
	r.s.noteRace(meta, nodeWon)
	if _, ok := r.s.store.LookupIP(addr); ok {
		t.Fatal("one lost race on a bare address became a verdict")
	}
	for i := 1; i < raceLossesToLearn; i++ {
		r.clock = r.clock.Add(raceLossSpacing + time.Second)
		r.s.noteRace(meta, nodeWon)
	}
	rec, ok := r.s.store.LookupIP(addr)
	if !ok || rec.Decision != verdict.Proxy || !rec.Unverified {
		t.Fatalf("got %+v, ok=%v", rec, ok)
	}
	if r.probes.Load() != 0 {
		t.Fatal("a bare address cannot be probed")
	}
}

func TestUnprovenRaceTeachesNothing(t *testing.T) {
	r := newLearnRig(t, verdict.Proxy)
	r.s.noteRace(udpMetadata("handover.example", 443), raceResult{ViaProxy: true, Unproven: true})
	time.Sleep(20 * time.Millisecond)
	if r.probes.Load() != 0 {
		t.Fatal("a handover started a probe")
	}
	if _, ok := r.s.store.Lookup("handover.example"); ok {
		t.Fatal("a handover wrote a verdict")
	}
}
