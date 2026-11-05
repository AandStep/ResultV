package proxy

import (
	"context"
	"errors"
	"testing"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/logger"
)

type testSocks5SessionEngine struct {
	serialGuardEngine
	setCalls int
	selected bool
	fail     bool
}

func (e *testSocks5SessionEngine) SetSocks5ExitEnabled(enabled bool) error {
	e.setCalls++
	if e.fail {
		return errors.New("selector unavailable")
	}
	e.selected = enabled
	return nil
}

func TestSocks5ExitSessionLifecycle(t *testing.T) {
	engine := &testSocks5SessionEngine{}
	if err := engine.Start(context.Background(), EngineConfig{}); err != nil {
		t.Fatal(err)
	}
	m := NewManager(logger.New())
	m.engine = engine
	m.connected = true
	m.proxy = &ProxyConfig{Type: "VLESS", IP: "192.0.2.1", Port: 443}
	m.mode, m.routingMode = ProxyModeTunnel, ModeGlobal
	m.socks5Exit = &config.Socks5ExitConfig{Host: "exit.example.test", Port: 8000, Domains: []string{"target.example.test"}}

	if got := m.Socks5ExitState(); !got.Available || got.Enabled {
		t.Fatalf("unexpected initial exit state: %+v", got)
	}
	if state, err := m.SetSocks5ExitEnabled(true, func() error { return errors.New("write failed") }); err == nil || state.Enabled || engine.selected {
		t.Fatal("selector enabled without a durable recovery marker")
	}
	if state, err := m.SetSocks5ExitEnabled(true, nil); err != nil || !state.Enabled || !engine.selected {
		t.Fatalf("unable to switch exit: state=%+v err=%v", state, err)
	}
	result := m.Connect(context.Background(), *m.proxy, ProxyModeProxy, ModeGlobal, nil, nil, nil, false, 0, false, nil, "", "", false, false)
	if result.ErrorCode != ConnectErrorSocks5ExitActive || !engine.IsRunning() {
		t.Fatal("a manual connect replaced the armed session")
	}
	if err := m.DisconnectForServerChange(); !errors.Is(err, ErrSocks5ExitActive) || !engine.IsRunning() {
		t.Fatal("preparing a server change tore down the armed session")
	}
	result = m.ReconnectWithRoutingRules(context.Background(), ModeSmart, nil, nil, nil)
	if result.ErrorCode != ConnectErrorSocks5ExitActive || !engine.IsRunning() {
		t.Fatal("changing the routing mode replaced the armed session")
	}
	engine.fail = true
	if state, err := m.SetSocks5ExitEnabled(false, nil); err == nil || !state.Enabled || !engine.selected {
		t.Fatal("a failed toggle changed the confirmed state")
	}
	engine.fail = false
	if state, err := m.SetSocks5ExitEnabled(false, nil); err != nil || state.Enabled || engine.selected || !engine.IsRunning() {
		t.Fatal("disarming failed or stopped the engine")
	}
	if engine.setCalls != 3 {
		t.Fatalf("selector calls = %d, want 3", engine.setCalls)
	}
	if err := m.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if state, err := m.SetSocks5ExitEnabled(true, nil); err == nil || state.Enabled || state.Available {
		t.Fatal("toggle applied to a disconnected session")
	}
}
