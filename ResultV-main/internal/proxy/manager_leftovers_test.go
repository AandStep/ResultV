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
	"testing"

	"resultproxy-wails/internal/logger"
)

// stubSystemDNS records Override/Restore calls and fakes snapshot presence so
// teardown and leftover-detection paths can be asserted without touching the
// real OS resolver.
type stubSystemDNS struct {
	overrideCalls    int
	restoreCalls     int
	snapshot         bool
	tunOverrideCalls [][2]string
}

func (s *stubSystemDNS) Override(servers []string) error {
	s.overrideCalls++
	s.snapshot = true
	return nil
}

func (s *stubSystemDNS) OverrideTunnelAdapter(adapterIP, dnsIP string) error {
	s.tunOverrideCalls = append(s.tunOverrideCalls, [2]string{adapterIP, dnsIP})
	return nil
}

func (s *stubSystemDNS) Restore() error {
	s.restoreCalls++
	s.snapshot = false
	return nil
}

func (s *stubSystemDNS) SnapshotExists() bool { return s.snapshot }

func (s *stubSystemDNS) RestoreCommands() ([]string, error) { return nil, nil }
func (s *stubSystemDNS) DeleteSnapshot() error             { s.snapshot = false; return nil }

// TestShutdown_RestoresDNS pins the priority fix: a clean quit (tray → Quit →
// app.shutdown → Manager.Shutdown) must restore the system DNS, otherwise the
// OS resolver stays pointed at our override after the app is gone — internet
// "hangs" until the next launch. Restore must run unconditionally (the
// in-memory connected flag is not trustworthy at shutdown) and is idempotent.
func TestShutdown_RestoresDNS(t *testing.T) {
	stubLeftoverTun(t, false)

	log := logger.New()
	m := NewManager(log)
	m.engine = &stubEngine{}
	m.sysProxy = &stubSystemProxy{}
	sysDNS := &stubSystemDNS{snapshot: true}
	m.sysDNS = sysDNS

	m.Shutdown()

	if sysDNS.restoreCalls == 0 {
		t.Fatal("expected Shutdown to restore system DNS, but Restore was never called")
	}
}

// TestShutdown_TunnelClearsTunRoutes pins the tunnel-mode fix: when the engine
// is connected in tunnel mode, Shutdown must explicitly clear the sing-tun
// adapter's routes BEFORE calling engine.Stop(). If instance.Close() hangs
// (common with WireGuard/AmneziaWG) the auto_route default route would strand
// traffic and the user loses internet. By clearing routes first, the physical
// adapter's default route takes over immediately.
func TestShutdown_TunnelClearsTunRoutes(t *testing.T) {
	cleared := stubLeftoverTun(t, true)

	log := logger.New()
	m := NewManager(log)
	m.engine = &stubEngine{}
	m.sysProxy = &stubSystemProxy{}
	m.sysDNS = &stubSystemDNS{}
	m.connected = true
	m.mode = ProxyModeTunnel

	m.Shutdown()

	if *cleared == 0 {
		t.Fatal("expected Shutdown in tunnel mode to clear TUN routes, but clearLeftoverTun was never called")
	}
}


// stubLeftoverTun swaps the injectable sing-tun probe/cleanup so RecoverSystem-
// Leftovers can be tested without a real adapter or touching the routing table.
func stubLeftoverTun(t *testing.T, present bool) *int {
	t.Helper()
	prevHas, prevClear := hasLeftoverTunFn, clearLeftoverTunFn
	cleared := new(int)
	hasLeftoverTunFn = func() bool { return present }
	clearLeftoverTunFn = func() error { *cleared++; return nil }
	t.Cleanup(func() { hasLeftoverTunFn, clearLeftoverTunFn = prevHas, prevClear })
	return cleared
}

// stubAdmin pins the package-level admin check for the duration of a test so
// recovery behaviour doesn't depend on the real token of the dev shell.
func stubAdmin(t *testing.T, admin bool) {
	t.Helper()
	prev := isAdminCheck
	isAdminCheck = func() bool { return admin }
	t.Cleanup(func() { isAdminCheck = prev })
}

// TestRecoverSystemLeftovers_DetectsAndClearsAll pins the ELEVATED startup
// recovery path: a stranded system proxy, an un-restored DNS override, and a
// leftover sing-tun adapter are all detected and removed in one pass.
func TestRecoverSystemLeftovers_DetectsAndClearsAll(t *testing.T) {
	stubAdmin(t, true)
	cleared := stubLeftoverTun(t, true)

	log := logger.New()
	m := NewManager(log)
	m.engine = &stubEngine{}
	sysProxy := &stubSystemProxy{leftover: true}
	sysDNS := &stubSystemDNS{snapshot: true}
	m.sysProxy = sysProxy
	m.sysDNS = sysDNS

	scan := m.RecoverSystemLeftovers()

	if !scan.Proxy || !scan.DNS || !scan.Tun {
		t.Fatalf("expected all leftovers detected, got %+v", scan)
	}
	if scan.NeedsElevation {
		t.Fatalf("elevated process must not request elevation, got %+v", scan)
	}
	if sysProxy.disableCall == 0 {
		t.Fatal("expected system proxy Disable to be called")
	}
	if sysDNS.restoreCalls == 0 {
		t.Fatal("expected system DNS Restore to be called")
	}
	if *cleared == 0 {
		t.Fatal("expected leftover sing-tun adapter to be cleared")
	}
}

