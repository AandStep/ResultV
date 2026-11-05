package main

import (
	"errors"

	"resultproxy-wails/internal/config"
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
	return cfg
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
		if connected || establishing {
			return errors.New("disconnect the VPN before editing SOCKS5 settings")
		}
	}
	return a.config.SaveSocks5Exit(config.Socks5ExitConfig{
		Host: update.Host, Port: update.Port, Username: update.Username, Domains: update.Domains,
	}, update.Password)
}
