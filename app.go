// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"resultproxy-wails/internal/adblock"
	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/logger"
	"resultproxy-wails/internal/proxy"
	"resultproxy-wails/internal/system"
)

var stableHWIDProvider = config.StableHardwareID

const subscriptionPageUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

type App struct {
	ctx    context.Context
	cancel context.CancelFunc

	log        *logger.Logger
	crypto     *config.CryptoService
	config     *config.Manager
	proxy      *proxy.Manager
	adblock    *adblock.Blocker
	tray       *system.Tray
	killSwitch system.KillSwitch
	netmon     *system.NetMonitor

	trayIcon []byte

	stateMu       sync.Mutex
	quitRequested bool

	trayHidden    atomic.Uint32
	taskbarUnhook func()
	smartProvider proxy.BlockedListProvider

	startInTray bool
}

func NewApp() *App {
	return &App{
		log:     logger.New(),
		adblock: adblock.New(),
	}
}

func (a *App) SetTrayIcon(icon []byte) {
	a.trayIcon = icon
}

func (a *App) SetStartInTray(v bool) {
	a.startInTray = v
}

func (a *App) GetVersion() string {
	return productVersionFromWailsJSON()
}

func (a *App) startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)

	a.log.SetEmitter(func(eventName string, data any) {
		wailsRuntime.EventsEmit(a.ctx, eventName, data)
	})

	a.log.Info("ResultV запускается...")

	if err := system.MigrateLegacyUserData(); err != nil {
		a.log.Warning(fmt.Sprintf("[CONFIG] Ошибка миграции legacy-данных: %v", err))
	}

	userDataPath := a.getUserDataPath()
	a.log.Info(fmt.Sprintf("[CONFIG] UserDataDir: %s", userDataPath))

	cs, err := config.NewCryptoService(userDataPath)
	if err != nil {
		a.log.Error(fmt.Sprintf("Ошибка инициализации шифрования: %v", err))
		return
	}
	a.crypto = cs
	if src := cs.KeySource(); src != "" {
		a.log.Info(fmt.Sprintf("[CONFIG] Key source: %s", src))
	}

	a.config = config.NewManager(cs)
	if err := a.config.Init(userDataPath); err != nil {
		if errors.Is(err, config.ErrDecryptFailed) {
			a.log.Warning(fmt.Sprintf("Конфигурация сброшена: %v", err))
		} else {
			a.log.Error(fmt.Sprintf("Ошибка загрузки конфигурации: %v", err))
		}
	} else {
		a.log.Success("Конфигурация загружена")
	}

	a.proxy = proxy.NewManager(a.log)
	a.proxy.Init(a.ctx)
	rootDir := a.getAppRootDir()
	a.initSmartBlockedDomains(userDataPath, rootDir)

	if err := a.adblock.LoadFromCache(userDataPath); err != nil {
		a.log.Warning(fmt.Sprintf("Кэш AdBlock не загружен: %v", err))
	}

	a.killSwitch = system.NewKillSwitch()

	a.netmon = system.NewNetMonitor(func(status system.NetworkStatus) {
		wailsRuntime.EventsEmit(a.ctx, "network:status", status)
		if status.Online {
			a.log.Info("[СЕТЬ] Интернет-соединение восстановлено")
		} else {
			a.log.Warning("[СЕТЬ] Интернет-соединение потеряно")
		}
	})
	a.netmon.Start(a.ctx)

	a.tray = system.NewTray(a.trayIcon, system.TrayCallbacks{
		OnShowWindow: func() {
			a.restoreMainWindow()
		},
		OnSelectProxy: func(proxyID string) {
			if err := a.setLastSelectedProxy(proxyID); err != nil {
				a.log.Warning(fmt.Sprintf("Не удалось сохранить выбор сервера в трее: %v", err))
			}
		},
		OnConnectSelected: func(proxyID string) {
			if err := a.connectFromTray(proxyID); err != nil {
				a.log.Error(fmt.Sprintf("Ошибка подключения из трея: %v", err))
			}
		},
		OnDisconnect: func() {
			if err := a.Disconnect(); err != nil {
				a.log.Error(fmt.Sprintf("Ошибка отключения из трея: %v", err))
			}
		},
		OnQuit: func() {
			a.markQuitRequested()
			wailsRuntime.Quit(a.ctx)
		},
	})
	a.tray.Start()
	a.refreshTrayProxyList()
	a.startTrayPingLoop()

	if system.DetectGPOConflict() {
		a.log.Warning("[СИСТЕМА] Обнаружен конфликт с групповой политикой (GPO). Настройки прокси могут быть переопределены.")
		wailsRuntime.EventsEmit(a.ctx, "system:gpo-conflict", true)
	}

	a.taskbarUnhook = system.StartTaskbarRestoreHook(a.ctx, system.TaskbarRestoreConfig{
		ClassName: system.WailsWindowClassResultV,
		IsHiddenToTray: func() bool {
			return a.trayHidden.Load() != 0
		},
		OnRestore: func() {
			a.restoreMainWindow()
		},
	})

	if a.startInTray {
		a.trayHidden.Store(1)
		wailsRuntime.WindowHide(a.ctx)
	}

	a.log.Success("ResultV готов к работе")
}

func (a *App) shutdown(ctx context.Context) {
	a.log.Info("ResultV завершает работу...")

	if a.taskbarUnhook != nil {
		a.taskbarUnhook()
		a.taskbarUnhook = nil
	}

	if a.netmon != nil {
		a.netmon.Stop()
	}

	if a.tray != nil {
		a.tray.Stop()
	}

	if a.killSwitch != nil && a.killSwitch.IsEnabled() {
		_ = a.killSwitch.Disable()
	}

	if a.proxy != nil {
		a.proxy.Shutdown()
	}

	if a.cancel != nil {
		a.cancel()
	}
}

func (a *App) BeforeClose(ctx context.Context) bool {
	a.stateMu.Lock()
	quitRequested := a.quitRequested
	a.stateMu.Unlock()
	if quitRequested {
		return false
	}
	a.trayHidden.Store(1)
	wailsRuntime.WindowHide(ctx)
	return true
}

