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
	"reflect"
	"testing"

	"resultproxy-wails/internal/logger"
)

func TestDNSOverrideServers_EmptyFallsBackToCloudflareAndGoogle(t *testing.T) {
	got := dnsOverrideServers(nil)
	want := []string{"1.1.1.1", "8.8.8.8"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDNSOverrideServers_CustomDNSWins(t *testing.T) {
	got := dnsOverrideServers([]string{"9.9.9.9", "149.112.112.112"})
	want := []string{"9.9.9.9", "149.112.112.112"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDNSOverrideServers_StripsPortAndDedupes(t *testing.T) {
	got := dnsOverrideServers([]string{"1.1.1.1:53", "1.1.1.1", "8.8.8.8:5353"})
	want := []string{"1.1.1.1", "8.8.8.8"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDNSOverrideServers_SkipsBlankEntries(t *testing.T) {
	got := dnsOverrideServers([]string{"", "   ", "1.1.1.1"})
	want := []string{"1.1.1.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestConnect_PropagatesDNSLeakProtection ensures the UI toggle reaches the
// sing-box config: a Connect with dnsLeakProtection=true must produce an
// EngineConfig with DNSLeakProtection=true so BuildTunnelModeConfig in
// turn emits strict_route=true on the TUN inbound. Without this regression
// guard, a future signature refactor could silently drop the flag.
func TestConnect_PropagatesDNSLeakProtection(t *testing.T) {
	prevAdmin := isAdminCheck
	isAdminCheck = func() bool { return true }
	defer func() { isAdminCheck = prevAdmin }()

	// Stub the orphan-adapter pre-clear: on a dev machine with a REAL leftover
	// sing-tun adapter and no admin, the un-stubbed path would attempt a UAC
	// elevation from inside the test (faked isAdminCheck reaches tunnel connect,
	// but clearLeftoverTun checks the real token).
	prevHas, prevClear := hasLeftoverTunFn, clearLeftoverTunFn
	hasLeftoverTunFn = func() bool { return false }
	clearLeftoverTunFn = func() error { return nil }
	defer func() { hasLeftoverTunFn, clearLeftoverTunFn = prevHas, prevClear }()

	// The tunnel post-start probe now goes through the loopback probe inbound;
	// stub it — no engine actually serves it under stubEngine.
	prevProbe := probeHTTPThroughProxyProbe
	probeHTTPThroughProxyProbe = func(string) (bool, string) { return true, "" }
	defer func() { probeHTTPThroughProxyProbe = prevProbe }()

	host, port, closeFn := startReachableTCP(t)
	defer closeFn()

	log := logger.New()
	engine := &stubEngine{}
	sysProxy := &stubSystemProxy{}
	m := NewManager(log)
	m.engine = engine
	m.sysProxy = sysProxy

	res := m.Connect(
		context.Background(),
		ProxyConfig{IP: host, Port: port, Type: "http"},
		ProxyModeTunnel,
		ModeGlobal,
		nil,
		nil,
		nil,
		false,
		0,
		false,
		nil,
		"",
		"",
		true,
		false,
	)
	if !res.Success {
		t.Fatalf("connect failed: %+v", res)
	}
	if len(engine.startCalls) == 0 {
		t.Fatal("engine.Start not called")
	}
	last := engine.startCalls[len(engine.startCalls)-1]
	if !last.DNSLeakProtection {
		t.Fatalf("expected EngineConfig.DNSLeakProtection=true, got %+v", last)
	}
}
