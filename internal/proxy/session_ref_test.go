package proxy

import (
	"context"
	"testing"
)

// A measurement scheduled for one session must not be credited to whatever
// session is live when its delay runs out: after a switch the probe would
// time the new node and file the figure under the old one.
func TestPostConnectProbesSkipAnotherSession(t *testing.T) {
	first := ProxyConfig{IP: "78.17.18.170", Port: 31523, Type: "AMNEZIAWG"}
	second := ProxyConfig{IP: "203.0.113.5", Port: 443, Type: "VLESS"}
	m := &Manager{connected: true, mode: ProxyModeTunnel, proxy: &first, localPort: 1}
	ref := m.CurrentSession()

	m.proxy = &second

	if r := m.ProbeThroughputNow(context.Background(), ref); r.Reason != ProbeSessionChanged {
		t.Fatalf("скорость: %q", r.Reason)
	}
	if r := m.ProbeUDPRelayNow(context.Background(), ref); r.Reason != ProbeSessionChanged {
		t.Fatalf("UDP: %q", r.Reason)
	}

	m.connected, m.proxy = false, nil
	if r := m.ProbeThroughputNow(context.Background(), ref); r.Reason != ProbeSessionChanged {
		t.Fatalf("после отключения: %q", r.Reason)
	}
}