func (a *App) GetConfig() (config.AppConfig, error) {
	if a.config == nil {
		return config.DefaultConfig(), nil
	}
	return a.config.GetConfig(), nil
}

func (a *App) SaveConfig(cfg config.AppConfig) error {
	if a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}

	existing := a.config.GetConfig()
	if cfg.Subscriptions == nil || (len(cfg.Subscriptions) == 0 && len(existing.Subscriptions) > 0) {
		cfg.Subscriptions = existing.Subscriptions
	}
	if err := a.config.SaveConfig(cfg); err != nil {
		a.log.Error(fmt.Sprintf("Ошибка сохранения конфигурации: %v", err))
		return err
	}
	a.log.Success("Конфигурация сохранена")
	return nil
}

func (a *App) Connect(proxyDTO proxy.ProxyConfig, rules config.RoutingRules,
	killSwitch, adBlock bool) (proxy.ConnectResultDTO, error) {

	if a.proxy == nil {
		return proxy.ConnectResultDTO{Success: false, Message: "Proxy manager not initialized"}, nil
	}

	cfg := a.config.GetConfig()
	mode := proxy.ProxyMode(cfg.Settings.Mode)
	dnsServers := append([]string(nil), cfg.Settings.DNSServers...)
	if fromProxy := dnsServersFromProxyExtra(proxyDTO); len(fromProxy) > 0 {
		dnsServers = fromProxy
	}

	result := a.proxy.Connect(
		a.ctx,
		proxyDTO,
		mode,
		proxy.RoutingMode(rules.Mode),
		rules.Whitelist,
		rules.AppWhitelist,
		killSwitch,
		adBlock,
		cfg.Settings.LocalPort,
		cfg.Settings.ListenLAN,
		dnsServers,
		cfg.Settings.TunIPv4,
	)

	if result.Success {
		serverName := fmt.Sprintf("%s:%d", proxyDTO.IP, proxyDTO.Port)
		if a.tray != nil {
			a.tray.SetConnectedProxy(a.resolveProxyID(proxyDTO), serverName)
		}
		wailsRuntime.EventsEmit(a.ctx, "proxy:connected", proxyDTO)
	}

	return result, nil
}

func dnsServersFromProxyExtra(proxyDTO proxy.ProxyConfig) []string {
	if len(proxyDTO.Extra) == 0 {
		return nil
	}
	var extra map[string]interface{}
	if err := json.Unmarshal(proxyDTO.Extra, &extra); err != nil || extra == nil {
		return nil
	}
	readList := func(key string) []string {
		v, ok := extra[key]
		if !ok || v == nil {
			return nil
		}
		out := []string{}
		switch t := v.(type) {
		case []interface{}:
			for _, item := range t {
				s := strings.TrimSpace(fmt.Sprint(item))
				if s != "" {
					out = append(out, s)
				}
			}
		case []string:
			for _, item := range t {
				s := strings.TrimSpace(item)
				if s != "" {
					out = append(out, s)
				}
			}
		case string:
			for _, part := range strings.Split(t, ",") {
				s := strings.TrimSpace(part)
				if s != "" {
					out = append(out, s)
				}
			}
		}
		if len(out) == 0 {
			return nil
		}
		seen := make(map[string]struct{}, len(out))
		uniq := make([]string, 0, len(out))
		for _, s := range out {
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			uniq = append(uniq, s)
		}
		return uniq
	}
	if v := readList("dns_servers"); len(v) > 0 {
		return v
	}
	if v := readList("dns"); len(v) > 0 {
		return v
	}
	return nil
}

func (a *App) Disconnect() error {
	if a.proxy == nil {
		return nil
	}
	err := a.proxy.Disconnect()
	if err == nil {
		if a.tray != nil {
			a.tray.SetDisconnected()
		}
		wailsRuntime.EventsEmit(a.ctx, "proxy:disconnected", nil)
	}
	return err
}

func (a *App) GetStatus() proxy.StatusDTO {
	if a.proxy == nil {
		return proxy.StatusDTO{Mode: proxy.ProxyModeProxy}
	}
	return a.proxy.GetStatus()
}

func (a *App) SetMode(mode string) error {
	result, err := a.ApplyMode(mode)
	if err != nil {
		return err
	}
	if !result.Success {
		return errors.New(result.Message)
	}
	return nil
}

