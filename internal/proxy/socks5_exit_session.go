package proxy

import (
	"errors"

	"resultproxy-wails/internal/config"
)

const ConnectErrorSocks5ExitActive = "socks5_exit_active"

var ErrSocks5ExitActive = errors.New("выключите SOCKS5 через VPN перед сменой узла или режима")

type Socks5ExitState struct {
	Available bool `json:"available"`
	Enabled   bool `json:"enabled"`
}

type socks5ExitSwitcher interface {
	SetSocks5ExitEnabled(enabled bool) error
}

func socks5ExitActiveResult() ConnectResultDTO {
	return ConnectResultDTO{Message: ErrSocks5ExitActive.Error(), ErrorCode: ConnectErrorSocks5ExitActive}
}

func (m *Manager) Socks5ExitState() Socks5ExitState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.socks5ExitStateLocked()
}

func (m *Manager) CurrentSocks5ExitProfile() *config.Socks5ExitConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.socks5Exit
}

func (m *Manager) socks5ExitStateLocked() Socks5ExitState {
	_, canSwitch := m.engine.(socks5ExitSwitcher)
	return Socks5ExitState{
		Available: canSwitch && m.connected && m.proxy != nil && m.engine.IsRunning() &&
			supportsSocks5Exit(m.socks5Exit, m.mode, m.routingMode, m.proxy.Type),
		Enabled: m.socks5ExitEnabled,
	}
}

func (m *Manager) SetSocks5ExitEnabled(enabled bool, beforeEnable func() error) (Socks5ExitState, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.socks5ExitStateLocked()
	if !state.Available && enabled {
		return state, errSocks5ExitUnsupported
	}
	if enabled == state.Enabled {
		return state, nil
	}
	if enabled && beforeEnable != nil {
		if err := beforeEnable(); err != nil {
			return state, err
		}
	}
	if state.Available {
		if err := m.engine.(socks5ExitSwitcher).SetSocks5ExitEnabled(enabled); err != nil {
			return state, err
		}
	}
	m.socks5ExitEnabled = enabled
	m.emitStatusLocked()
	return m.socks5ExitStateLocked(), nil
}
