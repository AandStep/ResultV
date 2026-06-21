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

// connectWithTunStubs runs a Connect with the leftover-tun detection stubbed to
// `present` and reports whether clearLeftoverTunFn was invoked.
func connectWithTunStubs(t *testing.T, mode ProxyMode, present bool) bool {
	t.Helper()

	prevAdmin := isAdminCheck
	isAdminCheck = func() bool { return true }
	prevHas, prevClear := hasLeftoverTunFn, clearLeftoverTunFn
	cleared := false
	hasLeftoverTunFn = func() bool { return present }
	clearLeftoverTunFn = func() error { cleared = true; return nil }
	prevTunnelProbe := probeHTTPThroughProxyProbe
	probeHTTPThroughProxyProbe = func(string) (bool, string) { return true, "" }
	t.Cleanup(func() {
		isAdminCheck = prevAdmin
		hasLeftoverTunFn, clearLeftoverTunFn = prevHas, prevClear
		probeHTTPThroughProxyProbe = prevTunnelProbe
	})

	host, port, closeFn := startReachableTCP(t)
	defer closeFn()

	m := NewManager(logger.New())
	m.engine = &stubEngine{}
	m.sysProxy = &stubSystemProxy{}

	res := m.Connect(
		context.Background(),
		ProxyConfig{IP: host, Port: port, Type: "http"},
		mode,
		ModeGlobal,
		nil, nil,
		false, false,
		0, false, nil, "", "",
		false,
	)
	if !res.Success {
		t.Fatalf("connect failed: %+v", res)
	}
	return cleared
}

// A force-killed prior session leaves an orphan sing-tun adapter holding a
// stale default route; tunnel connect must clear it BEFORE the engine reclaims
// the adapter (the route conflict behind the false kill-switch trips).
func TestConnect_TunnelClearsOrphanTunBeforeStart(t *testing.T) {
	if !connectWithTunStubs(t, ProxyModeTunnel, true) {
		t.Fatal("expected clearLeftoverTunFn to be called for tunnel connect with an orphan present")
	}
}

func TestConnect_TunnelNoOrphanNoClear(t *testing.T) {
	if connectWithTunStubs(t, ProxyModeTunnel, false) {
		t.Fatal("clearLeftoverTunFn must not be called when no orphan is present")
	}
}

func TestConnect_ProxyModeNeverClearsTun(t *testing.T) {
	if connectWithTunStubs(t, ProxyModeProxy, true) {
		t.Fatal("clearLeftoverTunFn must not be called in proxy mode (no admin guarantee)")
	}
}
