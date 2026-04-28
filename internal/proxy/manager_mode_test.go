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
	"errors"
	"net"
	"strings"
	"testing"

	"resultproxy-wails/internal/logger"
)

type stubEngine struct {
	startErr   error
	startCalls []EngineConfig
	stopCalls  int
	running    bool
}

func (s *stubEngine) Start(_ context.Context, cfg EngineConfig) error {
	s.startCalls = append(s.startCalls, cfg)
	if s.startErr != nil {
		return s.startErr
	}
	s.running = true
	return nil
}

func (s *stubEngine) Stop() error {
	s.stopCalls++
	s.running = false
	return nil
}

func (s *stubEngine) IsRunning() bool { return s.running }
func (s *stubEngine) GetTrafficStats() (up, down int64) {
	return 0, 0
}

type stubSystemProxy struct {
	setCalls    []string
	disableCall int
}

func (s *stubSystemProxy) Set(addr string, _ []string) error {
	s.setCalls = append(s.setCalls, addr)
	return nil
}

func (s *stubSystemProxy) Disable() error {
	s.disableCall++
	return nil
}

func (s *stubSystemProxy) DisableSync()           {}
func (s *stubSystemProxy) ApplyKillSwitch() error { return nil }

func startReachableTCP(t *testing.T) (host string, port int, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	return "127.0.0.1", addr.Port, func() {
		_ = ln.Close()
		<-done
	}
}

func TestConnect_TunnelStartFailureIncludesReasonAndFallbackFlag(t *testing.T) {
	prev := isAdminCheck
	isAdminCheck = func() bool { return true }
	defer func() { isAdminCheck = prev }()

	host, port, closeFn := startReachableTCP(t)
	defer closeFn()

	log := logger.New()
	engine := &stubEngine{
		startErr: errors.New("starting sing-box: start inbound/tun[tun-in]: configure tun interface: Access is denied"),
	}
	sysProxy := &stubSystemProxy{}
	m := NewManager(log)
	m.engine = engine
	m.sysProxy = sysProxy

	result := m.Connect(
		context.Background(),
		ProxyConfig{IP: host, Port: port, Type: "http"},
		ProxyModeTunnel,
		ModeGlobal,
		nil,
		nil,
		false,
		false,
		0,
		false,
		nil,
		"",
	)
	if !result.Success {
		t.Fatalf("expected fallback success, got: %+v", result)
	}
	if !result.TunnelFailed {
		t.Fatalf("expected tunnel failure flag, got: %+v", result)
	}
	if !result.FallbackUsed {
		t.Fatalf("expected fallback flag, got: %+v", result)
	}
	if !strings.Contains(strings.ToLower(result.Reason), "access is denied") {
		t.Fatalf("expected reason to mention access denied, got: %q", result.Reason)
	}
	if result.ErrorCode != ConnectErrorTunPrivileges {
		t.Fatalf("expected tun privilege error code, got: %q", result.ErrorCode)
	}
}

func TestSetMode_ReconnectsWhenConnected(t *testing.T) {
	prev := isAdminCheck
	isAdminCheck = func() bool { return true }
	defer func() { isAdminCheck = prev }()

	host, port, closeFn := startReachableTCP(t)
	defer closeFn()

	log := logger.New()
	engine := &stubEngine{}
	sysProxy := &stubSystemProxy{}

	m := NewManager(log)
	m.engine = engine
	m.sysProxy = sysProxy

	connectRes := m.Connect(
		context.Background(),
		ProxyConfig{IP: host, Port: port, Type: "http"},
		ProxyModeProxy,
		ModeWhitelist,
		[]string{"localhost"},
		[]string{"notepad.exe"},
		true,
		true,
		0,
		false,
		nil,
		"",
	)
	if !connectRes.Success {
		t.Fatalf("initial connect failed: %+v", connectRes)
	}

	if err := m.SetMode(ProxyModeTunnel); err != nil {
		t.Fatalf("set mode failed: %v", err)
	}

	if len(engine.startCalls) < 2 {
		t.Fatalf("expected reconnect start call, got %d", len(engine.startCalls))
	}
	last := engine.startCalls[len(engine.startCalls)-1]
	if last.Mode != ProxyModeTunnel {
		t.Fatalf("expected reconnect in tunnel mode, got: %s", last.Mode)
	}
	if last.RoutingMode != ModeWhitelist {
		t.Fatalf("expected routing mode to be preserved, got: %s", last.RoutingMode)
	}
	if !last.KillSwitch || !last.AdBlock {
		t.Fatalf("expected feature flags to be preserved, got killSwitch=%v adblock=%v", last.KillSwitch, last.AdBlock)
	}
}

