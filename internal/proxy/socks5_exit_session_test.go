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
	profile *config.Socks5ExitConfig
	fail    bool
}

func (e *testSocks5SessionEngine) SetSocks5Exit(profile *config.Socks5ExitConfig) error {
	if e.fail {
		return errors.New("selector unavailable")
	}
	e.profile = profile
	return nil
}

func TestSocks5ExitSessionLifecycle(t *testing.T) {
	engine := &testSocks5SessionEngine{}
	m := NewManager(logger.New())
	m.engine = engine
	pending := &config.Socks5ExitConfig{Enabled: true}
	if err := m.SetSocks5Exit(pending); err != nil || m.socks5Exit != pending || engine.IsRunning() {
		t.Fatal("the switch should work before connecting")
	}
	if err := engine.Start(context.Background(), EngineConfig{}); err != nil {
		t.Fatal(err)
	}
	m.connected = true
	profile := &config.Socks5ExitConfig{Enabled: true, Host: "exit.example.test", Port: 8000}
	if err := m.SetSocks5Exit(profile); err != nil || engine.profile != profile || !engine.IsRunning() {
		t.Fatal("editing the live profile stopped the engine or lost the profile")
	}
	engine.fail = true
	if err := m.SetSocks5Exit(nil); err == nil || m.socks5Exit != profile {
		t.Fatal("a failed update replaced the active profile")
	}
	if err := m.Disconnect(); err != nil || m.socks5Exit != profile {
		t.Fatal("disconnect cleared the SOCKS5 preference")
	}
}
