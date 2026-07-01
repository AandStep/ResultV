package proxy

import (
	"encoding/json"
	"testing"
	"time"

	"resultproxy-wails/internal/config"
)

// TestPingWireGuardHandshake_HardCapOnHungSetup proves the probe always returns
// within an overall hard budget even when the device setup/teardown (resolve,
// NewDevice, Up, Close) hangs — the failure mode behind the Android "infinite
// ping" for AmneziaWG, where net.ResolveUDPAddr on a .conf hostname endpoint can
// block forever and the 5s poll-loop budget never gets a chance to apply.
func TestPingWireGuardHandshake_HardCapOnHungSetup(t *testing.T) {
	// Substitute a probe body that never returns, simulating a wedged setup.
	orig := wgHandshakeProbe
	origCap := wgProbeHardCap
	wgHandshakeProbe = func(uapi, endpoint string) (int64, bool, string) {
		select {} // block forever
	}
	wgProbeHardCap = 200 * time.Millisecond
	defer func() { wgHandshakeProbe = orig; wgProbeHardCap = origCap }()

	extra := `{"private_key":"` + b64key(0x01) + `","public_key":"` + b64key(0x02) + `"}`
	entry := config.ProxyEntry{IP: "203.0.113.7", Port: 51820, Type: "AMNEZIAWG", Extra: []byte(extra)}
	b, _ := json.Marshal(entry)

	done := make(chan struct{})
	var reachable bool
	var reason string
	start := time.Now()
	go func() {
		_, reachable, reason = PingWireGuardHandshake(string(b))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("probe did not return despite hard cap — still hangs")
	}
	if reachable {
		t.Error("hung probe must not report reachable")
	}
	if reason != "timeout" {
		t.Errorf("reason = %q, want timeout", reason)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("returned in %v, want ~hard cap", elapsed)
	}
}