func (a *App) ApplyMode(mode string) (proxy.ConnectResultDTO, error) {
	if mode != string(proxy.ProxyModeProxy) && mode != string(proxy.ProxyModeTunnel) {
		return proxy.ConnectResultDTO{
			Success: false,
			Message: fmt.Sprintf("неподдерживаемый режим: %s", mode),
		}, nil
	}
	if a.config == nil {
		return proxy.ConnectResultDTO{Success: false, Message: "config manager not initialized"}, nil
	}
	if a.proxy == nil {
		return proxy.ConnectResultDTO{Success: false, Message: "proxy manager not initialized"}, nil
	}

	cfg := a.config.GetConfig()
	previousMode := cfg.Settings.Mode
	cfg.Settings.Mode = mode
	if err := a.config.SaveConfig(cfg); err != nil {
		a.log.Error(fmt.Sprintf("Ошибка сохранения режима: %v", err))
		return proxy.ConnectResultDTO{Success: false, Message: fmt.Sprintf("Ошибка сохранения режима: %v", err)}, nil
	}

	status := a.proxy.GetStatus()
	if status.CurrentProxy != nil {
		prevProxy := *status.CurrentProxy
		modeSwitchDNS := append([]string(nil), cfg.Settings.DNSServers...)
		if fromProxy := dnsServersFromProxyExtra(prevProxy); len(fromProxy) > 0 {
			modeSwitchDNS = fromProxy
		}
		result := a.proxy.Connect(
			a.ctx,
			prevProxy,
			proxy.ProxyMode(mode),
			proxy.RoutingMode(cfg.RoutingRules.Mode),
			cfg.RoutingRules.Whitelist,
			cfg.RoutingRules.AppWhitelist,
			cfg.Settings.KillSwitch,
			cfg.Settings.AdBlock,
			cfg.Settings.LocalPort,
			cfg.Settings.ListenLAN,
			modeSwitchDNS,
			cfg.Settings.TunIPv4,
		)
		if result.Success {
			serverName := fmt.Sprintf("%s:%d", status.CurrentProxy.IP, status.CurrentProxy.Port)
			if a.tray != nil {
				a.tray.SetConnectedProxy(a.resolveProxyID(*status.CurrentProxy), serverName)
			}
			wailsRuntime.EventsEmit(a.ctx, "proxy:connected", *status.CurrentProxy)
		} else if !result.FallbackUsed {

			cfg.Settings.Mode = previousMode
			_ = a.config.SaveConfig(cfg)
			rollback := a.proxy.Connect(
				a.ctx,
				prevProxy,
				proxy.ProxyMode(previousMode),
				proxy.RoutingMode(cfg.RoutingRules.Mode),
				cfg.RoutingRules.Whitelist,
				cfg.RoutingRules.AppWhitelist,
				cfg.Settings.KillSwitch,
				cfg.Settings.AdBlock,
				cfg.Settings.LocalPort,
				cfg.Settings.ListenLAN,
				modeSwitchDNS,
				cfg.Settings.TunIPv4,
			)
			if rollback.Success {
				if a.tray != nil {
					a.tray.SetConnectedProxy(a.resolveProxyID(prevProxy), fmt.Sprintf("%s:%d", prevProxy.IP, prevProxy.Port))
				}
				wailsRuntime.EventsEmit(a.ctx, "proxy:connected", prevProxy)
			} else {
				if a.tray != nil {
					a.tray.SetDisconnected()
				}
				wailsRuntime.EventsEmit(a.ctx, "proxy:disconnected", nil)
			}
		}
		return result, nil
	}

	if err := a.proxy.SetMode(proxy.ProxyMode(mode)); err != nil {
		return proxy.ConnectResultDTO{Success: false, Message: fmt.Sprintf("Ошибка применения режима: %v", err)}, nil
	}
	return proxy.ConnectResultDTO{Success: true, Message: "Режим сохранен"}, nil
}

func (a *App) GetMode() string {
	if a.proxy == nil {
		return "proxy"
	}
	return string(a.proxy.GetMode())
}

func (a *App) PingProxy(ip string, port int, proxyType string) proxy.PingResultDTO {
	if a.proxy == nil {
		return proxy.PingResultDTO{}
	}
	return a.proxy.Ping(ip, port, proxyType)
}

func (a *App) GetLogs(page, size int) logger.LogPage {
	return a.log.GetLogs(page, size)
}

func (a *App) ToggleKillSwitch(enable bool) error {
	if a.proxy == nil {
		return fmt.Errorf("proxy manager not initialized")
	}

	if enable && a.killSwitch != nil {
		status := a.proxy.GetStatus()
		proxyAddr := ""
		if status.CurrentProxy != nil {
			proxyAddr = fmt.Sprintf("%s:%d", status.CurrentProxy.IP, status.CurrentProxy.Port)
		}
		if err := a.killSwitch.Enable(proxyAddr); err != nil {
			a.log.Warning(fmt.Sprintf("[KILL SWITCH] Firewall недоступен, используем fallback: %v", err))

			return a.proxy.ToggleKillSwitch(enable)
		}
		if a.tray != nil {
			a.tray.SetKillSwitchActive()
		}
		a.log.Warning("[KILL SWITCH] Активирована полная блокировка интернета (firewall)")
		return nil
	}

	if !enable && a.killSwitch != nil && a.killSwitch.IsEnabled() {
		if err := a.killSwitch.Disable(); err != nil {
			a.log.Error(fmt.Sprintf("[KILL SWITCH] Ошибка отключения: %v", err))
		}
		a.log.Info("[KILL SWITCH] Деактивирован")
	}

	return a.proxy.ToggleKillSwitch(enable)
}

func (a *App) ToggleAdBlock(enable bool) error {
	if a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}
	cfg := a.config.GetConfig()
	cfg.Settings.AdBlock = enable
	return a.config.SaveConfig(cfg)
}

func (a *App) SetAutostart(enable bool) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}
	if enable {
		if err := system.EnableAutostart(exe); err != nil {
			a.log.Error(fmt.Sprintf("[СИСТЕМА] Ошибка создания автозапуска: %v", err))
			return err
		}
		a.log.Success("[СИСТЕМА] Автозапуск включен")
	} else {
		if err := system.DisableAutostart(); err != nil {
			a.log.Warning(fmt.Sprintf("[СИСТЕМА] Ошибка удаления автозапуска: %v", err))
			return err
		}
		a.log.Info("[СИСТЕМА] Автозапуск отключен")
	}
	return nil
}

func (a *App) IsAutostartEnabled() bool {
	return system.IsAutostartEnabled()
}

func (a *App) UpdateRules(rules config.RoutingRules) error {
	if a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}
	if err := a.config.UpdateRoutingRules(rules); err != nil {
		return err
	}
	if a.proxy == nil {
		return nil
	}
	status := a.proxy.GetStatus()
	if !status.IsConnected || status.CurrentProxy == nil {
		return nil
	}

	cur := *status.CurrentProxy
	result := a.proxy.ReconnectWithRoutingRules(
		a.ctx,
		proxy.RoutingMode(rules.Mode),
		rules.Whitelist,
		rules.AppWhitelist,
	)
	if !result.Success {
		a.log.Error(fmt.Sprintf("Ошибка применения правил маршрутизации: %s", result.Message))
		if a.tray != nil {
			a.tray.SetDisconnected()
		}
		wailsRuntime.EventsEmit(a.ctx, "proxy:disconnected", nil)
		return fmt.Errorf("%s", result.Message)
	}

	a.log.Info("[PROXY] Правила маршрутизации применены")
	if a.tray != nil {
		a.tray.SetConnectedProxy(a.resolveProxyID(cur), fmt.Sprintf("%s:%d", cur.IP, cur.Port))
	}
	wailsRuntime.EventsEmit(a.ctx, "proxy:connected", cur)
	return nil
}

