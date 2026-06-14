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

import "testing"

func TestManagerPing_TunnelUsesLANProbeForNonHysteria2(t *testing.T) {
	m := NewManager(nil)
	m.connected = true
	m.mode = ProxyModeTunnel
	m.proxy = &ProxyConfig{
		IP:   "1.2.3.4",
		Port: 443,
		Type: "http",
	}

	oldTCP := pingTCPProbe
	oldLAN := pingLANProbe
	oldHY2 := pingHysteria2Probe
	defer func() {
		pingTCPProbe = oldTCP
		pingLANProbe = oldLAN
		pingHysteria2Probe = oldHY2
	}()

	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		return 0, false, "timeout"
	}
	pingLANProbe = func(_ string, _ int) (int64, bool, string) {
		return 55, true, ""
	}
	pingHysteria2Probe = func(_ string, _ int) (int64, bool, string, string) {
		return 0, false, "error", "udp"
	}

	res := m.Ping("1.2.3.4", 443, "http")
	if !res.Reachable || res.LatencyMs != 55 || res.CheckType != "tcp_lan_bind" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestManagerPing_TunnelUsesLANProbeForHysteria2(t *testing.T) {
	m := NewManager(nil)
	m.connected = true
	m.mode = ProxyModeTunnel
	m.proxy = &ProxyConfig{
		IP:   "9.9.9.9",
		Port: 443,
		Type: "hysteria2",
	}

	oldHY2 := pingHysteria2Probe
	oldH2LAN := pingHysteria2LANProbe
	defer func() {
		pingHysteria2Probe = oldHY2
		pingHysteria2LANProbe = oldH2LAN
	}()

	hy2Called := 0
	hy2LANCalled := 0
	pingHysteria2Probe = func(_ string, _ int) (int64, bool, string, string) {
		hy2Called++
		return 4, true, "", "tcp_fallback"
	}
	pingHysteria2LANProbe = func(_ string, _ int) (int64, bool, string, string) {
		hy2LANCalled++
		return -1, true, "", "udp_lan_bind"
	}

	res := m.Ping("1.2.3.4", 443, "hysteria2")
	if !res.Reachable || res.LatencyMs != -1 || res.CheckType != "udp_lan_bind" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if hy2Called != 0 || hy2LANCalled != 1 {
		t.Fatalf("unexpected calls hy2=%d hy2lan=%d", hy2Called, hy2LANCalled)
	}
}

func TestManagerPing_Hysteria2FallsBackToTCPWhenUDPProbeFails(t *testing.T) {
	m := NewManager(nil)
	m.connected = false
	m.mode = ProxyModeProxy
	m.proxy = &ProxyConfig{
		IP:   "1.2.3.4",
		Port: 443,
		Type: "hysteria2",
	}

	oldTCP := pingTCPProbe
	oldLAN := pingLANProbe
	oldHY2 := pingHysteria2Probe
	defer func() {
		pingTCPProbe = oldTCP
		pingLANProbe = oldLAN
		pingHysteria2Probe = oldHY2
	}()

	
	
	pingHysteria2Probe = func(_ string, _ int) (int64, bool, string, string) {
		return 61, true, "", "tcp_fallback"
	}
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		t.Error("pingTCPProbe should not be called — TCP fallback is internal to pingHysteria2Probe")
		return 0, false, "timeout"
	}
	pingLANProbe = func(_ string, _ int) (int64, bool, string) {
		return 0, false, "timeout"
	}

	res := m.Ping("1.2.3.4", 443, "hysteria2")
	if !res.Reachable || res.LatencyMs != 61 || res.CheckType != "tcp_fallback" || res.Reason != "" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestManagerPing_TunnelUsesLANProbeForWireGuard(t *testing.T) {
	m := NewManager(nil)
	m.connected = true
	m.mode = ProxyModeTunnel
	m.proxy = &ProxyConfig{
		IP:   "9.9.9.9",
		Port: 51820,
		Type: "wireguard",
	}

	oldTCP := pingTCPProbe
	oldWG := pingWireGuardProbe
	oldWGLAN := pingWireGuardLANProbe
	defer func() {
		pingTCPProbe = oldTCP
		pingWireGuardProbe = oldWG
		pingWireGuardLANProbe = oldWGLAN
	}()

	tcpCalled := 0
	wgCalled := 0
	wgLANCalled := 0

	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		tcpCalled++
		return 4, true, ""
	}
	pingWireGuardProbe = func(_ string, _ int) (int64, bool, string) {
		wgCalled++
		return 4, true, ""
	}
	pingWireGuardLANProbe = func(_ string, _ int) (int64, bool, string) {
		wgLANCalled++
		return -1, true, ""
	}

	res := m.Ping("1.2.3.4", 51820, "wireguard")
	if !res.Reachable || res.LatencyMs != -1 || res.CheckType != "udp_lan_bind" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if tcpCalled != 0 || wgCalled != 0 || wgLANCalled != 1 {
		t.Fatalf("unexpected calls tcp=%d wg=%d wglan=%d", tcpCalled, wgCalled, wgLANCalled)
	}
}

