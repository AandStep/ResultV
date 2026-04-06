// Copyright (C) 2026 ResultProxy
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"resultproxy-wails/internal/adblock"
	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/logger"
	"resultproxy-wails/internal/proxy"
	"resultproxy-wails/internal/system"
)

// App is the main application struct — coordinator of all services.
// Bound methods on this struct become the frontend API via Wails bindings.
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

	// Embedded icon for the system tray (set by main).
	trayIcon []byte
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{
		log:     logger.New(),
		adblock: adblock.New(),
	}
}

// SetTrayIcon sets the icon bytes for the system tray (call before startup).
func (a *App) SetTrayIcon(icon []byte) {
	a.trayIcon = icon
}

// GetVersion returns the application version (wails.json info.productVersion).
func (a *App) GetVersion() string {
	return productVersionFromWailsJSON()
}

// startup is called when the Wails app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)

	// Wire up the logger to push events to the frontend.
	a.log.SetEmitter(func(eventName string, data any) {
		wailsRuntime.EventsEmit(a.ctx, eventName, data)
	})

	a.log.Info("ResultProxy запускается...")

	// Initialize crypto service.
	cs, err := config.NewCryptoService()
	if err != nil {
		a.log.Error(fmt.Sprintf("Ошибка инициализации шифрования: %v", err))
		return
	}
	a.crypto = cs

	// Initialize config manager.
	a.config = config.NewManager(cs)
	userDataPath := a.getUserDataPath()
	if err := a.config.Init(userDataPath); err != nil {
		a.log.Error(fmt.Sprintf("Ошибка загрузки конфигурации: %v", err))
	} else {
		a.log.Success("Конфигурация загружена")
	}

	// Initialize proxy manager.
	a.proxy = proxy.NewManager(a.log)
	a.proxy.Init(a.ctx)

	// Load blocked domain lists for smart mode.
	rootDir := a.getAppRootDir()
	a.proxy.LoadBlockedLists(
		filepath.Join(rootDir, "list-general.txt"),
		filepath.Join(rootDir, "list-google.txt"),
	)

	// Load adblock cache.
	if err := a.adblock.LoadFromCache(userDataPath); err != nil {
		a.log.Warning(fmt.Sprintf("Кэш AdBlock не загружен: %v", err))
	}

	// Initialize kill switch.
	a.killSwitch = system.NewKillSwitch()

	// Initialize network monitor.
	a.netmon = system.NewNetMonitor(func(status system.NetworkStatus) {
		wailsRuntime.EventsEmit(a.ctx, "network:status", status)
		if status.Online {
			a.log.Info("[СЕТЬ] Интернет-соединение восстановлено")
		} else {
			a.log.Warning("[СЕТЬ] Интернет-соединение потеряно")
		}
	})
	a.netmon.Start(a.ctx)

	// Initialize system tray.
	a.tray = system.NewTray(a.trayIcon, system.TrayCallbacks{
		OnShowWindow: func() {
			wailsRuntime.WindowShow(a.ctx)
			wailsRuntime.WindowSetAlwaysOnTop(a.ctx, true)
			wailsRuntime.WindowSetAlwaysOnTop(a.ctx, false)
		},
		OnDisconnect: func() {
			if err := a.Disconnect(); err != nil {
				a.log.Error(fmt.Sprintf("Ошибка отключения из трея: %v", err))
			}
		},
		OnQuit: func() {
			wailsRuntime.Quit(a.ctx)
		},
	})
	a.tray.Start()

	// Check for GPO conflicts.
	if system.DetectGPOConflict() {
		a.log.Warning("[СИСТЕМА] Обнаружен конфликт с групповой политикой (GPO). Настройки прокси могут быть переопределены.")
		wailsRuntime.EventsEmit(a.ctx, "system:gpo-conflict", true)
	}

	a.log.Success("ResultProxy готов к работе")
}

// shutdown is called when the Wails app is closing.
func (a *App) shutdown(ctx context.Context) {
	a.log.Info("ResultProxy завершает работу...")

	// Stop network monitor.
	if a.netmon != nil {
		a.netmon.Stop()
	}

	// Stop system tray.
	if a.tray != nil {
		a.tray.Stop()
	}

	// Disable kill switch if active.
	if a.killSwitch != nil && a.killSwitch.IsEnabled() {
		_ = a.killSwitch.Disable()
	}

	// Critical: clean up proxy and system proxy settings.
	if a.proxy != nil {
		a.proxy.Shutdown()
	}

	if a.cancel != nil {
		a.cancel()
	}
}