func TestConnect_TunnelRequiresAdmin(t *testing.T) {
	prev := isAdminCheck
	isAdminCheck = func() bool { return false }
	defer func() { isAdminCheck = prev }()

	log := logger.New()
	engine := &stubEngine{}
	m := NewManager(log)
	m.engine = engine

	res := m.Connect(
		context.Background(),
		ProxyConfig{IP: "127.0.0.1", Port: 1080, Type: "http"},
		ProxyModeTunnel,
		ModeGlobal,
		nil,
		nil,
		false,
		false,
		0,
		false,
		nil,
		"",
	)
	if res.Success {
		t.Fatalf("expected failure, got %+v", res)
	}
	if res.ErrorCode != ConnectErrorTunPrivileges {
		t.Fatalf("unexpected error code: %q", res.ErrorCode)
	}
	if len(engine.startCalls) != 0 {
		t.Fatalf("engine should not start, got calls=%d", len(engine.startCalls))
	}
}

func TestConnect_Hysteria2PostStartProbeFailure(t *testing.T) {
	prevAdmin := isAdminCheck
	prevHY2 := pingHysteria2Probe
	prevHTTPProxy := probeHTTPThroughProxyProbe
	isAdminCheck = func() bool { return true }
	pingHysteria2Probe = func(ip string, port int) (int64, bool, string, string) {
		return 0, false, "quic timeout", "quic"
	}
	// HTTP-проба тоже должна упасть: только сетевая ошибка убеждает нас что QUIC сервер недоступен.
	// Если HTTP-проба успешна — это уже другой сценарий (misconfiguration), не этот тест.
	probeHTTPThroughProxyProbe = func(string) (bool, string) { return false, "timeout" }
	defer func() {
		isAdminCheck = prevAdmin
		pingHysteria2Probe = prevHY2
		probeHTTPThroughProxyProbe = prevHTTPProxy
	}()

	log := logger.New()
	engine := &stubEngine{}
	m := NewManager(log)
	m.engine = engine

	res := m.Connect(
		context.Background(),
		ProxyConfig{IP: "1.2.3.4", Port: 443, Type: "hysteria2", Password: "p"},
		ProxyModeProxy,
		ModeGlobal,
		nil,
		nil,
		false,
		false,
		0,
		false,
		nil,
		"",
	)
	if res.Success {
		t.Fatalf("expected failure, got %+v", res)
	}
	if res.ErrorCode != "post_start_probe_failed" {
		t.Fatalf("unexpected code: %q", res.ErrorCode)
	}
	if engine.stopCalls == 0 {
		t.Fatal("expected engine stop on failed probe")
	}
}

