package main

import (
	"errors"
	"runtime"
	"strings"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

type Socks5ExitSettingsDTO struct {
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	Username    string   `json:"username"`
	Domains     []string `json:"domains"`
	HasPassword bool     `json:"hasPassword"`
}

type Socks5ExitSettingsUpdate struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Username string   `json:"username"`
	Password *string  `json:"password"`
	Domains  []string `json:"domains"`
}

func appConfigForUI(cfg config.AppConfig) config.AppConfig {
	cfg.Settings.Socks5Exit = nil
	cfg.Settings.Socks5ExitArmed = false
	return cfg
}

func socks5ExitProfileFor(cfg config.AppConfig, selectedID string) *config.Socks5ExitConfig {
	if runtime.GOOS != "windows" || cfg.Settings.Socks5Exit == nil || selectedID == "" {
		return nil
	}
	for _, node := range cfg.Proxies {
		if node.ID == selectedID {
			if strings.EqualFold(node.Type, "vless") {
				return cfg.Settings.Socks5Exit
			}
			return nil
		}
	}
	return nil
}

func (a *App) SetSocks5ExitEnabled(enabled bool) (proxy.Socks5ExitState, error) {
	if a.proxy == nil || a.config == nil {
		return proxy.Socks5ExitState{}, errors.New("proxy manager not initialized")
	}
	a.socks5ExitMu.Lock()
	defer a.socks5ExitMu.Unlock()
	state, err := a.proxy.SetSocks5ExitEnabled(enabled, func() error {
		return a.config.SetSocks5ExitArmed(true)
	})
	if !enabled && err == nil {
		err = errors.Join(err, a.config.SetSocks5ExitArmed(false))
	}
	return state, err
}

func (a *App) GetSocks5ExitSettings() (Socks5ExitSettingsDTO, error) {
	if a.config == nil {
		return Socks5ExitSettingsDTO{}, errors.New("config manager not initialized")
	}
	p := a.config.GetConfig().Settings.Socks5Exit
	if p == nil {
		return Socks5ExitSettingsDTO{}, nil
	}
	return Socks5ExitSettingsDTO{
		Host: p.Host, Port: p.Port, Username: p.Username,
		Domains: append([]string(nil), p.Domains...), HasPassword: p.Password != "",
	}, nil
}

func (a *App) SaveSocks5ExitSettings(update Socks5ExitSettingsUpdate) error {
	if a.config == nil {
		return errors.New("config manager not initialized")
	}
	if a.proxy != nil {
		connected, establishing := a.proxy.SessionState()
		if connected || establishing || a.proxy.Socks5ExitState().Enabled {
			return errors.New("disconnect the VPN before editing SOCKS5 settings")
		}
	}
	return a.config.SaveSocks5Exit(config.Socks5ExitConfig{
		Host: update.Host, Port: update.Port, Username: update.Username, Domains: update.Domains,
	}, update.Password)
}
