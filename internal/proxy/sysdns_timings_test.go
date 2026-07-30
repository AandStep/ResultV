package proxy

import (
	"testing"
	"time"
)

func TestDNSPhaseTimings_EmptyFormatsToEmpty(t *testing.T) {
	var tm dnsPhaseTimings
	if got := tm.take(); got != "" {
		t.Fatalf("timings without any recorded step must format to \"\", got %q", got)
	}
}

func TestDNSPhaseTimings_FormatAndReset(t *testing.T) {
	var tm dnsPhaseTimings
	tm.recordList(12*time.Millisecond, false)
	tm.recordSnapshot(3 * time.Millisecond)
	tm.recordSet(700*time.Millisecond, false)
	tm.recordSet(680*time.Millisecond, true)
	tm.recordTun(126*time.Millisecond, false)

	want := "list=12ms(native) snapshot=3ms adapters=2 set=1380ms(ps=1) tun=126ms(native)"
	if got := tm.take(); got != want {
		t.Fatalf("take() = %q, want %q", got, want)
	}
	if got := tm.take(); got != "" {
		t.Fatalf("take() must reset, second call returned %q", got)
	}
}

func TestDNSPhaseTimings_TunOnly(t *testing.T) {
	var tm dnsPhaseTimings
	tm.recordTun(40*time.Millisecond, true)
	want := "list=0ms(native) snapshot=0ms adapters=0 set=0ms(ps=0) tun=40ms(ps)"
	if got := tm.take(); got != want {
		t.Fatalf("take() = %q, want %q", got, want)
	}
}