func TestConnect_WireGuardTunnelFailsWhenE2EProbeFails(t *testing.T) {
	prevAdmin := isAdminCheck
	prevWG := pingWireGuardProbe
	prevHTTP := probeTunnelHTTPProbe
	isAdminCheck = func() bool { return true }
	pingWireGuardProbe = func(ip string, port int) (int64, bool, string) {
		return 5, true, ""
	}
	probeTunnelHTTPProbe = func() (bool, string) { return false, "timeout" }
	defer func() {
		isAdminCheck = prevAdmin
		pingWireGuardProbe = prevWG
		probeTunnelHTTPProbe = prevHTTP
	}()

	extra := `{"private_key":"a","public_key":"b","address":["10.0.0.2/32"],"allowed_ips":["0.0.0.0/0"]}`
	log := logger.New()
	engine := &stubEngine{}
	sysProxy := &stubSystemProxy{}
	m := NewManager(log)
	m.engine = engine
	m.sysProxy = sysProxy

	res := m.Connect(
		context.Background(),
		ProxyConfig{IP: "1.2.3.4", Port: 51820, Type: "wireguard", Extra: []byte(extra)},
		ProxyModeTunnel,
		ModeGlobal,
		nil,
		nil,
		false,
		false,
		0,
		false,
		nil,
		"",
	)
	if res.Success {
		t.Fatalf("expected failure when e2e probe fails, got %+v", res)
	}
	if res.ErrorCode != "post_start_probe_failed" {
		t.Fatalf("unexpected error code: %q", res.ErrorCode)
	}
	if engine.stopCalls == 0 {
		t.Fatal("expected engine stop on failed e2e probe")
	}
}

func TestConnect_WireGuardPostStartProbeSuccess(t *testing.T) {
	prevAdmin := isAdminCheck
	prevWG := pingWireGuardProbe
	prevHTTP := probeTunnelHTTPProbe
	isAdminCheck = func() bool { return true }
	pingWireGuardProbe = func(ip string, port int) (int64, bool, string) {
		return 5, true, ""
	}
	probeTunnelHTTPProbe = func() (bool, string) { return true, "" }
	defer func() {
		isAdminCheck = prevAdmin
		pingWireGuardProbe = prevWG
		probeTunnelHTTPProbe = prevHTTP
	}()

	extra := `{"private_key":"a","public_key":"b","address":["10.0.0.2/32"],"allowed_ips":["0.0.0.0/0"]}`
	log := logger.New()
	engine := &stubEngine{}
	sysProxy := &stubSystemProxy{}
	m := NewManager(log)
	m.engine = engine
	m.sysProxy = sysProxy

	res := m.Connect(
		context.Background(),
		ProxyConfig{IP: "1.2.3.4", Port: 51820, Type: "wireguard", Extra: []byte(extra)},
		ProxyModeTunnel,
		ModeGlobal,
		nil,
		nil,
		false,
		false,
		0,
		false,
		nil,
		"",
	)
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
}

func TestConnect_AmneziaWGTunnelFailsWhenE2EProbeFails(t *testing.T) {
	prevAdmin := isAdminCheck
	prevWG := pingWireGuardProbe
	prevHTTP := probeTunnelHTTPProbe
	isAdminCheck = func() bool { return true }
	pingWireGuardProbe = func(ip string, port int) (int64, bool, string) {
		return 5, true, ""
	}
	httpCalls := 0
	probeTunnelHTTPProbe = func() (bool, string) {
		httpCalls++
		return false, "timeout"
	}
	defer func() {
		isAdminCheck = prevAdmin
		pingWireGuardProbe = prevWG
		probeTunnelHTTPProbe = prevHTTP
	}()

	extra := `{"private_key":"a","public_key":"b","address":["10.0.0.2/32"],"allowed_ips":["0.0.0.0/0"],"amnezia":{"jc":3,"jmin":50,"jmax":1000,"s1":36,"s2":109,"h1":1129554205,"h2":1552545164,"h3":16997694,"h4":747701986}}`
	log := logger.New()
	engine := &stubEngine{}
	sysProxy := &stubSystemProxy{}
	m := NewManager(log)
	m.engine = engine
	m.sysProxy = sysProxy

	res := m.Connect(
		context.Background(),
		ProxyConfig{IP: "1.2.3.4", Port: 51820, Type: "amneziawg", Extra: []byte(extra)},
		ProxyModeTunnel,
		ModeGlobal,
		nil,
		nil,
		false,
		false,
		0,
		false,
		nil,
		"",
	)
	if res.Success {
		t.Fatalf("expected failure when e2e probe fails, got %+v", res)
	}
	if res.ErrorCode != "post_start_probe_failed" {
		t.Fatalf("unexpected error code: %q", res.ErrorCode)
	}
	if httpCalls != 4 {
		t.Fatalf("expected 4 http e2e probe attempts for amneziawg, got %d", httpCalls)
	}
}

