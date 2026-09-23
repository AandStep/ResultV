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
	"sync/atomic"
	"testing"
	"time"

	"resultproxy-wails/internal/logger"
)

// reloadHarness is a manager with a live session whose post-start probe can be
// switched to hang, so a reload can be parked mid-connect.
type reloadHarness struct {
	m      *Manager
	engine *serialGuardEngine
	hang   atomic.Bool
}

func newReloadHarness(t *testing.T) *reloadHarness {
	t.Helper()
	prevAdmin, prevProbe := isAdminCheck, probeHTTPThroughProxyProbe
	t.Cleanup(func() { isAdminCheck, probeHTTPThroughProxyProbe = prevAdmin, prevProbe })
	isAdminCheck = func() bool { return true }

	h := &reloadHarness{engine: &serialGuardEngine{}}
	probeHTTPThroughProxyProbe = func(string) (bool, string) {
		if h.hang.Load() {
			time.Sleep(3 * time.Second)
			return false, "timeout"
		}
		return true, ""
	}

	h.m = NewManager(logger.New())
	h.m.engine = h.engine
	h.m.sysProxy = &stubSystemProxy{}
	res := h.m.Connect(context.Background(),
		ProxyConfig{IP: "10.0.0.1", Port: 1080, Type: "trojan", Password: "x"},
		ProxyModeProxy, ModeGlobal, nil, nil, nil, false, 0, false, nil, "", "", false, false)
	if !res.Success {
		t.Fatalf("initial connect failed: %+v", res)
	}
	return h
}

func (h *reloadHarness) reload(mode RoutingMode) <-chan ConnectResultDTO {
	ch := make(chan ConnectResultDTO, 1)
	go func() {
		ch <- h.m.ReconnectWithRoutingRules(context.Background(), mode, nil, nil, nil)
	}()
	return ch
}

func waitResult(t *testing.T, ch <-chan ConnectResultDTO, within time.Duration) ConnectResultDTO {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(within):
		t.Fatalf("no result within %v", within)
		return ConnectResultDTO{}
	}
}

// Три быстрых перезагрузки применяют последние правила один раз, а не
// переподключаются трижды друг за другом.
func TestReconnectWithRoutingRules_NewestWins(t *testing.T) {
	h := newReloadHarness(t)
	h.hang.Store(true)

	first := h.reload(ModeSmart)
	time.Sleep(100 * time.Millisecond)
	second := h.reload(ModeGlobal)
	time.Sleep(50 * time.Millisecond)
	h.hang.Store(false)
	third := h.reload(ModeSmart)

	r1 := waitResult(t, first, time.Second)
	r2 := waitResult(t, second, time.Second)
	r3 := waitResult(t, third, 2*time.Second)

	if r1.ErrorCode != ConnectErrorSuperseded || r2.ErrorCode != ConnectErrorSuperseded {
		t.Fatalf("older reloads must be superseded: r1=%+v r2=%+v", r1, r2)
	}
	if !r3.Success {
		t.Fatalf("newest reload must apply: %+v", r3)
	}
	if connected, _ := h.m.SessionState(); !connected {
		t.Fatal("session must be up after the newest reload")
	}
	h.m.mu.Lock()
	got := h.m.routingMode
	h.m.mu.Unlock()
	if got != ModeSmart {
		t.Fatalf("newest routing mode must win, got %v", got)
	}
}

// Отключение посреди зависшей перезагрузки не ждёт её пробу.
func TestDisconnect_DuringHangingReloadIsFast(t *testing.T) {
	h := newReloadHarness(t)
	h.hang.Store(true)

	res := h.reload(ModeSmart)
	time.Sleep(100 * time.Millisecond)

	start := time.Now()
	if err := h.m.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("disconnect waited for the reload probe: %v", elapsed)
	}
	r := waitResult(t, res, time.Second)
	if r.Success {
		t.Fatalf("reload interrupted by disconnect must not report success: %+v", r)
	}
	if connected, _ := h.m.SessionState(); connected {
		t.Fatal("session must stay down after disconnect")
	}

	// Очередь перезагрузок не поднимает сессию после отключения.
	late := waitResult(t, h.reload(ModeGlobal), time.Second)
	if connected, _ := h.m.SessionState(); connected {
		t.Fatalf("a reload after disconnect must not reconnect: %+v", late)
	}
}

// Остановка обычного подключения, зависшего в пробе, тоже не ждёт пробу.
func TestDisconnect_DuringHangingConnectIsFast(t *testing.T) {
	prevAdmin, prevProbe := isAdminCheck, probeHTTPThroughProxyProbe
	t.Cleanup(func() { isAdminCheck, probeHTTPThroughProxyProbe = prevAdmin, prevProbe })
	isAdminCheck = func() bool { return true }
	probeHTTPThroughProxyProbe = func(string) (bool, string) {
		time.Sleep(3 * time.Second)
		return false, "timeout"
	}

	m := NewManager(logger.New())
	m.engine = &serialGuardEngine{}
	m.sysProxy = &stubSystemProxy{}
	res := make(chan ConnectResultDTO, 1)
	go func() {
		res <- m.Connect(context.Background(),
			ProxyConfig{IP: "10.0.0.1", Port: 1080, Type: "trojan", Password: "x"},
			ProxyModeProxy, ModeGlobal, nil, nil, nil, false, 0, false, nil, "", "", false, false)
	}()
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	m.CancelConnect()
	if err := m.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("disconnect waited for the connect probe: %v", elapsed)
	}
	if r := waitResult(t, res, time.Second); r.ErrorCode != "cancelled" {
		t.Fatalf("expected cancelled, got %+v", r)
	}
}
