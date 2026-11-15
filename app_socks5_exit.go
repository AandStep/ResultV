package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
)

const (
	socks5ExitCheckBudget = 30 * time.Second
	socks5ReplyNotAllowed = 2
)

type Socks5ExitStatusDTO struct {
	proxy.Socks5ExitCheck
	Trigger string `json:"trigger"`
	Checked bool   `json:"checked"`
}

type socks5ExitStatus struct {
	session proxy.SessionRef
	dto     Socks5ExitStatusDTO
}

type Socks5ExitSettingsDTO struct {
	Enabled  bool     `json:"enabled"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	Sites    []string `json:"sites"`
	Apps     []string `json:"apps"`
}

type Socks5ExitSettingsUpdate struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Username string   `json:"username"`
	Password *string  `json:"password"`
	Sites    []string `json:"sites"`
	Apps     []string `json:"apps"`
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
		Enabled: p.Enabled, Host: p.Host, Port: p.Port, Username: p.Username, Password: p.Password,
		Sites: p.Sites, Apps: p.Apps,
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
		Host: update.Host, Port: update.Port, Username: update.Username, Sites: update.Sites, Apps: update.Apps,
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
		if a.proxy.Socks5ExitActive() {
			go a.runSocks5ExitCheck("change")
		}
	}
	return nil
}

func (a *App) CheckSocks5Exit() Socks5ExitStatusDTO {
	return a.runSocks5ExitCheck("manual")
}

func (a *App) GetSocks5ExitStatus() Socks5ExitStatusDTO {
	if a.proxy == nil {
		return Socks5ExitStatusDTO{}
	}
	a.socks5StatusMu.Lock()
	stored := a.socks5Status
	a.socks5StatusMu.Unlock()
	return statusForSession(stored.session, a.proxy.CurrentSession(), stored.dto)
}

func (a *App) runSocks5ExitCheck(trigger string) Socks5ExitStatusDTO {
	if a.proxy == nil {
		return Socks5ExitStatusDTO{}
	}
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	session := a.proxy.CurrentSession()
	ctx, cancel := context.WithTimeout(parent, socks5ExitCheckBudget)
	check := a.proxy.CheckSocks5Exit(ctx)
	cancel()
	dto := Socks5ExitStatusDTO{Socks5ExitCheck: check, Trigger: trigger, Checked: check.Reason != proxy.Socks5CheckInactive}
	if dto.Checked {
		a.socks5StatusMu.Lock()
		a.socks5Status = socks5ExitStatus{session: session, dto: dto}
		a.socks5StatusMu.Unlock()
		if line, ok := socks5ExitLogLine(check); a.log != nil && ok {
			a.log.Info(line)
		} else if a.log != nil {
			a.log.Warning(line)
		}
	}
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "socks5:status", dto)
	}
	return dto
}

func statusForSession[K comparable](stored, current K, dto Socks5ExitStatusDTO) Socks5ExitStatusDTO {
	var none K
	if current == none || stored != current {
		return Socks5ExitStatusDTO{}
	}
	return dto
}

func socks5ExitLogLine(c proxy.Socks5ExitCheck) (string, bool) {
	if c.OK {
		udp := "нет"
		if c.UDP {
			udp = "есть"
		}
		where := c.IP
		if c.Country != "" {
			where += " (" + c.Country + ")"
		}
		return fmt.Sprintf("[SOCKS5] Выход работает: %s, UDP: %s, %d мс", where, udp, c.LatencyMs), true
	}
	reason := "сайт проверки не ответил"
	switch c.Reason {
	case proxy.Socks5CheckFirstHopDown:
		reason = "VPN-узел не отвечает"
	case proxy.Socks5CheckNoResponse:
		reason = "SOCKS5-сервер не отвечает"
	case proxy.Socks5CheckBadAuth:
		reason = "неверный логин или пароль"
	case proxy.Socks5CheckAuthMethod:
		reason = "сервер требует другой способ входа"
	case proxy.Socks5CheckRejected:
		reason = fmt.Sprintf("сервер отказал в соединении (код %d)", c.Code)
		if c.Code == socks5ReplyNotAllowed {
			reason = fmt.Sprintf("сервер запретил соединение, проверьте логин и пароль (код %d)", c.Code)
		}
	}
	return "[SOCKS5] Выход не работает: " + reason, false
}