func TestConnect_WireGuardTunnelE2EProbeRetriesThreeTimes(t *testing.T) {
	prevAdmin := isAdminCheck
	prevWG := pingWireGuardProbe
	prevHTTP := probeTunnelHTTPProbe
	isAdminCheck = func() bool { return true }
	pingWireGuardProbe = func(ip string, port int) (int64, bool, string) {
		return 5, true, ""
	}
	httpCalls := 0
	probeTunnelHTTPProbe = func() (bool, string) {
		httpCalls++
		return false, "timeout"
	}
	defer func() {
		isAdminCheck = prevAdmin
		pingWireGuardProbe = prevWG
		probeTunnelHTTPProbe = prevHTTP
	}()

	extra := `{"private_key":"a","public_key":"b","address":["10.0.0.2/32"],"allowed_ips":["0.0.0.0/0"]}`
	log := logger.New()
	engine := &stubEngine{}
	sysProxy := &stubSystemProxy{}
	m := NewManager(log)
	m.engine = engine
	m.sysProxy = sysProxy

	res := m.Connect(
		context.Background(),
		ProxyConfig{IP: "1.2.3.4", Port: 51820, Type: "wireguard", Extra: []byte(extra)},
		ProxyModeTunnel,
		ModeGlobal,
		nil,
		nil,
		false,
		false,
		0,
		false,
		nil,
		"",
	)
	if res.Success {
		t.Fatalf("expected failure, got %+v", res)
	}
	if httpCalls != 3 {
		t.Fatalf("expected 3 http probe attempts for wireguard, got %d", httpCalls)
	}
}

func TestConnect_TrojanTunnelFailsWhenE2EProbeFails(t *testing.T) {
	prevAdmin := isAdminCheck
	prevHTTP := probeTunnelHTTPProbe
	isAdminCheck = func() bool { return true }
	probeTunnelHTTPProbe = func() (bool, string) { return false, "timeout" }
	defer func() {
		isAdminCheck = prevAdmin
		probeTunnelHTTPProbe = prevHTTP
	}()

	host, port, closeFn := startReachableTCP(t)
	defer closeFn()

	log := logger.New()
	engine := &stubEngine{}
	m := NewManager(log)
	m.engine = engine

	res := m.Connect(
		context.Background(),
		ProxyConfig{IP: host, Port: port, Type: "trojan", Password: "x"},
		ProxyModeTunnel,
		ModeGlobal,
		nil,
		nil,
		false,
		false,
		0,
		false,
		nil,
		"",
	)
	if res.Success {
		t.Fatalf("expected failure when e2e probe fails, got %+v", res)
	}
	if res.ErrorCode != "post_start_probe_failed" {
		t.Fatalf("unexpected error code: %q", res.ErrorCode)
	}
	if engine.stopCalls == 0 {
		t.Fatal("expected engine stop on failed e2e probe")
	}
}