func (a *App) ExportConfig() (string, error) {
	if a.config == nil {
		return "", fmt.Errorf("config manager not initialized")
	}
	cfg := a.config.GetConfig()
	result, err := config.ExportConfig(cfg)
	if err != nil {
		a.log.Error(fmt.Sprintf("Ошибка экспорта: %v", err))
		return "", err
	}
	a.log.Success("Конфигурация экспортирована")
	return result, nil
}

func (a *App) ImportConfig(data string) error {
	if a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}
	imported, err := config.ImportConfig(data)
	if err != nil {
		a.log.Error(fmt.Sprintf("Ошибка импорта: %v", err))
		return err
	}
	existing := a.config.GetConfig()
	merged := config.MergeImport(existing, imported)
	if err := a.config.SaveConfig(merged); err != nil {
		return err
	}
	a.log.Success(fmt.Sprintf("Импортировано %d прокси", len(imported.Proxies)))
	wailsRuntime.EventsEmit(a.ctx, "config:updated", merged)
	return nil
}

func (a *App) GetPlatform() string {
	return runtime.GOOS
}

func (a *App) IsAdmin() bool {
	return system.IsAdmin()
}

func (a *App) RestartAsAdmin() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	err = system.RestartAsAdmin(exe)
	if err == nil {
		a.markQuitRequested()
		wailsRuntime.Quit(a.ctx)
	}
	return err
}

func (a *App) GetNetworkTraffic() system.TrafficStats {
	return system.GetNetworkTraffic()
}

func (a *App) GetNetworkStatus() system.NetworkStatus {
	if a.netmon == nil {
		return system.NetworkStatus{Online: true}
	}
	return a.netmon.GetStatus()
}

func (a *App) GetLANIPs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return []string{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}
			if ip4[0] == 127 {
				continue
			}
			if ip4[0] == 169 && ip4[1] == 254 {
				continue
			}
			s := ip4.String()
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func (a *App) SyncProxies(proxies []config.ProxyEntry) error {
	if a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}
	cfg := a.config.GetConfig()
	cfg.Proxies = proxies
	if err := a.config.SaveConfig(cfg); err != nil {
		return err
	}
	a.refreshTrayProxyList()
	return nil
}

