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
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"resultproxy-wails/internal/logger"
)

type stubEngine struct {
	startErr   error
	startCalls []EngineConfig
	stopCalls  int
	running    bool
	applyCalls [][]string
	applyErr   error
	proxyUp    int64
	proxyDown  int64
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

func (s *stubEngine) GetProxyTrafficStats() (up, down int64) {
	return s.proxyUp, s.proxyDown
}

func (s *stubEngine) ApplyAppWhitelist(paths []string) error {
	s.applyCalls = append(s.applyCalls, append([]string(nil), paths...))
	return s.applyErr
}

type stubSystemProxy struct {
	setCalls    []string
	disableCall int
	leftover    bool
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
func (s *stubSystemProxy) LeftoverActive() bool   { return s.leftover }

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
	prevDelay := tunRetryDelay
	tunRetryDelay = 0
	defer func() { tunRetryDelay = prevDelay }()

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
		nil,
		false,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
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
	// Elevated already, so a TUN setup failure must NOT be reported as a
	// privilege problem (that would pop a futile UAC prompt) — it is downgraded
	// to a generic engine-start error while the reason still names the cause.
	if result.ErrorCode != ConnectErrorEngineStart {
		t.Fatalf("expected engine-start error code for elevated user, got: %q", result.ErrorCode)
	}
}

func TestSetMode_ReconnectsWhenConnected(t *testing.T) {
	prev := isAdminCheck
	isAdminCheck = func() bool { return true }
	defer func() { isAdminCheck = prev }()

	// The tunnel reconnect runs the post-start tunnel probe; stub it so the
	// test is hermetic instead of dialing the real loopback probe listener
	// (no engine actually serves it under stubEngine).
	prevTunnelProbe := probeHTTPThroughProxyProbe
	probeHTTPThroughProxyProbe = func(string) (bool, string) { return true, "" }
	defer func() { probeHTTPThroughProxyProbe = prevTunnelProbe }()

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
		nil,
		true,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
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
	if !last.KillSwitch {
		t.Fatalf("expected feature flags to be preserved, got killSwitch=%v", last.KillSwitch)
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
		nil,
		false,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
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
		nil,
		false,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
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
	prevHTTP := probeHTTPThroughProxyProbe
	isAdminCheck = func() bool { return true }
	pingWireGuardProbe = func(ip string, port int) (int64, bool, string) {
		return 5, true, ""
	}
	probeHTTPThroughProxyProbe = func(string) (bool, string) { return false, "timeout" }
	defer func() {
		isAdminCheck = prevAdmin
		pingWireGuardProbe = prevWG
		probeHTTPThroughProxyProbe = prevHTTP
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
		nil,
		false,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
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
	prevHTTP := probeHTTPThroughProxyProbe
	isAdminCheck = func() bool { return true }
	pingWireGuardProbe = func(ip string, port int) (int64, bool, string) {
		return 5, true, ""
	}
	probeHTTPThroughProxyProbe = func(string) (bool, string) { return true, "" }
	defer func() {
		isAdminCheck = prevAdmin
		pingWireGuardProbe = prevWG
		probeHTTPThroughProxyProbe = prevHTTP
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
		nil,
		false,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
	)
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
}

func TestConnect_AmneziaWGTunnelFailsWhenE2EProbeFails(t *testing.T) {
	prevAdmin := isAdminCheck
	prevWG := pingWireGuardProbe
	prevHTTP := probeHTTPThroughProxyProbe
	isAdminCheck = func() bool { return true }
	pingWireGuardProbe = func(ip string, port int) (int64, bool, string) {
		return 5, true, ""
	}
	httpCalls := 0
	probeHTTPThroughProxyProbe = func(string) (bool, string) {
		httpCalls++
		return false, "timeout"
	}
	defer func() {
		isAdminCheck = prevAdmin
		pingWireGuardProbe = prevWG
		probeHTTPThroughProxyProbe = prevHTTP
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
		nil,
		false,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
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
	prevHTTP := probeHTTPThroughProxyProbe
	isAdminCheck = func() bool { return true }
	pingWireGuardProbe = func(ip string, port int) (int64, bool, string) {
		return 5, true, ""
	}
	httpCalls := 0
	probeHTTPThroughProxyProbe = func(string) (bool, string) {
		httpCalls++
		return false, "timeout"
	}
	defer func() {
		isAdminCheck = prevAdmin
		pingWireGuardProbe = prevWG
		probeHTTPThroughProxyProbe = prevHTTP
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
		nil,
		false,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
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
	prevHTTP := probeHTTPThroughProxyProbe
	isAdminCheck = func() bool { return true }
	probeHTTPThroughProxyProbe = func(string) (bool, string) { return false, "timeout" }
	defer func() {
		isAdminCheck = prevAdmin
		probeHTTPThroughProxyProbe = prevHTTP
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
		nil,
		false,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
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
		nil,
		false,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
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
	prevHTTP := probeHTTPThroughProxyProbe
	isAdminCheck = func() bool { return true }
	pingWireGuardProbe = func(ip string, port int) (int64, bool, string) {
		return 5, true, ""
	}
	httpCalls := 0
	probeHTTPThroughProxyProbe = func(string) (bool, string) {
		httpCalls++
		return false, "timeout"
	}
	defer func() {
		isAdminCheck = prevAdmin
		pingWireGuardProbe = prevWG
		probeHTTPThroughProxyProbe = prevHTTP
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
		nil,
		false,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
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
	prevHTTP := probeHTTPThroughProxyProbe
	isAdminCheck = func() bool { return true }
	pingWireGuardProbe = func(ip string, port int) (int64, bool, string) {
		return 5, true, ""
	}
	probeHTTPThroughProxyProbe = func(string) (bool, string) { return true, "" }
	defer func() {
		isAdminCheck = prevAdmin
		pingWireGuardProbe = prevWG
		probeHTTPThroughProxyProbe = prevHTTP
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
		nil,
		true,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
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
		nil,
		false,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
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
		nil,
		false,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
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

// Проба принимает только тот ответ, который может дать настоящий эндпоинт.
//
// Прежнее правило («любой HTTP-ответ через прокси означает, что туннель
// работает, 5xx от CDN — норма») было дырой в форме проверки: пробы ходят
// обычным http, sing такие запросы не пробрасывает, а на любой ошибке
// аутбаунда сам пишет клиенту «502 Bad Gateway» без заголовков и тела
// (sing/protocol/http/handshake.go). Мёртвый аутбаунд отвечал валидным HTTP —
// мгновенно и изнутри этой же машины, — и подключение объявлялось успешным
// (probe=2ms в логах 05.09.2026). Ожидаемый статус ловит и подделку, и
// captive portal, и подменную страницу: ни одна из них не выдаст 204.
func TestProbeResponseMatches(t *testing.T) {
	gen204 := probeTarget{url: "http://example.test/generate_204", wantStatus: http.StatusNoContent}
	withBody := probeTarget{url: "http://example.test/connecttest.txt", wantStatus: http.StatusOK, wantBody: "Microsoft Connect Test"}

	resp := func(code int, body string) *http.Response {
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body))}
	}

	if ok, reason := probeResponseMatches(gen204, resp(204, "")); !ok {
		t.Fatalf("204 от generate_204 — ровно то, что нужно, получили отказ: %s", reason)
	}
	for _, code := range []int{200, 302, 403, 500, 502, 503} {
		if ok, _ := probeResponseMatches(gen204, resp(code, "")); ok {
			t.Errorf("статус %d вместо 204 не доказывает, что запрос дошёл до эндпоинта", code)
		}
	}
	if ok, _ := probeResponseMatches(gen204, resp(407, "")); ok {
		t.Error("407 означает, что запрос отверг сам локальный прокси")
	}
	if ok, reason := probeResponseMatches(withBody, resp(200, "Microsoft Connect Test")); !ok {
		t.Fatalf("правильный ответ connecttest.txt отвергнут: %s", reason)
	}
	if ok, _ := probeResponseMatches(withBody, resp(200, "<html>Вход в сеть отеля</html>")); ok {
		t.Error("200 с чужим телом — это подмена, а не рабочий туннель")
	}
}

func TestSetMode_PreservesAppForceVPN(t *testing.T) {
	prev := isAdminCheck
	isAdminCheck = func() bool { return true }
	defer func() { isAdminCheck = prev }()

	prevTunnelProbe := probeHTTPThroughProxyProbe
	probeHTTPThroughProxyProbe = func(string) (bool, string) { return true, "" }
	defer func() { probeHTTPThroughProxyProbe = prevTunnelProbe }()

	host, port, closeFn := startReachableTCP(t)
	defer closeFn()

	log := logger.New()
	engine := &stubEngine{}
	m := NewManager(log)
	m.engine = engine
	m.sysProxy = &stubSystemProxy{}

	connectRes := m.Connect(
		context.Background(),
		ProxyConfig{IP: host, Port: port, Type: "http"},
		ProxyModeProxy,
		ModeSmart,
		nil,
		nil,
		[]string{"discord.exe"},
		false,
		0,
		false,
		nil,
		"",
		"",
		false,
		false,
	)
	if !connectRes.Success {
		t.Fatalf("initial connect failed: %+v", connectRes)
	}

	if err := m.SetMode(ProxyModeTunnel); err != nil {
		t.Fatalf("set mode failed: %v", err)
	}

	last := engine.startCalls[len(engine.startCalls)-1]
	if len(last.AppForceVPN) != 1 || last.AppForceVPN[0] != "discord.exe" {
		t.Fatalf("appForceVPN must survive SetMode reconnect, got %v", last.AppForceVPN)
	}
}

// TestBasenameRoots_DropsPathQualifiedEntries guards the widening hazard:
// processtree.normalizeRoots strips a root to its basename, so feeding it a
// path-qualified entry would collapse "Battle.net\Agent\Agent.exe" back to a
// bare "agent.exe" and re-open the ambiguity the path form exists to close.
func TestBasenameRoots_DropsPathQualifiedEntries(t *testing.T) {
	got := basenameRoots([]string{
		"Battle.net.exe",
		`Battle.net\Agent\Agent.exe`,
		"Wow.exe",
		"some/dir/thing.exe",
	})
	want := []string{"Battle.net.exe", "Wow.exe"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