// --- Bound methods (frontend API) ---

// GetConfig returns the current application config.
func (a *App) GetConfig() (config.AppConfig, error) {
	if a.config == nil {
		return config.DefaultConfig(), nil
	}
	return a.config.GetConfig(), nil
}

// SaveConfig saves the application config.
func (a *App) SaveConfig(cfg config.AppConfig) error {
	if a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}
	// Partial saves from the frontend: missing key → nil; empty [] can also arrive if React state
	// was not hydrated yet — never replace a non-empty stored list with an empty one.
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

// Connect establishes a proxy connection.
func (a *App) Connect(proxyDTO proxy.ProxyConfig, rules config.RoutingRules,
	killSwitch, adBlock bool) (proxy.ConnectResultDTO, error) {

	if a.proxy == nil {
		return proxy.ConnectResultDTO{Success: false, Message: "Proxy manager not initialized"}, nil
	}

	cfg := a.config.GetConfig()
	mode := proxy.ProxyMode(cfg.Settings.Mode)

	result := a.proxy.Connect(
		a.ctx,
		proxyDTO,
		mode,
		proxy.RoutingMode(rules.Mode),
		rules.Whitelist,
		rules.AppWhitelist,
		killSwitch,
		adBlock,
	)

	// Update tray and emit events on success.
	if result.Success {
		serverName := fmt.Sprintf("%s:%d", proxyDTO.IP, proxyDTO.Port)
		if a.tray != nil {
			a.tray.SetConnected(serverName)
		}
		wailsRuntime.EventsEmit(a.ctx, "proxy:connected", proxyDTO)
	}

	return result, nil
}

// Disconnect stops the proxy connection.
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

// GetStatus returns the current proxy status.
func (a *App) GetStatus() proxy.StatusDTO {
	if a.proxy == nil {
		return proxy.StatusDTO{Mode: proxy.ProxyModeProxy}
	}
	return a.proxy.GetStatus()
}

// SetMode switches the proxy mode (proxy/tunnel).
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

// ApplyMode saves mode and reapplies connection if needed.
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
		result := a.proxy.Connect(
			a.ctx,
			*status.CurrentProxy,
			proxy.ProxyMode(mode),
			proxy.RoutingMode(cfg.RoutingRules.Mode),
			cfg.RoutingRules.Whitelist,
			cfg.RoutingRules.AppWhitelist,
			cfg.Settings.KillSwitch,
			cfg.Settings.AdBlock,
		)
		if result.Success {
			serverName := fmt.Sprintf("%s:%d", status.CurrentProxy.IP, status.CurrentProxy.Port)
			if a.tray != nil {
				a.tray.SetConnected(serverName)
			}
			wailsRuntime.EventsEmit(a.ctx, "proxy:connected", *status.CurrentProxy)
		} else if !result.FallbackUsed {
			// Reconnect fully failed (no fallback) — rollback mode in config.
			cfg.Settings.Mode = previousMode
			_ = a.config.SaveConfig(cfg)
			if a.tray != nil {
				a.tray.SetDisconnected()
			}
			wailsRuntime.EventsEmit(a.ctx, "proxy:disconnected", nil)
		}
		return result, nil
	}

	if err := a.proxy.SetMode(proxy.ProxyMode(mode)); err != nil {
		return proxy.ConnectResultDTO{Success: false, Message: fmt.Sprintf("Ошибка применения режима: %v", err)}, nil
	}
	return proxy.ConnectResultDTO{Success: true, Message: "Режим сохранен"}, nil
}

// GetMode returns the current proxy mode.
func (a *App) GetMode() string {
	if a.proxy == nil {
		return "proxy"
	}
	return string(a.proxy.GetMode())
}

// PingProxy tests proxy server reachability.
func (a *App) PingProxy(ip string, port int) proxy.PingResultDTO {
	if a.proxy == nil {
		return proxy.PingResultDTO{}
	}
	return a.proxy.Ping(ip, port)
}

// GetLogs returns paginated log entries.
func (a *App) GetLogs(page, size int) logger.LogPage {
	return a.log.GetLogs(page, size)
}

