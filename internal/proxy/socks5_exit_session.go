package proxy

import (
	"fmt"

	"resultproxy-wails/internal/config"
)

func (m *Manager) SetSocks5Exit(profile *config.Socks5ExitConfig) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()

	if profile == nil {
		profile = &config.Socks5ExitConfig{}
	}
	if m.connected {
		if m.engine != nil && m.engine.IsRunning() {
			engine, ok := m.engine.(interface {
				SetSocks5Exit(*config.Socks5ExitConfig) error
			})
			if !ok {
				return errSocks5ExitUnavailable
			}
			if err := engine.SetSocks5Exit(profile); err != nil {
				return err
			}
		} else if profile.IsActive() {
			return errSocks5ExitUnavailable
		}
	}
	m.socks5Exit = profile
	return nil
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
