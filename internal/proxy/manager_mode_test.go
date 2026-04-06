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
}

func TestSetMode_ReconnectsWhenConnected(t *testing.T) {
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
