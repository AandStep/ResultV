package proxy

import (
	"errors"
	"fmt"

	"resultproxy-wails/internal/config"
)

func (m *Manager) SetSocks5Exit(profile *config.Socks5ExitConfig) error {
	if profile == nil {
		profile = &config.Socks5ExitConfig{}
	}
	previous, reconnect, err := m.applySocks5Exit(profile)
	if err != nil || !reconnect {
		return err
	}
	if res := m.reload(m.ctx, func(*reloadSession) {}); !res.Success && res.ErrorCode != ConnectErrorSuperseded {
		m.mu.Lock()
		m.socks5Exit = previous
		m.mu.Unlock()
		return errors.New(res.Message)
	}
	return nil
}

// A session built without the SOCKS5 route has no selector to switch.
func socks5ExitNeedsReconnect(connected, wired bool, profile *config.Socks5ExitConfig) bool {
	return connected && !wired && socks5ExitWired(profile)
}

func (m *Manager) applySocks5Exit(profile *config.Socks5ExitConfig) (*config.Socks5ExitConfig, bool, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()

	previous := m.socks5Exit
	if socks5ExitNeedsReconnect(m.connected, m.socks5ExitWired, profile) {
		m.socks5Exit = profile
		return previous, true, nil
	}
	if m.connected && m.socks5ExitWired {
		if m.engine != nil && m.engine.IsRunning() {
			engine, ok := m.engine.(interface {
				SetSocks5Exit(*config.Socks5ExitConfig) error
			})
			if !ok {
				return previous, false, errSocks5ExitUnavailable
			}
			if err := engine.SetSocks5Exit(profile); err != nil {
				return previous, false, err
			}
		} else if profile.IsActive() {
			return previous, false, errSocks5ExitUnavailable
		}
	}
	m.socks5Exit = profile
	return previous, false, nil
}

func probePortForSession(localPort, firstHopProbePort int) int {
	if firstHopProbePort != 0 {
		return firstHopProbePort
	}
	return localPort
}

// Startup verification must test the VPN node independently of the SOCKS5 exit.
func (m *Manager) Socks5ExitFirstHopProbeAddress() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected || !m.socks5Exit.IsActive() || m.localPort == 0 {
		return ""
	}
	return fmt.Sprintf("127.0.0.1:%d", probePortForSession(m.localPort, m.firstHopProbePort))
}
