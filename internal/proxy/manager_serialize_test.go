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
	"sync"
	"testing"
	"time"

	"resultproxy-wails/internal/logger"
)

// serialGuardEngine mirrors the real engine's single-instance guard: Start
// errors with "engine already running" if a previous Start was not balanced by
// a Stop. It records whether that guard ever fired so a test can assert two
// connect operations never overlapped on the engine.
type serialGuardEngine struct {
	mu      sync.Mutex
	running bool
	already bool
}

func (e *serialGuardEngine) Start(_ context.Context, _ EngineConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		e.already = true
		return errors.New("engine already running")
	}
	e.running = true
	return nil
}

func (e *serialGuardEngine) Stop() error {
	e.mu.Lock()
	e.running = false
	e.mu.Unlock()
	return nil
}

func (e *serialGuardEngine) IsRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

func (e *serialGuardEngine) GetTrafficStats() (up, down int64)      { return 0, 0 }
func (e *serialGuardEngine) GetProxyTrafficStats() (up, down int64) { return 0, 0 }
func (e *serialGuardEngine) ApplyAppWhitelist([]string) error       { return nil }

func (e *serialGuardEngine) sawAlreadyRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.already
}

// TestConnect_SerializesConcurrentConnects reproduces the hot-reload race: two
// connect operations overlapping on the single engine. The frontend fires them
// from independent React effects (selectAndConnect + the routing-rules/adblock
// reconnect), so the backend must serialize them itself.
//
// We park the first Connect inside its post-start probe (engine already
// "running"), then launch a second Connect. Without serialization the second
// reaches engine.Start while the first is in-flight and trips the real
// "engine already running" guard — the exact failure seen in the field logs.
// With opMu the second blocks until the first finishes, so both succeed and the
// guard never fires.
func TestConnect_SerializesConcurrentConnects(t *testing.T) {
	prevAdmin := isAdminCheck
	prevProbe := probeHTTPThroughProxyProbe
	isAdminCheck = func() bool { return true }

	entered := make(chan struct{})
	proceed := make(chan struct{})
	var once sync.Once
	probeHTTPThroughProxyProbe = func(string) (bool, string) {
		once.Do(func() { close(entered) })
		<-proceed
		return true, ""
	}
	defer func() {
		isAdminCheck = prevAdmin
		probeHTTPThroughProxyProbe = prevProbe
	}()

	log := logger.New()
	engine := &serialGuardEngine{}
	m := NewManager(log)
	m.engine = engine
	m.sysProxy = &stubSystemProxy{}

	connect := func() ConnectResultDTO {
		return m.Connect(
			context.Background(),
			ProxyConfig{IP: "10.0.0.1", Port: 1080, Type: "trojan", Password: "x"},
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
	}

	resA := make(chan ConnectResultDTO, 1)
	go func() { resA <- connect() }()

	// First Connect is now parked inside its probe, holding opMu with the engine
	// running.
	<-entered

	resB := make(chan ConnectResultDTO, 1)
	go func() { resB <- connect() }()

	// Give a non-serialized second Connect time to reach engine.Start and trip
	// the guard. With opMu it stays blocked on the mutex instead.
	time.Sleep(150 * time.Millisecond)
	if engine.sawAlreadyRunning() {
		t.Fatal("second Connect reached engine.Start while the first was in-flight — operations not serialized")
	}

	close(proceed)

	rA := <-resA
	rB := <-resB
	if !rA.Success || !rB.Success {
		t.Fatalf("both connects should succeed once serialized: A=%+v B=%+v", rA, rB)
	}
	if engine.sawAlreadyRunning() {
		t.Fatal("engine reported 'already running' — connect operations overlapped")
	}
}