func TestProbeProxyAlive_TunnelUsesLANProbeForTCPTransports(t *testing.T) {
	m := NewManager(nil)
	proxy := ProxyConfig{IP: "1.2.3.4", Port: 443, Type: "http"}

	oldTCP := pingTCPProbe
	oldLAN := pingLANProbe
	defer func() {
		pingTCPProbe = oldTCP
		pingLANProbe = oldLAN
	}()

	tcpCalled := 0
	lanCalled := 0
	pingTCPProbe = func(_ string, _ int) (int64, bool, string) {
		tcpCalled++
		return 0, false, "timeout"
	}
	pingLANProbe = func(_ string, _ int) (int64, bool, string) {
		lanCalled++
		return 12, true, ""
	}

	alive, _ := m.probeProxyAlive(proxy, ProxyModeTunnel)
	if !alive {
		t.Fatal("expected alive via LAN probe")
	}
	if tcpCalled != 0 || lanCalled != 1 {
		t.Fatalf("unexpected calls tcp=%d lan=%d", tcpCalled, lanCalled)
	}
}

func TestProbeProxyAlive_TunnelUsesLANProbeForWireGuard(t *testing.T) {
	m := NewManager(nil)
	proxy := ProxyConfig{IP: "1.2.3.4", Port: 51820, Type: "wireguard"}

	oldWG := pingWireGuardProbe
	oldWGLAN := pingWireGuardLANProbe
	defer func() {
		pingWireGuardProbe = oldWG
		pingWireGuardLANProbe = oldWGLAN
	}()

	wgCalled := 0
	wgLANCalled := 0
	pingWireGuardProbe = func(_ string, _ int) (int64, bool, string) {
		wgCalled++
		return 0, false, "timeout"
	}
	pingWireGuardLANProbe = func(_ string, _ int) (int64, bool, string) {
		wgLANCalled++
		return -1, true, ""
	}

	alive, _ := m.probeProxyAlive(proxy, ProxyModeTunnel)
	if !alive {
		t.Fatal("expected alive via wireguard LAN probe")
	}
	if wgCalled != 0 || wgLANCalled != 1 {
		t.Fatalf("unexpected calls wg=%d wglan=%d", wgCalled, wgLANCalled)
	}
}

// TestProbeHealthy_TunnelWithEngineUsesLoopbackProbe pins the 2026-06 fix for
// false kill-switch trips: the tunnel watchdog probe must go through the local
// probe inbound (127.0.0.1:localPort), where sing-box resolves the hostname
// remotely — NOT through the TUN default route, whose getaddrinfo dies once
// the session's own DNS override + strict_route degrade the OS resolver.
func TestProbeHealthy_TunnelWithEngineUsesLoopbackProbe(t *testing.T) {
	m := NewManager(nil)
	m.engine = &stubEngine{running: true}

	oldTunnel := probeTunnelHealthProbe
	oldProxy := probeProxyHealthProbe
	defer func() {
		probeTunnelHealthProbe = oldTunnel
		probeProxyHealthProbe = oldProxy
	}()

	tunnelCalled := 0
	proxyCalled := 0
	gotPort := 0
	probeTunnelHealthProbe = func() (bool, string) {
		tunnelCalled++
		return false, "must not be used"
	}
	probeProxyHealthProbe = func(port int) (bool, string) {
		proxyCalled++
		gotPort = port
		return true, ""
	}

	alive, _ := m.probeHealthy(ProxyConfig{IP: "1.2.3.4", Port: 443, Type: "hysteria2"}, ProxyModeTunnel, 14081, true)
	if !alive {
		t.Fatal("expected alive via loopback probe")
	}
	if proxyCalled != 1 || tunnelCalled != 0 {
		t.Fatalf("unexpected calls loopback=%d tunnel=%d", proxyCalled, tunnelCalled)
	}
	if gotPort != 14081 {
		t.Fatalf("expected probe on local port 14081, got %d", gotPort)
	}
}