func TestConnect_TrojanProxyFailsWhenE2EProbeFails(t *testing.T) {
	prevAdmin := isAdminCheck
	prevHTTPProxy := probeHTTPThroughProxyProbe
	isAdminCheck = func() bool { return true }
	probeHTTPThroughProxyProbe = func(string) (bool, string) { return false, "timeout" }
	defer func() {
		isAdminCheck = prevAdmin
		probeHTTPThroughProxyProbe = prevHTTPProxy
	}()

	host, port, closeFn := startReachableTCP(t)
	defer closeFn()

	log := logger.New()
	engine := &stubEngine{}
	m := NewManager(log)
	m.engine = engine

	res := m.Connect(
		context.Background(),
		ProxyConfig{IP: host, Port: port, Type: "trojan", Password: "x"},
		ProxyModeProxy,
		ModeGlobal,
		nil,
		nil,
		false,
		false,
		0,
		false,
		nil,
		"",
	)
	if res.Success {
		t.Fatalf("expected failure when e2e proxy probe fails, got %+v", res)
	}
	if res.ErrorCode != "post_start_probe_failed" {
		t.Fatalf("unexpected error code: %q", res.ErrorCode)
	}
	if engine.stopCalls == 0 {
		t.Fatal("expected engine stop on failed proxy e2e probe")
	}
}

func TestConnect_AmneziaWGTunnelStopsSessionWhenE2EProbeFails(t *testing.T) {
	prevAdmin := isAdminCheck
	prevWG := pingWireGuardProbe
	prevHTTP := probeTunnelHTTPProbe
	isAdminCheck = func() bool { return true }
	pingWireGuardProbe = func(ip string, port int) (int64, bool, string) {
		return 5, true, ""
	}
	httpCalls := 0
	probeTunnelHTTPProbe = func() (bool, string) {
		httpCalls++
		return false, "timeout"
	}
	defer func() {
		isAdminCheck = prevAdmin
		pingWireGuardProbe = prevWG
		probeTunnelHTTPProbe = prevHTTP
	}()

	extra := `{"private_key":"a","public_key":"b","address":["10.0.0.2/32"],"allowed_ips":["0.0.0.0/0"],"amnezia":{"jc":3,"jmin":50,"jmax":1000,"s1":36,"s2":109,"h1":1129554205,"h2":1552545164,"h3":16997694,"h4":747701986}}`
	log := logger.New()
	engine := &stubEngine{}
	sysProxy := &stubSystemProxy{}
	m := NewManager(log)
	m.engine = engine
	m.sysProxy = sysProxy

	res := m.Connect(
		context.Background(),
		ProxyConfig{IP: "1.2.3.4", Port: 51820, Type: "amneziawg", Extra: []byte(extra)},
		ProxyModeTunnel,
		ModeGlobal,
		nil,
		nil,
		false,
		false,
		0,
		false,
		nil,
		"",
	)
	if res.Success {
		t.Fatalf("expected failure for amneziawg when e2e probe fails, got %+v", res)
	}
	if res.ErrorCode != "post_start_probe_failed" {
		t.Fatalf("unexpected error code: %q", res.ErrorCode)
	}
	if httpCalls != 4 {
		t.Fatalf("expected 4 http e2e probe attempts for amneziawg, got %d", httpCalls)
	}
	if engine.stopCalls == 0 {
		t.Fatalf("expected engine stop on failed e2e probe")
	}
}

func TestConnect_AmneziaWGTunnelClearsSystemProxy(t *testing.T) {
	prevAdmin := isAdminCheck
	prevWG := pingWireGuardProbe
	prevHTTP := probeTunnelHTTPProbe
	isAdminCheck = func() bool { return true }
	pingWireGuardProbe = func(ip string, port int) (int64, bool, string) {
		return 5, true, ""
	}
	probeTunnelHTTPProbe = func() (bool, string) { return true, "" }
	defer func() {
		isAdminCheck = prevAdmin
		pingWireGuardProbe = prevWG
		probeTunnelHTTPProbe = prevHTTP
	}()

	extra := `{"private_key":"a","public_key":"b","address":["10.0.0.2/32"],"allowed_ips":["0.0.0.0/0"],"amnezia":{"jc":3,"jmin":50}}`
	log := logger.New()
	engine := &stubEngine{}
	sysProxy := &stubSystemProxy{}
	m := NewManager(log)
	m.engine = engine
	m.sysProxy = sysProxy

	res := m.Connect(
		context.Background(),
		ProxyConfig{IP: "1.2.3.4", Port: 51820, Type: "amneziawg", Extra: []byte(extra)},
		ProxyModeTunnel,
		ModeGlobal,
		nil,
		nil,
		true,
		false,
		0,
		false,
		nil,
		"",
	)
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
	if sysProxy.disableCall == 0 {
		t.Fatalf("expected system proxy disable for amneziawg tunnel")
	}
}

