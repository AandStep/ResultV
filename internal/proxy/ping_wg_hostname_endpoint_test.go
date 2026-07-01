package proxy

import (
	"encoding/json"
	"testing"
	"time"

	"resultproxy-wails/internal/config"
)

// TestPingWireGuardHandshake_HostnameEndpointResolves proves the probe resolves
// a hostname endpoint to a literal IP before handing it to the device, instead
// of passing the hostname straight into the UAPI endpoint= line. The vendored
// wireguard-go fork's Bind.ParseEndpoint (StdNetBind: netip.ParseAddrPort;
// WinRingBind: GetAddrInfoW with AI_NUMERICHOST) accepts ONLY numeric IP:port
// and never resolves hostnames, so an unresolved hostname endpoint makes
// dev.IpcSet fail instantly and deterministically with "probe_error" — the
// "AWG ping always errors" symptom for any server configured by hostname, even
// though the same hostname connects fine through the real (sing-box) path.
//
// "localhost" resolves locally without any network dependency and nothing
// listens on the probe port, so a correctly-fixed probe must reach the poll
// loop and time out — not fail fast with probe_error.
func TestPingWireGuardHandshake_HostnameEndpointResolves(t *testing.T) {
	if testing.Short() {
		t.Skip("5s handshake timeout; skipped under -short")
	}
	extra := `{"private_key":"` + b64key(0x01) + `","public_key":"` + b64key(0x02) + `"}`
	entry := config.ProxyEntry{IP: "localhost", Port: 51820, Type: "WIREGUARD", Extra: []byte(extra)}
	b, _ := json.Marshal(entry)

	done := make(chan struct{})
	var reachable bool
	var reason string
	go func() {
		_, reachable, reason = PingWireGuardHandshake(string(b))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(wgProbeHardCap + 3*time.Second):
		t.Fatal("probe did not return within timeout budget")
	}
	if reachable {
		t.Error("nothing listens on the probe port; must not report reachable")
	}
	if reason != "timeout" {
		t.Errorf("reason = %q, want timeout (got probe_error means the hostname endpoint was never resolved to a literal IP before IpcSet)", reason)
	}
}