// Without a known local port the watchdog falls back to the legacy
// default-route probe rather than probing nothing at all.
func TestProbeHealthy_TunnelWithoutLocalPortFallsBackToTunnelProbe(t *testing.T) {
	m := NewManager(nil)
	m.engine = &stubEngine{running: true}

	oldTunnel := probeTunnelHealthProbe
	oldProxy := probeProxyHealthProbe
	defer func() {
		probeTunnelHealthProbe = oldTunnel
		probeProxyHealthProbe = oldProxy
	}()

	tunnelCalled := 0
	proxyCalled := 0
	probeTunnelHealthProbe = func() (bool, string) {
		tunnelCalled++
		return true, ""
	}
	probeProxyHealthProbe = func(_ int) (bool, string) {
		proxyCalled++
		return false, "must not be used"
	}

	alive, _ := m.probeHealthy(ProxyConfig{IP: "1.2.3.4", Port: 443, Type: "hysteria2"}, ProxyModeTunnel, 0, true)
	if !alive {
		t.Fatal("expected alive via tunnel HTTP probe fallback")
	}
	if tunnelCalled != 1 || proxyCalled != 0 {
		t.Fatalf("unexpected calls tunnel=%d loopback=%d", tunnelCalled, proxyCalled)
	}
}

func TestProbeHealthy_TunnelEngineNotRunningFallsBackToDirectProbe(t *testing.T) {
	m := NewManager(nil)

	oldTunnel := probeTunnelHealthProbe
	oldLAN := pingHysteria2LANProbe
	defer func() {
		probeTunnelHealthProbe = oldTunnel
		pingHysteria2LANProbe = oldLAN
	}()

	tunnelCalled := 0
	probeTunnelHealthProbe = func() (bool, string) {
		tunnelCalled++
		return true, ""
	}
	pingHysteria2LANProbe = func(_ string, _ int) (int64, bool, string, string) {
		return -1, true, "", "udp_lan_bind"
	}

	alive, _ := m.probeHealthy(ProxyConfig{IP: "1.2.3.4", Port: 443, Type: "hysteria2"}, ProxyModeTunnel, 0, false)
	if !alive {
		t.Fatal("expected alive via direct probe fallback")
	}
	if tunnelCalled != 0 {
		t.Fatalf("tunnel HTTP probe must not be called when engine is not running")
	}
}

func TestProbeHealthy_TunnelLoopbackProbeCalledForAllProtocols(t *testing.T) {
	protocols := []string{"hysteria2", "wireguard", "amneziawg", "http", "vmess", "vless"}
	for _, proto := range protocols {
		t.Run(proto, func(t *testing.T) {
			m := NewManager(nil)
			m.engine = &stubEngine{running: true}

			oldProxy := probeProxyHealthProbe
			defer func() { probeProxyHealthProbe = oldProxy }()

			called := 0
			probeProxyHealthProbe = func(_ int) (bool, string) {
				called++
				return true, ""
			}

			alive, _ := m.probeHealthy(ProxyConfig{IP: "1.2.3.4", Port: 443, Type: proto}, ProxyModeTunnel, 14081, true)
			if !alive || called != 1 {
				t.Fatalf("proto=%s: expected loopback probe, called=%d alive=%v", proto, called, alive)
			}
		})
	}
}

func TestProbeProxyAlive_TunnelUsesLANProbeForHysteria2(t *testing.T) {
	m := NewManager(nil)
	proxy := ProxyConfig{IP: "1.2.3.4", Port: 443, Type: "hysteria2"}

	oldHY2 := pingHysteria2Probe
	oldH2LAN := pingHysteria2LANProbe
	defer func() {
		pingHysteria2Probe = oldHY2
		pingHysteria2LANProbe = oldH2LAN
	}()

	hy2Called := 0
	hy2LANCalled := 0
	pingHysteria2Probe = func(_ string, _ int) (int64, bool, string, string) {
		hy2Called++
		return 0, false, "timeout", "udp"
	}
	pingHysteria2LANProbe = func(_ string, _ int) (int64, bool, string, string) {
		hy2LANCalled++
		return -1, true, "", "udp_lan_bind"
	}

	alive, _ := m.probeProxyAlive(proxy, ProxyModeTunnel)
	if !alive {
		t.Fatal("expected alive via hysteria2 LAN probe")
	}
	if hy2Called != 0 || hy2LANCalled != 1 {
		t.Fatalf("unexpected calls hy2=%d hy2lan=%d", hy2Called, hy2LANCalled)
	}
}
