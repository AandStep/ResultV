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
