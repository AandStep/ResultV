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
	m.socks5ExitWired = true
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

func TestSocks5ExitUnwiredSession(t *testing.T) {
	engine := &testSocks5SessionEngine{}
	m := NewManager(logger.New())
	m.engine = engine
	if err := engine.Start(context.Background(), EngineConfig{}); err != nil {
		t.Fatal(err)
	}
	m.connected = true
	pending := &config.Socks5ExitConfig{Enabled: true}
	if err := m.SetSocks5Exit(pending); err != nil || m.socks5Exit != pending || engine.profile != nil {
		t.Fatal("a profile without an address touched a session built without the SOCKS5 route")
	}
}

func TestSocks5ExitNeedsReconnect(t *testing.T) {
	active := &config.Socks5ExitConfig{Enabled: true, Host: "exit.example.test", Port: 8000}
	listed := &config.Socks5ExitConfig{Enabled: true, Host: "exit.example.test", Port: 8000, Sites: []string{"example.test"}}
	listedOff := &config.Socks5ExitConfig{Host: "exit.example.test", Port: 8000, Sites: []string{"example.test"}}
	addressedOff := &config.Socks5ExitConfig{Host: "exit.example.test", Port: 8000}
	for _, tc := range []struct {
		name             string
		connected, wired bool
		built, next      *config.Socks5ExitConfig
		want             bool
	}{
		{"not connected", false, false, nil, active, false},
		{"enable on a plain session", true, false, nil, active, true},
		{"live edit, same lists", true, true, active, active, false},
		{"lists changed", true, true, active, listed, true},
		{"disabled with other lists", true, true, active, listedOff, false},
		{"saved address while off", true, false, nil, addressedOff, false},
		{"no profile", true, false, nil, nil, false},
	} {
		if got := socks5ExitNeedsReconnect(tc.connected, tc.wired, tc.built, tc.next); got != tc.want {
			t.Errorf("%s: reconnect=%t, want %t", tc.name, got, tc.want)
		}
	}
}
