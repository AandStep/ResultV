package main

import (
	"context"
	"testing"
)

// A clean quit must not pay the unconditional firewall sweep when no
// kill-switch rules are installed: RemoveLeftoverRules spawns ~20 serial netsh
// processes (seconds of delay before the network-critical DNS/route cleanup in
// proxy.Shutdown) and, on a non-admin instance, fires a pointless UAC prompt.
// HasLeftoverRules is the cheap probe (one netsh query for _BlockAll — the only
// rule that severs traffic; stray allow rules are harmless) that decides.
func TestShutdown_SkipsFirewallSweepWithoutLeftoverRules(t *testing.T) {
	app := NewApp()
	ks := &stubKillSwitch{hasLeftover: false}
	app.killSwitch = ks

	app.shutdown(context.Background())

	if ks.removeCalls != 0 {
		t.Fatalf("expected no RemoveLeftoverRules call when nothing is installed, got %d", ks.removeCalls)
	}
}

// When the _BlockAll rule IS present (kill switch tripped, then quit), the
// sweep must still run — it is the only thing standing between the user and a
// permanently severed internet after the process exits.
func TestShutdown_RemovesLeftoverFirewallRules(t *testing.T) {
	app := NewApp()
	ks := &stubKillSwitch{hasLeftover: true}
	app.killSwitch = ks

	app.shutdown(context.Background())

	if ks.removeCalls != 1 {
		t.Fatalf("expected RemoveLeftoverRules to run exactly once, got %d", ks.removeCalls)
	}
}
