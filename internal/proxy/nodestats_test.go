package proxy

import (
	"math"
	"path/filepath"
	"testing"
)

func TestNodeStatStore_EWMAConvergesTowardNewSamples(t *testing.T) {
	s := NewNodeStatStore(t.TempDir())

	s.RecordProbe("k", 100, 0, true, "")
	if got := s.Get("k").EWMARTTms; got != 100 {
		t.Fatalf("первый замер должен задавать значение целиком, получили %v", got)
	}

	s.RecordProbe("k", 200, 0, true, "")
	want := 100*(1-nodeStatAlpha) + 200*nodeStatAlpha
	if got := s.Get("k").EWMARTTms; math.Abs(got-want) > 0.001 {
		t.Errorf("ожидали EWMA %v, получили %v", want, got)
	}
}

func TestNodeStatStore_ConsecFailsResetsOnSuccess(t *testing.T) {
	s := NewNodeStatStore(t.TempDir())

	s.RecordConnect("k", false, "timeout")
	s.RecordConnect("k", false, "timeout")
	if got := s.Get("k").ConsecFails; got != 2 {
		t.Fatalf("ожидали 2 подряд отказа, получили %d", got)
	}
	if got := s.Get("k").ConnectFail; got != 2 {
		t.Errorf("ожидали ConnectFail=2, получили %d", got)
	}

	s.RecordConnect("k", true, "")
	st := s.Get("k")
	if st.ConsecFails != 0 {
		t.Errorf("успех должен обнулять серию отказов, получили %d", st.ConsecFails)
	}
	if st.ConnectOK != 1 {
		t.Errorf("ожидали ConnectOK=1, получили %d", st.ConnectOK)
	}
	if st.LastSuccessAt.IsZero() {
		t.Error("успех должен проставлять LastSuccessAt")
	}
}

func TestNodeStatStore_FailedProbeDoesNotPoisonRTT(t *testing.T) {
	s := NewNodeStatStore(t.TempDir())

	s.RecordProbe("k", 50, 5, true, "")
	s.RecordProbe("k", 0, 0, false, "timeout")

	st := s.Get("k")
	if st.EWMARTTms != 50 {
		t.Errorf("неудачная проба не должна затягивать RTT к нулю, получили %v", st.EWMARTTms)
	}
	if st.LastReason != "timeout" {
		t.Errorf("ожидали сохранённый reason, получили %q", st.LastReason)
	}
}

func TestNodeStatStore_RoundTripsThroughDisk(t *testing.T) {
	dir := t.TempDir()

	s1 := NewNodeStatStore(dir)
	s1.RecordProbe("k", 77, 3, true, "")
	s1.RecordConnect("k", false, "refused")
	if err := s1.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	s2 := NewNodeStatStore(dir)
	st := s2.Get("k")
	if st.EWMARTTms != 77 || st.ConnectFail != 1 {
		t.Errorf("состояние не пережило перезагрузку: %+v", st)
	}

	if _, err := filepath.Abs(dir); err != nil {
		t.Fatal(err)
	}
}

func TestNodeStatStore_UnknownKeyReturnsZeroValue(t *testing.T) {
	s := NewNodeStatStore(t.TempDir())
	if got := s.Get("missing"); got.ConnectOK != 0 || got.EWMARTTms != 0 {
		t.Errorf("неизвестный ключ должен давать нулевое значение, получили %+v", got)
	}
}

// TestNodeStatStore_RecordConnectSanitizesReasonItself is the regression test
// for the finding: RecordProbe and the package-level RecordConnectOutcome
// both sanitize reason before it can reach node_stats.json (an unencrypted
// file, on the promise it holds no addresses), but the exported
// NodeStatStore.RecordConnect method did not — it stored whatever the caller
// passed verbatim. No current caller misuses it, but the invariant should
// hold by construction, not by every present and future caller remembering
// to pre-sanitize.
func TestNodeStatStore_RecordConnectSanitizesReasonItself(t *testing.T) {
	s := NewNodeStatStore(t.TempDir())

	// A raw dial error, exactly the shape sanitizeStatReason exists to
	// collapse: Go's net package embeds the remote address in it.
	s.RecordConnect("k", false, "dial tcp 203.0.113.5:443: i/o timeout")

	got := s.Get("k").LastReason
	if got == "dial tcp 203.0.113.5:443: i/o timeout" {
		t.Fatal("RecordConnect stored the raw reason verbatim — an address reached node_stats.json unsanitized")
	}
	if got != "error" {
		t.Errorf("unrecognised reason should collapse to the canned \"error\", got %q", got)
	}

	// A reason already in the fixed vocabulary must survive unchanged.
	s.RecordConnect("k2", false, "timeout")
	if got := s.Get("k2").LastReason; got != "timeout" {
		t.Errorf("known-vocabulary reason should pass through, got %q", got)
	}
}