func (a *App) DetectCountry(ip string) (string, error) {

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://ip-api.com/json/%s?fields=status,countryCode", ip))
	if err != nil {
		return "Unknown", err
	}
	defer resp.Body.Close()

	var result struct {
		Status      string `json:"status"`
		CountryCode string `json:"countryCode"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "Unknown", err
	}

	if result.Status == "success" && result.CountryCode != "" {
		return strings.ToLower(result.CountryCode), nil
	}

	return "Unknown", nil
}

func parseSubscriptionUserInfoHeader(v string) (upload, download, total, expire int64) {
	if v == "" {
		return 0, 0, 0, 0
	}
	for _, part := range strings.Split(v, ";") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(kv[0]))
		val := strings.TrimSpace(kv[1])
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "upload":
			upload = n
		case "download":
			download = n
		case "total":
			total = n
		case "expire":
			expire = n
		}
	}
	return upload, download, total, expire
}

func parseSubscriptionHeaderText(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(v), "base64:") {
		raw := strings.TrimSpace(v[len("base64:"):])
		for _, enc := range [](*base64.Encoding){base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
			if decoded, err := enc.DecodeString(raw); err == nil {
				return strings.TrimSpace(string(decoded))
			}
		}
	}
	return v
}

func headerIsTruthy(h http.Header, key string) bool {
	v := strings.ToLower(strings.TrimSpace(h.Get(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func subscriptionEmptyBodyError(h http.Header) error {
	title := parseSubscriptionHeaderText(h.Get("Profile-Title"))
	announce := parseSubscriptionHeaderText(h.Get("Announce"))
	supportURL := strings.TrimSpace(h.Get("Support-Url"))
	hwidLimit := headerIsTruthy(h, "X-Hwid-Limit") || headerIsTruthy(h, "X-Hwid-Max-Devices-Reached")
	hwidNotSupported := headerIsTruthy(h, "X-Hwid-Not-Supported")

	reason := "подписка вернула пустой ответ"
	if hwidLimit {
		reason = "достигнут лимит устройств для подписки"
	} else if hwidNotSupported {
		reason = "провайдер требует передачу HWID"
	}

	details := make([]string, 0, 3)
	if title != "" {
		details = append(details, title)
	}
	if announce != "" {
		details = append(details, announce)
	}
	if supportURL != "" {
		details = append(details, "Поддержка: "+supportURL)
	}
	if len(details) == 0 {
		return errors.New(reason)
	}
	return fmt.Errorf("%s. %s", reason, strings.Join(details, " | "))
}

func (a *App) subscriptionHWID() string {
	hwid, err := stableHWIDProvider(a.getUserDataPath())
	if err != nil {
		a.log.Warning(fmt.Sprintf("Не удалось получить HWID для запроса подписки: %v", err))
		return ""
	}
	return strings.TrimSpace(hwid)
}

func subscriptionIconCandidates(subURL string, h http.Header) []string {
	parsed, err := url.Parse(subURL)
	if err != nil {
		parsed = nil
	}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, x := range out {
			if x == s {
				return
			}
		}
		out = append(out, s)
	}
	for _, key := range []string{
		"Profile-Icon-Url",
		"Icon-Url",
		"Subscription-Icon",
		"Icon",
		"Profile-Icon",
	} {
		v := strings.TrimSpace(h.Get(key))
		if v == "" {
			continue
		}
		low := strings.ToLower(v)
		if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
			add(v)
			continue
		}
		if strings.HasPrefix(v, "/") && parsed != nil && parsed.Scheme != "" && parsed.Host != "" {
			add(parsed.Scheme + "://" + parsed.Host + v)
		}
	}
	for key, vals := range h {
		if len(vals) == 0 {
			continue
		}
		lk := strings.ToLower(key)
		if !strings.Contains(lk, "icon") {
			continue
		}
		v := strings.TrimSpace(vals[0])
		low := strings.ToLower(v)
		if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
			add(v)
		}
	}
	return out
}

func imageContentTypeFromBytes(buf []byte, headerCT string) string {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(headerCT, ";")[0]))
	if strings.HasPrefix(ct, "image/") {
		return ct
	}
	if ct == "application/vnd.microsoft.icon" || ct == "image/vnd.microsoft.icon" {
		return "image/x-icon"
	}
	if len(buf) >= 8 && buf[0] == 0x89 && buf[1] == 0x50 && buf[2] == 0x4e && buf[3] == 0x47 {
		return "image/png"
	}
	if len(buf) >= 4 && buf[0] == 0 && buf[1] == 0 && buf[2] == 1 && buf[3] == 0 {
		return "image/x-icon"
	}
	if len(buf) >= 2 && buf[0] == 0xff && buf[1] == 0xd8 {
		return "image/jpeg"
	}
	if len(buf) >= 6 {
		s6 := string(buf[0:6])
		if s6 == "GIF87a" || s6 == "GIF89a" {
			return "image/gif"
		}
	}
	if ct == "application/octet-stream" || ct == "binary/octet-stream" || ct == "" {
		if g := http.DetectContentType(buf); strings.HasPrefix(strings.ToLower(g), "image/") {
			return strings.ToLower(g)
		}
	}
	return ""
}

func inlineSmallImageFromURL(client *http.Client, imageURL string, referer string) string {
	if imageURL == "" {
		return ""
	}
	low := strings.ToLower(imageURL)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", subscriptionPageUserAgent)
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	if strings.TrimSpace(referer) != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return ""
	}
	defer resp.Body.Close()
	const maxBytes = 262144
	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil || len(buf) > maxBytes {
		return ""
	}
	ct := imageContentTypeFromBytes(buf, resp.Header.Get("Content-Type"))
	if ct == "" {
		return ""
	}
	return "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(buf)
}

func resolveSubscriptionIcon(client *http.Client, subURL string, h http.Header) string {
	cands := subscriptionIconCandidates(subURL, h)
	for _, cand := range cands {
		if data := inlineSmallImageFromURL(client, cand, subURL); data != "" {
			return data
		}
	}
	if len(cands) > 0 {
		return cands[0]
	}
	if fromPage := discoverIconFromSubscriptionPage(client, subURL); fromPage != "" {
		return fromPage
	}
	for _, cand := range originIconFallbackURLs(subURL) {
		if data := inlineSmallImageFromURL(client, cand, subURL); data != "" {
			return data
		}
	}
	return ""
}

func originIconFallbackURLs(subURL string) []string {
	parsed, err := url.Parse(subURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil
	}
	base := parsed.Scheme + "://" + parsed.Host
	return []string{
		base + "/assets/apple-touch-icon-180x180.png",
		base + "/assets/favicon-32x32.png",
		base + "/assets/favicon.ico",
		base + "/apple-touch-icon.png",
		base + "/apple-touch-icon-precomposed.png",
		base + "/favicon.ico",
	}
}

func pickIconFromSubscriptionHTML(client *http.Client, subURL string, html string) string {
	html = strings.TrimSpace(html)
	if html == "" {
		return ""
	}
	if len(html) > 262144 {
		html = html[:262144]
	}
	reMeta := regexp.MustCompile(`(?is)<meta[^>]+(?:property|name)=["'](?:og:image|twitter:image)["'][^>]+content=["']([^"']+)["']`)
	reMetaRev := regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']+)["'][^>]+(?:property|name)=["'](?:og:image|twitter:image)["']`)
	reApple1 := regexp.MustCompile(`(?is)<link[^>]+rel=["']apple-touch-icon["'][^>]+href=["']([^"']+)["']`)
	reApple2 := regexp.MustCompile(`(?is)<link[^>]+href=["']([^"']+)["'][^>]+rel=["']apple-touch-icon["']`)
	reLink := regexp.MustCompile(`(?is)<link[^>]+rel=["'][^"']*icon[^"']*["'][^>]+href=["']([^"']+)["']`)
	reLinkHrefFirst := regexp.MustCompile(`(?is)<link[^>]+href=["']([^"']+)["'][^>]+rel=["'][^"']*icon[^"']*["']`)
	reImgLogo := regexp.MustCompile(`(?is)<img[^>]+src=["']([^"']+)["'][^>]*(?:logo|brand)|(?:logo|brand)[^>]*<img[^>]+src=["']([^"']+)["']`)
	parsedBase, _ := url.Parse(subURL)
	resolve := func(raw string) string {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return ""
		}
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		if u.IsAbs() {
			return u.String()
		}
		if parsedBase == nil {
			return ""
		}
		return parsedBase.ResolveReference(u).String()
	}
	tryRegexes := func(re *regexp.Regexp) string {
		m := re.FindStringSubmatch(html)
		if len(m) == 0 {
			return ""
		}
		for i := 1; i < len(m); i++ {
			candidate := resolve(m[i])
			if candidate == "" {
				continue
			}
			if data := inlineSmallImageFromURL(client, candidate, subURL); data != "" {
				return data
			}
			continue
		}
		return ""
	}
	for _, re := range []*regexp.Regexp{reMeta, reMetaRev, reApple1, reApple2, reLink, reLinkHrefFirst, reImgLogo} {
		if got := tryRegexes(re); got != "" {
			return got
		}
	}
	return ""
}

func loadHappApiKey() string {
	if key := os.Getenv("HAPP_API_KEY"); key != "" {
		fmt.Printf("[DEBUG] Using HAPP_API_KEY from environment\n")
		return key
	}
	paths := []string{"frontend/.env", ".env", "../frontend/.env"}
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			fmt.Printf("[DEBUG] Found .env file at %s\n", p)
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "VITE_HAPP_API_KEY=") {
					k := strings.TrimPrefix(line, "VITE_HAPP_API_KEY=")
					fmt.Printf("[DEBUG] Loaded VITE_HAPP_API_KEY from %s\n", p)
					return k
				}
				if strings.HasPrefix(line, "HAPP_API_KEY=") {
					k := strings.TrimPrefix(line, "HAPP_API_KEY=")
					fmt.Printf("[DEBUG] Loaded HAPP_API_KEY from %s\n", p)
					return k
				}
			}
		}
	}
	fmt.Printf("[DEBUG] No HAPP_API_KEY found in environment or .env files\n")
	return ""
}

