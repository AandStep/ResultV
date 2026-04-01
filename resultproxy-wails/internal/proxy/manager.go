// Copyright (C) 2026 ResultProxy
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"context"
	"fmt"
	"sync"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"resultproxy-wails/internal/logger"
)

// StatusDTO is pushed to the frontend via events.
type StatusDTO struct {
	IsConnected  bool        `json:"isConnected"`
	IsProxyDead  bool        `json:"isProxyDead"`
	CurrentProxy *ProxyConfig `json:"currentProxy"`
	Mode         ProxyMode   `json:"mode"`
	Uptime       int64       `json:"uptime"` // seconds
}

// ConnectResultDTO is returned from Connect().
type ConnectResultDTO struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	GPOConflict bool   `json:"gpoConflict"`
}

// PingResultDTO is the result of a proxy ping.
type PingResultDTO struct {
	Reachable bool  `json:"reachable"`
	LatencyMs int64 `json:"latencyMs"`
}

// Manager orchestrates the proxy engine, system proxy, and routing.
// It is the single point of control for connecting/disconnecting.
type Manager struct {
	mu     sync.Mutex
	ctx    context.Context // Wails app context for EventsEmit
	log    *logger.Logger
	engine Engine
	router *Router
	sysProxy SystemProxy

	// Current state.
	connected  bool
	mode       ProxyMode
	proxy      *ProxyConfig
	killSwitch bool
}

// NewManager creates a new proxy Manager.
func NewManager(log *logger.Logger) *Manager {
	router := NewRouter()
	engine := NewSingBoxEngine(log)

	return &Manager{
		log:    log,
		engine: engine,
		router: router,
		mode:   ProxyModeProxy,
	}
}

// Init sets the Wails context and initializes platform-specific components.
func (m *Manager) Init(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ctx = ctx
	m.sysProxy = newSystemProxy(m.router)
}

// LoadBlockedLists loads additional blocked domain lists for smart mode.
func (m *Manager) LoadBlockedLists(paths ...string) {
	m.router.LoadBlockedLists(paths...)
}

// Connect establishes a proxy connection.
func (m *Manager) Connect(ctx context.Context, proxy ProxyConfig, mode ProxyMode,
	routingMode RoutingMode, whitelist, appWhitelist []string,
	killSwitch, adBlock bool) ConnectResultDTO {

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connected {
		// Disconnect first.
		m.disconnectLocked()
	}

	m.log.Info(fmt.Sprintf("[PROXY] Подключение к %s:%d (%s)...", proxy.IP, proxy.Port, proxy.Type))

	// Check proxy reachability.
	latency, reachable := PingProxy(proxy.IP, proxy.Port)
	if !reachable {
		m.log.Error(fmt.Sprintf("[PROXY] Сервер %s:%d недоступен", proxy.IP, proxy.Port))
		return ConnectResultDTO{
			Success: false,
			Message: fmt.Sprintf("Сервер %s:%d недоступен", proxy.IP, proxy.Port),
		}
	}
	m.log.Info(fmt.Sprintf("[PROXY] Пинг: %dms", latency))

	// Configure and start engine.
	engineCfg := EngineConfig{
		Proxy:        proxy,
		Mode:         mode,
		ListenAddr:   "127.0.0.1:14081",
		RoutingMode:  routingMode,
		Whitelist:    whitelist,
		AppWhitelist: appWhitelist,
		AdBlock:      adBlock,
		KillSwitch:   killSwitch,
	}

	if err := m.engine.Start(ctx, engineCfg); err != nil {
		m.log.Error(fmt.Sprintf("[PROXY] Ошибка запуска движка: %v", err))
		return ConnectResultDTO{
			Success: false,
			Message: fmt.Sprintf("Ошибка запуска: %v", err),
		}
	}

	// For proxy mode: set system proxy.
	var gpoConflict bool
	if mode == ProxyModeProxy && m.sysProxy != nil {
		if err := m.sysProxy.Set("127.0.0.1:14081", whitelist); err != nil {
			m.log.Warning(fmt.Sprintf("[PROXY] Ошибка установки системного прокси: %v", err))
			// Not fatal — user can configure manually.
		} else {
			m.log.Success("[СИСТЕМА] Прокси применен к Windows успешно")
		}
	}

	m.connected = true
	m.mode = mode
	m.proxy = &proxy
	m.killSwitch = killSwitch

	// Push status update.
	m.emitStatus()

	m.log.Success(fmt.Sprintf("[PROXY] Подключено к %s:%d (%s)", proxy.IP, proxy.Port, proxy.Type))

	return ConnectResultDTO{
		Success:     true,
		Message:     "Подключено",
		GPOConflict: gpoConflict,
	}
}