// ToggleKillSwitch enables/disables the kill switch.
func (a *App) ToggleKillSwitch(enable bool) error {
	if a.proxy == nil {
		return fmt.Errorf("proxy manager not initialized")
	}

	// Use the enhanced firewall-based kill switch if admin.
	if enable && a.killSwitch != nil {
		status := a.proxy.GetStatus()
		proxyAddr := ""
		if status.CurrentProxy != nil {
			proxyAddr = fmt.Sprintf("%s:%d", status.CurrentProxy.IP, status.CurrentProxy.Port)
		}
		if err := a.killSwitch.Enable(proxyAddr); err != nil {
			a.log.Warning(fmt.Sprintf("[KILL SWITCH] Firewall недоступен, используем fallback: %v", err))
			// Fallback to dead-proxy kill switch in proxy manager.
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

// ToggleAdBlock enables/disables ad blocking.
func (a *App) ToggleAdBlock(enable bool) error {
	if a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}
	cfg := a.config.GetConfig()
	cfg.Settings.AdBlock = enable
	return a.config.SaveConfig(cfg)
}

// SetAutostart enables/disables autostart.
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

// IsAutostartEnabled checks if autostart is configured.
func (a *App) IsAutostartEnabled() bool {
	return system.IsAutostartEnabled()
}

// UpdateRules updates routing rules and reapplies them to sing-box / system proxy if connected.
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
		a.tray.SetConnected(fmt.Sprintf("%s:%d", cur.IP, cur.Port))
	}
	wailsRuntime.EventsEmit(a.ctx, "proxy:connected", cur)
	return nil
}

// ExportConfig exports the current config as a shareable string.
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

// ImportConfig imports config from a Base64 string.
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

// GetPlatform returns the current platform identifier (Go GOOS: windows, darwin, linux, …).
func (a *App) GetPlatform() string {
	return runtime.GOOS
}

// IsAdmin checks if the app is running with admin privileges.
func (a *App) IsAdmin() bool {
	return system.IsAdmin()
}

// RestartAsAdmin restarts the app with elevated privileges.
func (a *App) RestartAsAdmin() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}
	return system.RestartAsAdmin(exe)
}

// GetNetworkTraffic returns current network I/O stats.
func (a *App) GetNetworkTraffic() system.TrafficStats {
	return system.GetNetworkTraffic()
}

// GetNetworkStatus returns current internet connectivity status.
func (a *App) GetNetworkStatus() system.NetworkStatus {
	if a.netmon == nil {
		return system.NetworkStatus{Online: true}
	}
	return a.netmon.GetStatus()
}

// SyncProxies updates the proxy list (used by tray menu).
func (a *App) SyncProxies(proxies []config.ProxyEntry) error {
	if a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}
	cfg := a.config.GetConfig()
	cfg.Proxies = proxies
	return a.config.SaveConfig(cfg)
}

// DetectCountry determines country by IP address via external API.
func (a *App) DetectCountry(ip string) (string, error) {
	// Simple HTTP client to fetch country code from ip-api
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

// parseSubscriptionUserInfoHeader parses the Subscription-Userinfo response header
// (Clash-style: upload=0; download=0; total=0; expire=unix).
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

// subscriptionIconCandidates returns possible icon URLs (headers first, then site favicon, DuckDuckGo).
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
	if parsed != nil && parsed.Hostname() != "" {
		ho := parsed.Hostname()
		add("https://" + ho + "/favicon.ico")
		add("https://icons.duckduckgo.com/ip3/" + ho + ".ico")
	}
	return out
}

// inlineSmallImageFromURL downloads a small image and returns a data: URL (for WebView where remote images may be blocked).
func inlineSmallImageFromURL(client *http.Client, imageURL string) string {
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
	req.Header.Set("User-Agent", "Mozilla/5.0 ResultProxy/2.0")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return ""
	}
	defer resp.Body.Close()
	const maxBytes = 49152
	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil || len(buf) > maxBytes {
		return ""
	}
	ct := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	ct = strings.ToLower(ct)
	if ct == "" || ct == "application/octet-stream" || ct == "binary/octet-stream" {
		ct = http.DetectContentType(buf)
	}
	if !strings.HasPrefix(ct, "image/") {
		return ""
	}
	return "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(buf)
}

func resolveSubscriptionIcon(client *http.Client, subURL string, h http.Header) string {
	cands := subscriptionIconCandidates(subURL, h)
	for _, cand := range cands {
		if data := inlineSmallImageFromURL(client, cand); data != "" {
			return data
		}
	}
	if len(cands) > 0 {
		return cands[0]
	}
	return ""
}