func (a *App) DecryptHappLink(text string, apiKey string) string {
	if !strings.Contains(text, "happ://crypt") {
		return text
	}
	client := &http.Client{Timeout: 10 * time.Second}
	re := regexp.MustCompile(`happ://crypt[0-9]?/[A-Za-z0-9+/=]+`)

	if apiKey == "" {
		apiKey = loadHappApiKey()
	}

	return re.ReplaceAllStringFunc(text, func(match string) string {
		decrypted, err := decryptHappCryptLink(client, match, apiKey)
		if err != nil || decrypted == "" {
			a.log.Warning(fmt.Sprintf("Failed to decrypt %s: %v", match, err))
			return match
		}
		return decrypted
	})
}

func decryptHappCryptLink(client *http.Client, cryptLink string, apiKey string) (string, error) {
	fmt.Printf("[DEBUG] Decrypting happ link: %s\n", cryptLink)
	reqBody, _ := json.Marshal(map[string]string{"link": cryptLink})
	req, err := http.NewRequest(http.MethodPost, "https://api.sayori.cc/v1/decrypt", strings.NewReader(string(reqBody)))
	if err != nil {
		fmt.Printf("[DEBUG] Error creating request: %v\n", err)
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	if apiKey == "" {
		apiKey = loadHappApiKey()
	}
	req.Header.Set("x-api-key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("[DEBUG] Error sending request: %v\n", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[DEBUG] API returned status %d\n", resp.StatusCode)
		return "", fmt.Errorf("decrypt API returned status %d", resp.StatusCode)
	}

	var result struct {
		Success bool   `json:"success"`
		Result  string `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Printf("[DEBUG] Error decoding response: %v\n", err)
		return "", err
	}
	if !result.Success {
		fmt.Printf("[DEBUG] API returned success=false\n")
		return "", errors.New("decrypt API returned success=false")
	}
	fmt.Printf("[DEBUG] Successfully decrypted happ link\n")
	return result.Result, nil
}

func discoverIconFromSubscriptionPage(client *http.Client, subURL string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", subscriptionPageUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil || resp == nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 262144))
	if err != nil || len(body) == 0 {
		return ""
	}
	ct := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	html := string(body)
	htmlOK := strings.Contains(ct, "text/html") || strings.Contains(ct, "html") || strings.Contains(ct, "xhtml")
	if !htmlOK && !strings.HasPrefix(strings.TrimSpace(html), "<") {
		return ""
	}
	return pickIconFromSubscriptionHTML(client, subURL, html)
}

func (a *App) fetchSubscriptionFromURL(subURL string) ([]config.ProxyEntry, int64, int64, int64, int64, string, string, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 15 * time.Second, Jar: jar}
	req, err := http.NewRequest(http.MethodGet, subURL, nil)
	if err != nil {
		return nil, 0, 0, 0, 0, "", "", fmt.Errorf("creating subscription request: %w", err)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("ResultProxyPC/%s", productVersionFromWailsJSON()))
	if hwid := a.subscriptionHWID(); hwid != "" {
		req.Header.Set("x-hwid", hwid)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, 0, 0, 0, "", "", fmt.Errorf("fetching subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, 0, 0, 0, "", "", fmt.Errorf("subscription returned HTTP %d", resp.StatusCode)
	}

	profileTitle := parseSubscriptionHeaderText(resp.Header.Get("Profile-Title"))
	up, down, tot, exp := parseSubscriptionUserInfoHeader(resp.Header.Get("Subscription-Userinfo"))
	iconURL := resolveSubscriptionIcon(client, subURL, resp.Header)

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, up, down, tot, exp, iconURL, profileTitle, fmt.Errorf("reading subscription body: %w", err)
	}
	bodyStr := string(bodyBytes)

	var firstDecrypted string
	if strings.Contains(bodyStr, "happ://crypt") {
		re := regexp.MustCompile(`happ://crypt[0-9]?/[A-Za-z0-9+/=]+`)
		foundHapp := false

		bodyStr = re.ReplaceAllStringFunc(bodyStr, func(match string) string {
			decrypted, err := decryptHappCryptLink(client, match, "")
			if err != nil {
				a.log.Warning(fmt.Sprintf("Ошибка расшифровки happ:// ссылки: %v", err))
				return match
			}
			foundHapp = true
			if firstDecrypted == "" {
				firstDecrypted = decrypted
			}
			return decrypted
		})

		if foundHapp && (strings.Contains(bodyStr, "<html") || strings.Contains(bodyStr, "<body")) {
			fmt.Printf("[DEBUG] HTML detected, using decrypted content as body\n")
			bodyStr = firstDecrypted
		}
	}

	if firstDecrypted != "" {
		display := firstDecrypted
		if len(display) > 100 {
			display = display[:100] + "..."
		}
		fmt.Printf("[DEBUG] Final body snippet: %s\n", display)

		// КРИТИЧЕСКИЙ МОМЕНТ: если расшифровка вернула URL, нам нужно скачать его содержимое!
		trimmedResult := strings.TrimSpace(firstDecrypted)
		if strings.HasPrefix(trimmedResult, "http://") || strings.HasPrefix(trimmedResult, "https://") {
			fmt.Printf("[DEBUG] Decryption returned a URL, fetching recursively with HWID: %s\n", trimmedResult)
			req, err := http.NewRequest(http.MethodGet, trimmedResult, nil)
			if err == nil {
				req.Header.Set("User-Agent", fmt.Sprintf("ResultProxyPC/%s", productVersionFromWailsJSON()))
				if hwid := a.subscriptionHWID(); hwid != "" {
					req.Header.Set("x-hwid", hwid)
				}
				resp, err := client.Do(req)
				if err == nil && resp.StatusCode == http.StatusOK {
					newBody, err := io.ReadAll(resp.Body)
					if err == nil {
						bodyStr = string(newBody)
						fmt.Printf("[DEBUG] Successfully fetched recursive body (%d bytes)\n", len(bodyStr))
						
						// ОБНОВЛЯЕМ МЕТАДАННЫЕ (трафик, срок действия, название, иконка) из заголовков рекурсивного ответа
						u, d, t, e := parseSubscriptionUserInfoHeader(resp.Header.Get("Subscription-Userinfo"))
						if u > 0 || d > 0 || t > 0 || e > 0 {
							up, down, tot, exp = u, d, t, e
						}
						if pt := parseSubscriptionHeaderText(resp.Header.Get("Profile-Title")); pt != "" {
							profileTitle = pt
						}
						if icon := resolveSubscriptionIcon(client, trimmedResult, resp.Header); icon != "" {
							iconURL = icon
						}
					}
					resp.Body.Close()
				}
			}
		}
	}
	
	// Если после всех манипуляций иконка все еще пустая, пробуем найти её в финальном теле
	if iconURL == "" && strings.Contains(bodyStr, "<link") {
		if fromBody := pickIconFromSubscriptionHTML(client, subURL, bodyStr); fromBody != "" {
			iconURL = fromBody
		}
	}
	if strings.TrimSpace(strings.TrimPrefix(bodyStr, "\uFEFF")) == "" {
		return nil, up, down, tot, exp, iconURL, profileTitle, subscriptionEmptyBodyError(resp.Header)
	}

	entries, err := proxy.ParseSubscriptionBody(bodyStr)
	if err != nil {
		return nil, up, down, tot, exp, iconURL, profileTitle, err
	}

	providerName := extractProviderName(subURL)
	if profileTitle != "" {
		providerName = profileTitle
	}
	baseID := time.Now().UnixMilli()
	for i := range entries {
		entries[i].SubscriptionURL = subURL
		entries[i].Provider = providerName
		entries[i].ID = fmt.Sprintf("%d", baseID+int64(i))
	}

	a.log.Success(fmt.Sprintf("Подписка загружена: %d серверов", len(entries)))
	return entries, up, down, tot, exp, iconURL, profileTitle, nil
}

