package proxy

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"resultproxy-wails/internal/config"
)

// b64key returns the standard-base64 encoding of a 32-byte key whose every
// byte is b — handy for asserting the hex form in the generated UAPI config.
func b64key(b byte) string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = b
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func hexkey(b byte) string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = b
	}
	return hex.EncodeToString(raw)
}

func TestBuildWGUAPI_PlainWireGuard(t *testing.T) {
	extra := `{"private_key":"` + b64key(0x01) + `","public_key":"` + b64key(0x02) + `"}`
	entry := config.ProxyEntry{
		IP:    "203.0.113.7",
		Port:  51820,
		Type:  "WIREGUARD",
		Extra: []byte(extra),
	}

	uapi, endpoint, err := buildWGUAPI(entry)
	if err != nil {
		t.Fatalf("buildWGUAPI: %v", err)
	}
	if endpoint != "203.0.113.7:51820" {
		t.Errorf("endpoint = %q, want 203.0.113.7:51820", endpoint)
	}
	if !strings.Contains(uapi, "private_key="+hexkey(0x01)) {
		t.Errorf("uapi missing private_key hex:\n%s", uapi)
	}
	if !strings.Contains(uapi, "public_key="+hexkey(0x02)) {
		t.Errorf("uapi missing public_key hex:\n%s", uapi)
	}
	if !strings.Contains(uapi, "endpoint=203.0.113.7:51820") {
		t.Errorf("uapi missing endpoint:\n%s", uapi)
	}
	if !strings.Contains(uapi, "allowed_ip=0.0.0.0/0") {
		t.Errorf("uapi missing allowed_ip:\n%s", uapi)
	}
	// Plain WireGuard must not emit any AmneziaWG knobs.
	if strings.Contains(uapi, "\njc=") || strings.Contains(uapi, "\nh1=") {
		t.Errorf("plain WG should not carry amnezia params:\n%s", uapi)
	}
	// Device-level keys must precede the peer block (public_key starts the peer).
	if pkIdx, pubIdx := strings.Index(uapi, "private_key="), strings.Index(uapi, "public_key="); pkIdx < 0 || pubIdx < pkIdx {
		t.Errorf("private_key must come before public_key:\n%s", uapi)
	}
}

func TestBuildWGUAPI_AmneziaParams(t *testing.T) {
	extra := `{"private_key":"` + b64key(0x01) + `","public_key":"` + b64key(0x02) +
		`","pre_shared_key":"` + b64key(0x03) +
		`","amnezia":{"jc":4,"jmin":40,"jmax":70,"s1":15,"s2":18,"h1":"1","h2":"2","h3":"3","h4":"4"}}`
	entry := config.ProxyEntry{
		IP:    "198.51.100.9",
		Port:  443,
		Type:  "AMNEZIAWG",
		Extra: []byte(extra),
	}

	uapi, _, err := buildWGUAPI(entry)
	if err != nil {
		t.Fatalf("buildWGUAPI: %v", err)
	}
	for _, want := range []string{"jc=4", "jmin=40", "jmax=70", "s1=15", "s2=18", "h1=1", "h2=2", "h3=3", "h4=4"} {
		if !strings.Contains(uapi, want) {
			t.Errorf("uapi missing amnezia knob %q:\n%s", want, uapi)
		}
	}
	if !strings.Contains(uapi, "preshared_key="+hexkey(0x03)) {
		t.Errorf("uapi missing preshared_key:\n%s", uapi)
	}
	// Amnezia device knobs must precede the peer block.
	if jcIdx, pubIdx := strings.Index(uapi, "jc="), strings.Index(uapi, "public_key="); jcIdx < 0 || pubIdx < jcIdx {
		t.Errorf("amnezia knobs must come before public_key:\n%s", uapi)
	}
}

// TestPingWireGuardHandshake_Timeout drives the full Device lifecycle against a
// black-hole TEST-NET endpoint (RFC 5737, never routable). It must set up the
// device, poll, and tear down cleanly, returning a timeout rather than hanging
// or panicking.
func TestPingWireGuardHandshake_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("5s handshake timeout; skipped under -short")
	}
	extra := `{"private_key":"` + b64key(0x01) + `","public_key":"` + b64key(0x02) + `"}`
	entry := config.ProxyEntry{IP: "203.0.113.250", Port: 51820, Type: "WIREGUARD", Extra: []byte(extra)}
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
	case <-time.After(wgHandshakeTimeout + 3*time.Second):
		t.Fatal("probe did not return within timeout budget")
	}
	if reachable {
		t.Error("black-hole endpoint must not be reachable")
	}
	if reason != "timeout" {
		t.Errorf("reason = %q, want timeout", reason)
	}
}

func TestPingWireGuardHandshake_BadJSON(t *testing.T) {
	_, reachable, reason := PingWireGuardHandshake("{not json")
	if reachable || reason != "probe_error" {
		t.Errorf("got reachable=%v reason=%q, want false/probe_error", reachable, reason)
	}
}

func TestBuildWGUAPI_MissingKeys(t *testing.T) {
	entry := config.ProxyEntry{
		IP:    "203.0.113.7",
		Port:  51820,
		Type:  "WIREGUARD",
		Extra: []byte(`{"public_key":"` + b64key(0x02) + `"}`),
	}
	if _, _, err := buildWGUAPI(entry); err == nil {
		t.Error("expected error when private_key is missing")
	}
}