// Disconnect stops the proxy connection.
func (m *Manager) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.disconnectLocked()
}

func (m *Manager) disconnectLocked() error {
	if !m.connected {
		return nil
	}

	m.log.Info("[PROXY] Отключение...")

	// Stop engine.
	if err := m.engine.Stop(); err != nil {
		m.log.Error(fmt.Sprintf("[PROXY] Ошибка остановки движка: %v", err))
	}

	// Disable system proxy.
	if m.sysProxy != nil {
		if err := m.sysProxy.Disable(); err != nil {
			m.log.Warning(fmt.Sprintf("[СИСТЕМА] Ошибка отключения прокси: %v", err))
		} else {
			m.log.Info("[СИСТЕМА] Системный прокси отключен")
		}
	}

	m.connected = false
	m.proxy = nil

	// Push status update.
	m.emitStatus()

	return nil
}

// SetMode switches between proxy and tunnel modes (requires reconnect).
func (m *Manager) SetMode(mode ProxyMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.mode == mode {
		return nil
	}

	wasConnected := m.connected
	proxy := m.proxy

	// Disconnect if connected.
	if wasConnected {
		m.disconnectLocked()
	}

	m.mode = mode
	m.log.Info(fmt.Sprintf("[PROXY] Режим изменен: %s", mode))

	// Reconnect if was connected.
	if wasConnected && proxy != nil {
		// We'd need the full config here — for now just update mode.
		// Reconnection logic will be triggered by the frontend.
	}

	return nil
}

// GetStatus returns the current proxy status.
func (m *Manager) GetStatus() StatusDTO {
	m.mu.Lock()
	defer m.mu.Unlock()

	return StatusDTO{
		IsConnected:  m.connected,
		IsProxyDead:  false, // TODO: implement health check
		CurrentProxy: m.proxy,
		Mode:         m.mode,
	}
}

// GetMode returns the current proxy mode.
func (m *Manager) GetMode() ProxyMode {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mode
}

// Ping checks proxy reachability.
func (m *Manager) Ping(ip string, port int) PingResultDTO {
	latency, reachable := PingProxy(ip, port)
	return PingResultDTO{
		Reachable: reachable,
		LatencyMs: latency,
	}
}

// ToggleKillSwitch enables/disables the kill switch.
func (m *Manager) ToggleKillSwitch(enable bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.killSwitch = enable

	if enable && !m.connected && m.sysProxy != nil {
		// Apply kill switch immediately (dead proxy).
		if err := m.sysProxy.ApplyKillSwitch(); err != nil {
			return fmt.Errorf("applying kill switch: %w", err)
		}
		m.log.Warning("[KILL SWITCH] Активирована полная блокировка интернета!")
	} else if !enable && m.sysProxy != nil {
		// Remove kill switch.
		if !m.connected {
			if err := m.sysProxy.Disable(); err != nil {
				return fmt.Errorf("disabling kill switch: %w", err)
			}
		}
		m.log.Info("[KILL SWITCH] Деактивирован")
	}

	return nil
}

// Shutdown performs cleanup during app shutdown.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connected {
		m.engine.Stop()
	}

	// Critical: synchronous proxy cleanup to avoid leaving user without internet.
	if m.sysProxy != nil {
		m.sysProxy.DisableSync()
	}
}

// GetRouter returns the router for external use (e.g., building bypass lists).
func (m *Manager) GetRouter() *Router {
	return m.router
}

func (m *Manager) emitStatus() {
	if m.ctx == nil {
		return
	}
	status := StatusDTO{
		IsConnected:  m.connected,
		IsProxyDead:  false,
		CurrentProxy: m.proxy,
		Mode:         m.mode,
	}
	wailsRuntime.EventsEmit(m.ctx, "status:update", status)
}