func TestConnect_FailedSwitchClearsCurrentProxyInStatus(t *testing.T) {
	prevAdmin := isAdminCheck
	prevHTTPProxy := probeHTTPThroughProxyProbe
	isAdminCheck = func() bool { return true }
	probeHTTPThroughProxyProbe = func(string) (bool, string) { return true, "" }
	defer func() {
		isAdminCheck = prevAdmin
		probeHTTPThroughProxyProbe = prevHTTPProxy
	}()

	oldHost, oldPort, closeOld := startReachableTCP(t)
	defer closeOld()
	newHost, newPort, closeNew := startReachableTCP(t)
	defer closeNew()

	log := logger.New()
	engine := &stubEngine{}
	sysProxy := &stubSystemProxy{}
	m := NewManager(log)
	m.engine = engine
	m.sysProxy = sysProxy

	ok := m.Connect(
		context.Background(),
		ProxyConfig{IP: oldHost, Port: oldPort, Type: "trojan", Password: "x"},
		ProxyModeProxy,
		ModeGlobal,
		nil,
		nil,
		false,
		false,
		0,
		false,
		nil,
		"",
	)
	if !ok.Success {
		t.Fatalf("initial connect failed: %+v", ok)
	}

	engine.startErr = errors.New("start failed")
	res := m.Connect(
		context.Background(),
		ProxyConfig{IP: newHost, Port: newPort, Type: "trojan", Password: "x"},
		ProxyModeProxy,
		ModeGlobal,
		nil,
		nil,
		false,
		false,
		0,
		false,
		nil,
		"",
	)
	if res.Success {
		t.Fatalf("expected failed reconnect, got %+v", res)
	}

	status := m.GetStatus()
	if status.IsConnected {
		t.Fatalf("expected disconnected status, got %+v", status)
	}
	if status.CurrentProxy != nil {
		t.Fatalf("expected nil current proxy after failed switch, got %+v", status.CurrentProxy)
	}
}

func TestIsProbeHTTPStatusAcceptable(t *testing.T) {
	if !isProbeHTTPStatusAcceptable(204) {
		t.Fatal("expected 204 to be acceptable")
	}
	if !isProbeHTTPStatusAcceptable(400) {
		t.Fatal("expected 400 to be acceptable")
	}
	if isProbeHTTPStatusAcceptable(407) {
		t.Fatal("expected 407 to be rejected")
	}
	if isProbeHTTPStatusAcceptable(502) {
		t.Fatal("expected 502 to be rejected by direct probe")
	}
}

func TestIsProxyProbeResponseAcceptable(t *testing.T) {
	// Через прокси: 502/503/504 от CDN — нормально, туннель работает
	if !isProxyProbeResponseAcceptable(204) {
		t.Fatal("expected 204 to be acceptable via proxy")
	}
	if !isProxyProbeResponseAcceptable(502) {
		t.Fatal("expected 502 to be acceptable via proxy (CDN returned error, tunnel works)")
	}
	if !isProxyProbeResponseAcceptable(503) {
		t.Fatal("expected 503 to be acceptable via proxy")
	}
	if !isProxyProbeResponseAcceptable(200) {
		t.Fatal("expected 200 to be acceptable via proxy")
	}
	// 407 = прокси сам не принял запрос
	if isProxyProbeResponseAcceptable(407) {
		t.Fatal("expected 407 to be rejected (proxy auth required)")
	}
}
