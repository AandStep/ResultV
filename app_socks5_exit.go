package main

import (
	"errors"

	"resultproxy-wails/internal/config"
)

type Socks5ExitSettingsDTO struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	HasPassword bool   `json:"hasPassword"`
}

type Socks5ExitSettingsUpdate struct {
	Host     string  `json:"host"`
	Port     int     `json:"port"`
	Username string  `json:"username"`
	Password *string `json:"password"`
}

func appConfigForUI(cfg config.AppConfig) config.AppConfig {
	cfg.Settings.Socks5Exit = nil
	cfg.Settings.LegacySocks5ExitArmed = false
	return cfg
}

func (a *App) SetSocks5ExitEnabled(enabled bool) error {
	if a.config == nil {
		return errors.New("config manager not initialized")
	}
	a.socks5ExitMu.Lock()
	defer a.socks5ExitMu.Unlock()
	previous := a.config.GetConfig().Settings.Socks5Exit
	if err := a.config.SetSocks5ExitEnabled(enabled); err != nil {
		return err
	}
	if err := a.applySocks5Exit(previous); err != nil {
		return err
	}
	return a.config.ClearSocks5ExitPreviewGuard()
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
		Enabled: p.Enabled, Host: p.Host, Port: p.Port, Username: p.Username, HasPassword: p.Password != "",
	}, nil
}

func (a *App) SaveSocks5ExitSettings(update Socks5ExitSettingsUpdate) error {
	if a.config == nil {
		return errors.New("config manager not initialized")
	}
	a.socks5ExitMu.Lock()
	defer a.socks5ExitMu.Unlock()
	previous := a.config.GetConfig().Settings.Socks5Exit
	if err := a.config.SaveSocks5Exit(config.Socks5ExitConfig{
		Host: update.Host, Port: update.Port, Username: update.Username,
	}, update.Password); err != nil {
		return err
	}
	return a.applySocks5Exit(previous)
}

func (a *App) applySocks5Exit(previous *config.Socks5ExitConfig) error {
	if a.proxy != nil {
		if err := a.proxy.SetSocks5Exit(a.config.GetConfig().Settings.Socks5Exit); err != nil {
			return errors.Join(err, a.config.RestoreSocks5Exit(previous))
		}
	}
	return nil
}