func (a *App) FetchSubscription(subURL string) ([]config.ProxyEntry, error) {
	entries, _, _, _, _, _, _, err := a.fetchSubscriptionFromURL(subURL)
	return entries, err
}

func (a *App) RefreshSubscription(subID string) ([]config.ProxyEntry, error) {
	if a.config == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}

	cfg := a.config.GetConfig()
	var sub *config.Subscription
	for i := range cfg.Subscriptions {
		if cfg.Subscriptions[i].ID == subID {
			sub = &cfg.Subscriptions[i]
			break
		}
	}
	if sub == nil {
		return nil, fmt.Errorf("subscription %s not found", subID)
	}

	entries, up, down, tot, exp, iconURL, profileTitle, err := a.fetchSubscriptionFromURL(sub.URL)
	if err != nil {
		return nil, fmt.Errorf("refreshing subscription %s: %w", sub.Name, err)
	}

	displayName := sub.Name
	if profileTitle != "" {
		displayName = profileTitle
	}
	for i := range entries {
		entries[i].Provider = displayName
		entries[i].SubscriptionURL = sub.URL
	}

	for i := range cfg.Subscriptions {
		if cfg.Subscriptions[i].ID == subID {
			cfg.Subscriptions[i].UpdatedAt = time.Now().Format(time.RFC3339)
			cfg.Subscriptions[i].TrafficUpload = up
			cfg.Subscriptions[i].TrafficDownload = down
			cfg.Subscriptions[i].TrafficTotal = tot
			cfg.Subscriptions[i].ExpireUnix = exp
			if profileTitle != "" {
				cfg.Subscriptions[i].Name = profileTitle
			}
			if iconURL != "" {
				cfg.Subscriptions[i].IconURL = iconURL
			}
			break
		}
	}
	if err := a.config.SaveConfig(cfg); err != nil {
		a.log.Error(fmt.Sprintf("Ошибка сохранения после обновления подписки: %v", err))
	}

	a.log.Success(fmt.Sprintf("Подписка '%s' обновлена: %d серверов", displayName, len(entries)))
	return entries, nil
}

func (a *App) AddSubscription(name, subURL string) ([]config.ProxyEntry, error) {
	if a.config == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}

	cfg := a.config.GetConfig()
	for _, s := range cfg.Subscriptions {
		if s.URL == subURL {
			return nil, fmt.Errorf("подписка с этим URL уже добавлена")
		}
	}

	entries, up, down, tot, exp, iconURL, profileTitle, err := a.fetchSubscriptionFromURL(subURL)
	if err != nil {
		return nil, err
	}

	displayName := name
	if profileTitle != "" {
		displayName = profileTitle
	}

	sub := config.Subscription{
		ID:              fmt.Sprintf("%d", time.Now().UnixMilli()),
		Name:            displayName,
		URL:             subURL,
		UpdatedAt:       time.Now().Format(time.RFC3339),
		TrafficUpload:   up,
		TrafficDownload: down,
		TrafficTotal:    tot,
		ExpireUnix:      exp,
		IconURL:         iconURL,
	}

	for i := range entries {
		entries[i].Provider = displayName
	}

	cfg.Subscriptions = append(cfg.Subscriptions, sub)
	if err := a.config.SaveConfig(cfg); err != nil {
		return nil, fmt.Errorf("saving subscription: %w", err)
	}

	a.log.Success(fmt.Sprintf("Подписка '%s' добавлена: %d серверов", displayName, len(entries)))
	return entries, nil
}

func (a *App) DeleteSubscription(subID string) error {
	if a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}

	cfg := a.config.GetConfig()
	found := false
	newSubs := make([]config.Subscription, 0, len(cfg.Subscriptions))
	for _, s := range cfg.Subscriptions {
		if s.ID == subID {
			found = true
			continue
		}
		newSubs = append(newSubs, s)
	}
	if !found {
		return fmt.Errorf("subscription %s not found", subID)
	}
	cfg.Subscriptions = newSubs
	return a.config.SaveConfig(cfg)
}

func extractProviderName(subURL string) string {
	u, err := url.Parse(subURL)
	if err != nil || u.Host == "" {
		return "Subscription"
	}
	host := u.Hostname()

	host = strings.TrimPrefix(host, "www.")

	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		name := parts[len(parts)-2]

		if len(name) > 0 {
			return strings.ToUpper(name[:1]) + name[1:]
		}
	}
	return host
}