// TestRecoverSystemLeftovers_NonAdminReportsElevation pins the new no-surprise-
// UAC contract: without admin, DNS/tun leftovers are DETECTED but not touched —
// the scan asks the app layer to offer an elevated restart. The system proxy
// (registry-only) is still cleaned immediately.
func TestRecoverSystemLeftovers_NonAdminReportsElevation(t *testing.T) {
	stubAdmin(t, false)
	cleared := stubLeftoverTun(t, true)

	log := logger.New()
	m := NewManager(log)
	m.engine = &stubEngine{}
	sysProxy := &stubSystemProxy{leftover: true}
	sysDNS := &stubSystemDNS{snapshot: true}
	m.sysProxy = sysProxy
	m.sysDNS = sysDNS

	scan := m.RecoverSystemLeftovers()

	if !scan.Proxy || !scan.DNS || !scan.Tun || !scan.NeedsElevation {
		t.Fatalf("expected detection + elevation request, got %+v", scan)
	}
	if sysProxy.disableCall == 0 {
		t.Fatal("system proxy cleanup needs no admin and must still run")
	}
	if sysDNS.restoreCalls != 0 {
		t.Fatal("DNS restore must NOT run without admin (it would fail or UAC)")
	}
	if *cleared != 0 {
		t.Fatal("tun cleanup must NOT run without admin (it would fire a UAC)")
	}
}

// TestRecoverSystemLeftovers_SkipsWhileSessionActive is the regression guard for
// the field failure where async startup recovery finished ~2s AFTER a fast
// connect, mistook the LIVE sing-tun adapter (and the system proxy / DNS WE had
// just set) for leftovers, reverted them — deleting the tunnel's default route —
// and tripped the kill switch on a healthy HYSTERIA2 server. While a session of
// ours is connected, establishing, or the engine is running, recovery must
// revert NOTHING.
func TestRecoverSystemLeftovers_SkipsWhileSessionActive(t *testing.T) {
	cases := []struct {
		name  string
		setup func(m *Manager)
	}{
		{"connected", func(m *Manager) { m.connected = true }},
		{"establishing", func(m *Manager) { p := ProxyConfig{IP: "1.2.3.4", Port: 443}; m.pendingProxy = &p }},
		{"engineRunning", func(m *Manager) { m.engine = &stubEngine{running: true} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prevHas, prevClear := hasLeftoverTunFn, clearLeftoverTunFn
			hasLeftoverTunFn = func() bool { return true }
			clearLeftoverTunFn = func() error {
				t.Fatal("clearLeftoverTun must NOT run while our own session is active — it would delete the live tunnel's default route")
				return nil
			}
			t.Cleanup(func() { hasLeftoverTunFn, clearLeftoverTunFn = prevHas, prevClear })

			log := logger.New()
			m := NewManager(log)
			m.engine = &stubEngine{}
			sysProxy := &stubSystemProxy{leftover: true}
			sysDNS := &stubSystemDNS{snapshot: true}
			m.sysProxy = sysProxy
			m.sysDNS = sysDNS

			tc.setup(m)

			scan := m.RecoverSystemLeftovers()

			if scan.Any() || scan.NeedsElevation {
				t.Fatalf("recovery must report nothing while session active, got %+v", scan)
			}
			if sysProxy.disableCall != 0 {
				t.Fatal("system proxy Disable must not run while our session is active — it would break the live proxy session")
			}
			if sysDNS.restoreCalls != 0 {
				t.Fatal("system DNS Restore must not run while our session is active")
			}
		})
	}
}

// TestRecoverSystemLeftovers_NoneWhenClean guarantees recovery is inert on a
// clean machine: nothing is detected and no removal side effect runs.
func TestRecoverSystemLeftovers_NoneWhenClean(t *testing.T) {
	stubAdmin(t, true)
	prevHas, prevClear := hasLeftoverTunFn, clearLeftoverTunFn
	hasLeftoverTunFn = func() bool { return false }
	clearLeftoverTunFn = func() error {
		t.Fatal("clearLeftoverTun must not run when no tun leftover is present")
		return nil
	}
	t.Cleanup(func() { hasLeftoverTunFn, clearLeftoverTunFn = prevHas, prevClear })

	log := logger.New()
	m := NewManager(log)
	m.engine = &stubEngine{}
	sysProxy := &stubSystemProxy{leftover: false}
	sysDNS := &stubSystemDNS{snapshot: false}
	m.sysProxy = sysProxy
	m.sysDNS = sysDNS

	scan := m.RecoverSystemLeftovers()

	if scan.Any() || scan.NeedsElevation {
		t.Fatalf("expected no leftovers, got %+v", scan)
	}
	if sysProxy.disableCall != 0 {
		t.Fatal("system proxy Disable must not run when nothing is stranded")
	}
	if sysDNS.restoreCalls != 0 {
		t.Fatal("system DNS Restore must not run when no snapshot exists")
	}
}
