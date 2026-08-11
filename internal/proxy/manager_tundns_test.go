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
	"testing"

	"resultproxy-wails/internal/logger"
)

func TestTunAdapterDNSAddrs(t *testing.T) {
	cases := []struct {
		name        string
		cidr        string
		wantAdapter string
		wantDNS     string
	}{
		{"default when empty", "", "172.19.0.1", "172.19.0.2"},
		{"standard /30", "172.19.0.1/30", "172.19.0.1", "172.19.0.2"},
		{"custom subnet", "10.10.0.1/24", "10.10.0.1", "10.10.0.2"},
		{"octet carry", "10.0.0.255/16", "10.0.0.255", "10.0.1.0"},
		{"no second host", "10.0.0.3/30", "", ""},
		{"garbage", "not-a-cidr", "", ""},
		{"ipv6 rejected", "fd00::1/64", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapterIP, dnsIP := tunAdapterDNSAddrs(tc.cidr)
			if adapterIP != tc.wantAdapter || dnsIP != tc.wantDNS {
				t.Fatalf("tunAdapterDNSAddrs(%q) = (%q, %q), want (%q, %q)",
					tc.cidr, adapterIP, dnsIP, tc.wantAdapter, tc.wantDNS)
			}
		})
	}
}

// TestConnect_TunnelAppliesTunAdapterDNS pins the second half of the 2026-06
// DNS fix: a tunnel connect must point the TUN adapter's resolver at the
// in-subnet hijack address so Windows system DNS keeps resolving through the
// tunnel (hijack-dns → sing-box DNS). Without it the OS resolver survives the
// session only on cache and app-level DoH — the physical adapters are pinned
// to resolvers unreachable outside the tunnel.
func TestConnect_TunnelAppliesTunAdapterDNS(t *testing.T) {
	prevAdmin := isAdminCheck
	isAdminCheck = func() bool { return true }
	prevHas, prevClear := hasLeftoverTunFn, clearLeftoverTunFn
	hasLeftoverTunFn = func() bool { return false }
	clearLeftoverTunFn = func() error { return nil }
	prevProbe := probeHTTPThroughProxyProbe
	probeHTTPThroughProxyProbe = func(string) (bool, string) { return true, "" }
	defer func() {
		isAdminCheck = prevAdmin
		hasLeftoverTunFn, clearLeftoverTunFn = prevHas, prevClear
		probeHTTPThroughProxyProbe = prevProbe
	}()

	host, port, closeFn := startReachableTCP(t)
	defer closeFn()

	sysDNS := &stubSystemDNS{}
	m := NewManager(logger.New())
	m.engine = &stubEngine{}
	m.sysProxy = &stubSystemProxy{}
	m.sysDNS = sysDNS

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
		false,
	)
	if !res.Success {
		t.Fatalf("connect failed: %+v", res)
	}
	if len(sysDNS.tunOverrideCalls) != 1 {
		t.Fatalf("expected exactly one tunnel-adapter DNS override, got %v", sysDNS.tunOverrideCalls)
	}
	got := sysDNS.tunOverrideCalls[0]
	if got[0] != "172.19.0.1" || got[1] != "172.19.0.2" {
		t.Fatalf("expected tunnel adapter 172.19.0.1 → dns 172.19.0.2, got %v", got)
	}
}

// Proxy mode has no TUN adapter — the tunnel-adapter DNS override must not run.
func TestConnect_ProxyModeDoesNotTouchTunnelAdapterDNS(t *testing.T) {
	prevAdmin := isAdminCheck
	isAdminCheck = func() bool { return true }
	defer func() { isAdminCheck = prevAdmin }()

	host, port, closeFn := startReachableTCP(t)
	defer closeFn()

	sysDNS := &stubSystemDNS{}
	m := NewManager(logger.New())
	m.engine = &stubEngine{}
	m.sysProxy = &stubSystemProxy{}
	m.sysDNS = sysDNS

	res := m.Connect(
		context.Background(),
		ProxyConfig{IP: host, Port: port, Type: "http"},
		ProxyModeProxy,
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
		false,
	)
	if !res.Success {
		t.Fatalf("connect failed: %+v", res)
	}
	if len(sysDNS.tunOverrideCalls) != 0 {
		t.Fatalf("proxy mode must not touch the tunnel adapter DNS, got %v", sysDNS.tunOverrideCalls)
	}
}