func (a *App) getUserDataPath() string {
	return system.UserDataDir()
}

func (a *App) getAppRootDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func (a *App) markQuitRequested() {
	a.stateMu.Lock()
	a.quitRequested = true
	a.stateMu.Unlock()
}

func (a *App) restoreMainWindow() {
	if a.ctx == nil {
		return
	}
	a.trayHidden.Store(0)
	wailsRuntime.WindowUnminimise(a.ctx)
	wailsRuntime.WindowShow(a.ctx)

	wailsRuntime.WindowSetAlwaysOnTop(a.ctx, true)
	wailsRuntime.WindowSetAlwaysOnTop(a.ctx, false)
}

func (a *App) refreshTrayProxyList() {
	if a.tray == nil || a.config == nil {
		return
	}
	cfg := a.config.GetConfig()
	selectedID := cfg.Settings.LastSelectedProxyID
	status := a.GetStatus()
	if status.IsConnected && status.CurrentProxy != nil {
		a.tray.SetConnectedProxy(a.resolveProxyID(*status.CurrentProxy), fmt.Sprintf("%s:%d", status.CurrentProxy.IP, status.CurrentProxy.Port))
	} else {
		a.tray.SetDisconnected()
	}
	a.tray.UpdateProxyList(cfg.Proxies, selectedID)
}

func (a *App) setLastSelectedProxy(proxyID string) error {
	if proxyID == "" || a.config == nil {
		return nil
	}
	cfg := a.config.GetConfig()
	if cfg.Settings.LastSelectedProxyID == proxyID {
		return nil
	}
	cfg.Settings.LastSelectedProxyID = proxyID
	if err := a.config.SaveConfig(cfg); err != nil {
		return err
	}
	a.refreshTrayProxyList()
	return nil
}

func (a *App) connectFromTray(proxyID string) error {
	if proxyID == "" || a.config == nil {
		return fmt.Errorf("proxy id is empty")
	}
	cfg := a.config.GetConfig()
	var selected *config.ProxyEntry
	for i := range cfg.Proxies {
		if cfg.Proxies[i].ID == proxyID {
			selected = &cfg.Proxies[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("proxy %s not found", proxyID)
	}
	cfg.Settings.LastSelectedProxyID = proxyID
	if err := a.config.SaveConfig(cfg); err != nil {
		return err
	}

	result, err := a.Connect(proxy.ProxyConfig{
		IP:       selected.IP,
		Port:     selected.Port,
		Type:     selected.Type,
		Username: selected.Username,
		Password: selected.Password,
		URI:      selected.URI,
		Extra:    selected.Extra,
	}, cfg.RoutingRules, cfg.Settings.KillSwitch, cfg.Settings.AdBlock)
	if err != nil {
		return err
	}
	if !result.Success {
		return errors.New(result.Message)
	}
	a.refreshTrayProxyList()
	return nil
}

func (a *App) resolveProxyID(proxyDTO proxy.ProxyConfig) string {
	if a.config == nil {
		return ""
	}
	cfg := a.config.GetConfig()
	for _, p := range cfg.Proxies {
		if p.IP == proxyDTO.IP && p.Port == proxyDTO.Port && strings.EqualFold(p.Type, proxyDTO.Type) {
			return p.ID
		}
	}
	return ""
}

func (a *App) startTrayPingLoop() {
	if a.ctx == nil || a.tray == nil || a.config == nil || a.proxy == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				cfg := a.config.GetConfig()
				if len(cfg.Proxies) == 0 {
					continue
				}
				pings := make(map[string]int64, len(cfg.Proxies))
				for _, p := range cfg.Proxies {
					res := a.proxy.Ping(p.IP, p.Port, p.Type)
					if res.Reachable {
						pings[p.ID] = res.LatencyMs
					} else {
						pings[p.ID] = -1
					}
				}
				a.tray.UpdateProxyPings(pings)
			}
		}
	}()
}

func (a *App) initSmartBlockedDomains(userDataPath, rootDir string) {
	if a.proxy == nil {
		return
	}
	cachePath := filepath.Join(userDataPath, "blocked_cache.json")
	localPaths := []string{
		filepath.Join(rootDir, "list-general.txt"),
		filepath.Join(rootDir, "list-google.txt"),
	}
	a.smartProvider = proxy.NewHTTPBlockedListProvider()
	result := proxy.ResolveBlockedDomains(a.ctx, a.smartProvider, cachePath, localPaths...)
	router := a.proxy.GetRouter()
	if router != nil && len(result.Domains) > 0 {
		router.SetBlockedDomains(result.Domains)
	}
	if result.Country != "" {
		a.log.Info(fmt.Sprintf("[SMART] Источник списков: %s (%s), записей: %d", result.Source, strings.ToUpper(result.Country), len(result.Domains)))
	} else {
		a.log.Info(fmt.Sprintf("[SMART] Источник списков: %s, записей: %d", result.Source, len(result.Domains)))
	}
	if result.Err != nil {
		a.log.Warning(fmt.Sprintf("[SMART] Fallback: %v", result.Err))
	}
	a.startSmartBlockedRefresh(cachePath)
}

func (a *App) startSmartBlockedRefresh(cachePath string) {
	if a.ctx == nil || a.proxy == nil || a.smartProvider == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				res := proxy.RefreshRemoteBlockedDomains(a.ctx, a.smartProvider, cachePath)
				if res.Err != nil {
					a.log.Warning(fmt.Sprintf("[SMART] Не удалось обновить списки: %v", res.Err))
					continue
				}
				router := a.proxy.GetRouter()
				if router != nil && len(res.Domains) > 0 {
					router.SetBlockedDomains(res.Domains)
				}
				if res.Country != "" {
					a.log.Info(fmt.Sprintf("[SMART] Списки обновлены (%s), записей: %d", strings.ToUpper(res.Country), len(res.Domains)))
				} else {
					a.log.Info(fmt.Sprintf("[SMART] Списки обновлены, записей: %d", len(res.Domains)))
				}
			}
		}
	}()
}