func (a *App) fetchSubscriptionFromURL(subURL string) ([]config.ProxyEntry, int64, int64, int64, int64, string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(subURL)
	if err != nil {
		return nil, 0, 0, 0, 0, "", fmt.Errorf("fetching subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, 0, 0, 0, "", fmt.Errorf("subscription returned HTTP %d", resp.StatusCode)
	}

	up, down, tot, exp := parseSubscriptionUserInfoHeader(resp.Header.Get("Subscription-Userinfo"))
	iconURL := resolveSubscriptionIcon(client, subURL, resp.Header)

	bodyBytes := make([]byte, 0, 1024*64)
	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			bodyBytes = append(bodyBytes, buf[:n]...)
		}
		if readErr != nil {
			break
		}
	}

	entries, err := proxy.ParseSubscriptionBody(string(bodyBytes))
	if err != nil {
		return nil, up, down, tot, exp, iconURL, err
	}

	providerName := extractProviderName(subURL)
	baseID := time.Now().UnixMilli()
	for i := range entries {
		entries[i].SubscriptionURL = subURL
		entries[i].Provider = providerName
		entries[i].ID = fmt.Sprintf("%d", baseID+int64(i))
	}

	a.log.Success(fmt.Sprintf("Подписка загружена: %d серверов", len(entries)))
	return entries, up, down, tot, exp, iconURL, nil
}

// FetchSubscription fetches and parses a subscription URL, returning proxy entries.
func (a *App) FetchSubscription(subURL string) ([]config.ProxyEntry, error) {
	entries, _, _, _, _, _, err := a.fetchSubscriptionFromURL(subURL)
	return entries, err
}

// RefreshSubscription refreshes a subscription by its ID and returns updated entries.
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

	entries, up, down, tot, exp, iconURL, err := a.fetchSubscriptionFromURL(sub.URL)
	if err != nil {
		return nil, fmt.Errorf("refreshing subscription %s: %w", sub.Name, err)
	}

	for i := range entries {
		entries[i].Provider = sub.Name
		entries[i].SubscriptionURL = sub.URL
	}

	for i := range cfg.Subscriptions {
		if cfg.Subscriptions[i].ID == subID {
			cfg.Subscriptions[i].UpdatedAt = time.Now().Format(time.RFC3339)
			cfg.Subscriptions[i].TrafficUpload = up
			cfg.Subscriptions[i].TrafficDownload = down
			cfg.Subscriptions[i].TrafficTotal = tot
			cfg.Subscriptions[i].ExpireUnix = exp
			if iconURL != "" {
				cfg.Subscriptions[i].IconURL = iconURL
			}
			break
		}
	}
	if err := a.config.SaveConfig(cfg); err != nil {
		a.log.Error(fmt.Sprintf("Ошибка сохранения после обновления подписки: %v", err))
	}

	a.log.Success(fmt.Sprintf("Подписка '%s' обновлена: %d серверов", sub.Name, len(entries)))
	return entries, nil
}

// AddSubscription saves a new subscription and fetches its servers.
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

	entries, up, down, tot, exp, iconURL, err := a.fetchSubscriptionFromURL(subURL)
	if err != nil {
		return nil, err
	}

	sub := config.Subscription{
		ID:              fmt.Sprintf("%d", time.Now().UnixMilli()),
		Name:            name,
		URL:             subURL,
		UpdatedAt:       time.Now().Format(time.RFC3339),
		TrafficUpload:   up,
		TrafficDownload: down,
		TrafficTotal:    tot,
		ExpireUnix:      exp,
		IconURL:         iconURL,
	}

	for i := range entries {
		entries[i].Provider = name
	}

	cfg.Subscriptions = append(cfg.Subscriptions, sub)
	if err := a.config.SaveConfig(cfg); err != nil {
		return nil, fmt.Errorf("saving subscription: %w", err)
	}

	a.log.Success(fmt.Sprintf("Подписка '%s' добавлена: %d серверов", name, len(entries)))
	return entries, nil
}

// DeleteSubscription removes a subscription by ID.
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

// --- Helpers ---

// extractProviderName derives a human-readable provider name from a subscription URL.
func extractProviderName(subURL string) string {
	u, err := url.Parse(subURL)
	if err != nil || u.Host == "" {
		return "Subscription"
	}
	host := u.Hostname()
	// Remove common prefixes
	host = strings.TrimPrefix(host, "www.")
	// Use the domain part before the first dot as provider name
	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		name := parts[len(parts)-2]
		// Capitalize first letter
		if len(name) > 0 {
			return strings.ToUpper(name[:1]) + name[1:]
		}
	}
	return host
}

func (a *App) getUserDataPath() string {
	appData := os.Getenv("APPDATA")
	if appData != "" {
		return filepath.Join(appData, "ResultProxy")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ResultProxy")
}

func (a *App) getAppRootDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}
