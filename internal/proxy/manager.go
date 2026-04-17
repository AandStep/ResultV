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

package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"resultproxy-wails/internal/logger"
	sys "resultproxy-wails/internal/system"
)


type StatusDTO struct {
	IsConnected      bool         `json:"isConnected"`
	IsProxyDead      bool         `json:"isProxyDead"`
	CurrentProxy     *ProxyConfig `json:"currentProxy"`
	Mode             ProxyMode    `json:"mode"`
	Uptime           int64        `json:"uptime"`
	BytesReceived    int64        `json:"bytesReceived"`
	BytesSent        int64        `json:"bytesSent"`
	SpeedReceived    int64        `json:"speedReceived"`
	SpeedSent        int64        `json:"speedSent"`
	KillSwitchActive bool         `json:"killSwitchActive"`
}


type ConnectResultDTO struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	GPOConflict  bool   `json:"gpoConflict"`
	TunnelFailed bool   `json:"tunnelFailed"`
	Reason       string `json:"reason"`
	FallbackUsed bool   `json:"fallbackUsed"`
	ErrorCode    string `json:"errorCode,omitempty"`
}


type PingResultDTO struct {
	Reachable bool   `json:"reachable"`
	LatencyMs int64  `json:"latencyMs"`
	Reason    string `json:"reason,omitempty"`
	CheckType string `json:"checkType,omitempty"`
}



type Manager struct {
	mu       sync.Mutex
	ctx      context.Context 
	log      *logger.Logger
	engine   Engine
	router   *Router
	sysProxy SystemProxy

	
	connected    bool
	mode         ProxyMode
	proxy        *ProxyConfig
	killSwitch   bool
	adBlock      bool
	routingMode  RoutingMode
	whitelist    []string
	appWhitelist []string
	connectedAt  time.Time

	
	prevUp   int64
	prevDown int64
	lastTick time.Time

	
	localPort  int
	listenLAN  bool
	dnsServers []string
	tunIPv4    string
}

var pingTCPProbe = PingProxy
var pingLANProbe = PingProxyLANBind
var pingHysteria2Probe = PingHysteria2QUIC
var pingWireGuardProbe = PingProxyUDP
var probeTunnelHTTPProbe = probeHTTPDirect
var probeHTTPThroughProxyProbe = probeHTTPThroughProxy
var isAdminCheck = sys.IsAdmin


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


func (m *Manager) Init(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ctx = ctx
	m.sysProxy = newSystemProxy(m.router)
}


func (m *Manager) LoadBlockedLists(paths ...string) {
	m.router.LoadBlockedLists(paths...)
}


func (m *Manager) Connect(ctx context.Context, proxy ProxyConfig, mode ProxyMode,
	routingMode RoutingMode, whitelist, appWhitelist []string,
	killSwitch, adBlock bool,
	localPort int, listenLAN bool, dnsServers []string, tunIPv4 string) ConnectResultDTO {

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connectLocked(ctx, proxy, mode, routingMode, whitelist, appWhitelist, killSwitch, adBlock, localPort, listenLAN, dnsServers, tunIPv4)
}

func (m *Manager) connectLocked(ctx context.Context, proxy ProxyConfig, mode ProxyMode,
	routingMode RoutingMode, whitelist, appWhitelist []string,
	killSwitch, adBlock bool,
	localPort int, listenLAN bool, dnsServers []string, tunIPv4 string) ConnectResultDTO {
	if m.connected {
		
		m.disconnectLocked()
	}

	m.log.Info(fmt.Sprintf("[PROXY] Подключение к %s:%d (%s)...", proxy.IP, proxy.Port, proxy.Type))

	proxyTypeLower := strings.ToLower(strings.TrimSpace(proxy.Type))
	m.log.Info(fmt.Sprintf("[PROXY] Параметры подключения: mode=%s proxyType=%s", mode, proxyTypeLower))

	
	
	
	isEndpointProtocol := proxyTypeLower == "wireguard" || proxyTypeLower == "amneziawg"
	if isEndpointProtocol && mode == ProxyModeProxy {
		return ConnectResultDTO{
			Success:      false,
			Message:      "Протоколы WireGuard и AmneziaWG не поддерживают Proxy-режим. Пожалуйста, включите Tunnel режим.",
			Reason:       "proxy mode not supported for udp endpoints",
			ErrorCode:    "proxy_not_supported",
		}
	}

	if mode == ProxyModeTunnel && !isAdminCheck() {
		return ConnectResultDTO{
			Success:      false,
			Message:      "Для tunnel режима нужны права администратора",
			TunnelFailed: true,
			Reason:       "administrator privileges required",
			ErrorCode:    ConnectErrorTunPrivileges,
		}
	}
	if proxyTypeLower != "wireguard" && proxyTypeLower != "amneziawg" && proxyTypeLower != "hysteria2" {
		latency, reachable, _ := PingProxy(proxy.IP, proxy.Port)
		if !reachable {
			m.log.Error(fmt.Sprintf("[PROXY] Сервер %s:%d недоступен", proxy.IP, proxy.Port))
			return ConnectResultDTO{
				Success: false,
				Message: fmt.Sprintf("Сервер %s:%d недоступен", proxy.IP, proxy.Port),
			}
		}
		m.log.Info(fmt.Sprintf("[PROXY] Пинг: %dms", latency))
	}

	actualLocalPort := localPort
	if actualLocalPort == 0 {
		actualLocalPort = getFreeLocalPort(14081)
	}

	listenHost := "127.0.0.1"
	if listenLAN {
		listenHost = "0.0.0.0"
	}

	
	engineCfg := EngineConfig{
		Proxy:        proxy,
		Mode:         mode,
		ListenAddr:   fmt.Sprintf("%s:%d", listenHost, actualLocalPort),
		RoutingMode:  routingMode,
		Whitelist:    whitelist,
		AppWhitelist: appWhitelist,
		AdBlock:      adBlock,
		KillSwitch:   killSwitch,
		LocalPort:    actualLocalPort,
		DNSServers:   dnsServers,
		TunIPv4:      tunIPv4,
		DataDir:      resultProxyDataDir(),
	}
	if code, err := validateEngineConfig(engineCfg); err != nil {
		return ConnectResultDTO{
			Success:   false,
			Message:   err.Error(),
			Reason:    err.Error(),
			ErrorCode: code,
		}
	}

	if err := m.engine.Start(ctx, engineCfg); err != nil {
		tunnelFailed, reason, errorCode := ClassifyEngineStartError(mode, err)
		m.log.Warning(fmt.Sprintf("[PROXY] sing-box не запустился: %v", err))

		
		
		proxyType := strings.ToLower(proxy.Type)
		if (proxyType == "http" || proxyType == "https" || proxyType == "socks5" || proxyType == "socks") && m.sysProxy != nil {
			directAddr := fmt.Sprintf("%s:%d", proxy.IP, proxy.Port)
			if setErr := m.sysProxy.Set(directAddr, whitelist); setErr == nil {
				m.log.Info(fmt.Sprintf("[PROXY] Fallback: системный прокси → %s (sing-box недоступен)", directAddr))
				m.connected = true
				m.mode = mode
				m.proxy = &proxy
				m.killSwitch = killSwitch
				m.adBlock = adBlock
				m.routingMode = routingMode
				m.whitelist = append([]string(nil), whitelist...)
				m.appWhitelist = append([]string(nil), appWhitelist...)
				m.connectedAt = time.Now()
				m.prevUp = 0
				m.prevDown = 0
				m.lastTick = time.Time{}
					m.localPort = actualLocalPort
					m.listenLAN = listenLAN
				m.dnsServers = dnsServers
				m.tunIPv4 = tunIPv4
				m.emitStatus()
				return ConnectResultDTO{
					Success:      true,
					Message:      fmt.Sprintf("Подключено с ограничениями (туннель не запущен): %s", directAddr),
					TunnelFailed: tunnelFailed,
					Reason:       reason,
					FallbackUsed: true,
					ErrorCode:    errorCode,
				}
			}
		}

		m.log.Error(fmt.Sprintf("[PROXY] Ошибка запуска движка: %v", err))
		return ConnectResultDTO{
			Success:      false,
			Message:      fmt.Sprintf("Ошибка запуска: %v", err),
			TunnelFailed: tunnelFailed,
			Reason:       reason,
			ErrorCode:    errorCode,
		}
	}
	
	
	
	
	shouldRunPostStartProbe := mode != ProxyModeTunnel || isEndpointProtocol || proxyTypeLower == "trojan"
	if shouldRunPostStartProbe {
		if code, reason := runPostStartProbe(proxyTypeLower, proxy.IP, proxy.Port, actualLocalPort, mode); code != "" {
			_ = m.engine.Stop()
			return ConnectResultDTO{
				Success:   false,
				Message:   reason,
				Reason:    reason,
				ErrorCode: code,
			}
		}
	}

	
	var gpoConflict bool
	if mode == ProxyModeProxy && m.sysProxy != nil {
		proxyAddr := fmt.Sprintf("127.0.0.1:%d", actualLocalPort)
		if err := m.sysProxy.Set(proxyAddr, whitelist); err != nil {
			m.log.Warning(fmt.Sprintf("[PROXY] Ошибка установки системного прокси: %v", err))
			
		} else {
			m.log.Success("[СИСТЕМА] Прокси применен к Windows успешно")
		}
	} else if mode == ProxyModeTunnel && proxyTypeLower == "amneziawg" && m.sysProxy != nil {
		if err := m.sysProxy.Disable(); err != nil {
			m.log.Warning(fmt.Sprintf("[СИСТЕМА] Ошибка сброса системного прокси для туннеля AMNEZIAWG: %v", err))
		}
	}

	m.connected = true
	m.mode = mode
	m.proxy = &proxy
	m.killSwitch = killSwitch
	m.adBlock = adBlock
	m.routingMode = routingMode
	m.whitelist = append([]string(nil), whitelist...)
	m.appWhitelist = append([]string(nil), appWhitelist...)
	m.connectedAt = time.Now()
	m.prevUp = 0
	m.prevDown = 0
	m.lastTick = time.Time{}
	m.localPort = actualLocalPort
	m.listenLAN = listenLAN
	m.dnsServers = dnsServers
	m.tunIPv4 = tunIPv4

	m.emitStatus()

	m.log.Success(fmt.Sprintf("[PROXY] Подключено к %s:%d (%s)", proxy.IP, proxy.Port, proxy.Type))

	return ConnectResultDTO{
		Success:     true,
		Message:     "Подключено",
		GPOConflict: gpoConflict,
	}
}

func runPostStartProbe(proxyTypeLower, ip string, port, localPort int, mode ProxyMode) (errorCode, reason string) {
	switch proxyTypeLower {
	case "hysteria2":
		
		
		
		proxyAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
		var ok bool
		var r string
		
		
		for i := 0; i < 3; i++ {
			ok, r = probeHTTPThroughProxy(proxyAddr)
			if ok {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if !ok {
			
			_, quicOK, quicR, _ := pingHysteria2Probe(ip, port)
			if quicOK {
				
				return "post_start_probe_failed", "proxy outbound misconfigured: " + r
			}
			
			if quicR == "" {
				quicR = r
			}
			if quicR == "" {
				quicR = "hysteria2 post-start probe failed"
			}
			return "post_start_probe_failed", quicR
		}
	case "wireguard", "amneziawg":
		_, ok, r := pingWireGuardProbe(ip, port)
		if !ok {
			if r == "" {
				r = "wireguard post-start probe failed"
			}
			return "post_start_probe_failed", r
		}
		if mode == ProxyModeTunnel {
			attempts := 1
			delay := 0 * time.Millisecond
			defaultReason := "wireguard e2e probe failed"
			var httpOK bool
			var httpReason string
			for i := 0; i < attempts; i++ {
				httpOK, httpReason = probeTunnelHTTPProbe()
				if httpOK {
					break
				}
				if delay > 0 && i+1 < attempts {
					time.Sleep(delay)
				}
			}
			if !httpOK {
				if httpReason == "" {
					httpReason = defaultReason
				}
				return "post_start_probe_failed", httpReason
			}
		}
	case "trojan":
		if mode == ProxyModeProxy {
			proxyAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
			var ok bool
			var r string
			// Trojan требует TLS-рукопожатие при первом соединении — это занимает время.
			// Даём 3 попытки с нарастающей паузой чтобы sing-box успел инициализироваться.
			delays := []time.Duration{300 * time.Millisecond, 600 * time.Millisecond}
			for i := 0; i < 3; i++ {
				ok, r = probeHTTPThroughProxyProbe(proxyAddr)
				if ok {
					break
				}
				if i < len(delays) {
					time.Sleep(delays[i])
				}
			}
			if !ok {
				if r == "" {
					r = "trojan proxy e2e probe failed"
				}
				return "post_start_probe_failed", r
			}
		} else if mode == ProxyModeTunnel {
			var ok bool
			var r string
			delays := []time.Duration{300 * time.Millisecond, 600 * time.Millisecond}
			for i := 0; i < 3; i++ {
				ok, r = probeTunnelHTTPProbe()
				if ok {
					break
				}
				if i < len(delays) {
					time.Sleep(delays[i])
				}
			}
			if !ok {
				if r == "" {
					r = "trojan e2e probe failed"
				}
				return "post_start_probe_failed", r
			}
		}
	}
	return "", ""
}



func probeHTTPThroughProxy(proxyAddr string) (bool, string) {
	proxyURL, _ := url.Parse("http://" + proxyAddr)
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}

	targets := []string{
		"http://connectivitycheck.gstatic.com/generate_204",
		"http://www.msftconnecttest.com/connecttest.txt",
		"http://cp.cloudflare.com/generate_204",
	}
	lastReason := ""
	for _, target := range targets {
		resp, err := client.Get(target)
		if err != nil {
			lastReason = pingReasonFromError(err)
			continue
		}
		_ = resp.Body.Close()
		// Любой HTTP-ответ (включая 5xx) через прокси означает что туннель работает.
		// 502/503/504 от connectivity-check endpoint'ов — норма при работе через прокси.
		// Только 407 (Proxy Auth Required) означает что прокси сам не принял запрос.
		if isProxyProbeResponseAcceptable(resp.StatusCode) {
			return true, ""
		}
		lastReason = fmt.Sprintf("unexpected status %d from %s", resp.StatusCode, target)
	}
	if lastReason == "" {
		lastReason = "http probe failed"
	}
	return false, lastReason
}

func probeHTTPDirect() (bool, string) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}
	targets := []string{
		"http://connectivitycheck.gstatic.com/generate_204",
		"http://www.msftconnecttest.com/connecttest.txt",
		"http://cp.cloudflare.com/generate_204",
	}
	lastReason := ""
	for _, target := range targets {
		resp, err := client.Get(target)
		if err != nil {
			lastReason = pingReasonFromError(err)
			continue
		}
		_ = resp.Body.Close()
		if isProbeHTTPStatusAcceptable(resp.StatusCode) {
			return true, ""
		}
		lastReason = fmt.Sprintf("unexpected status %d from %s", resp.StatusCode, target)
	}
	if lastReason == "" {
		lastReason = "http probe failed"
	}
	return false, lastReason
}

func isProbeHTTPStatusAcceptable(statusCode int) bool {
	if statusCode == http.StatusProxyAuthRequired {
		return false
	}
	return statusCode >= 200 && statusCode < 500
}

// isProxyProbeResponseAcceptable используется при проверке соединения ЧЕРЕЗ прокси.
// Любой HTTP-ответ (включая 5xx) означает что туннель работает — сервер ответил.
// Connectivity-check endpoint'ы (generate_204, connecttest.txt) могут вернуть 502/503/504
// когда к ним обращаются через прокси — это нормально и не означает неисправность туннеля.
func isProxyProbeResponseAcceptable(statusCode int) bool {
	// 407 = прокси требует авторизацию — это означает что сам прокси не принял запрос
	if statusCode == http.StatusProxyAuthRequired {
		return false
	}
	// Любой другой HTTP-статус означает что соединение прошло через туннель
	return statusCode >= 100
}


func (m *Manager) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.disconnectLocked()
	m.proxy = nil
	return err
}

func (m *Manager) disconnectLocked() error {
	if !m.connected {
		return nil
	}

	m.log.Info("[PROXY] Отключение...")

	
	if err := m.engine.Stop(); err != nil {
		m.log.Error(fmt.Sprintf("[PROXY] Ошибка остановки движка: %v", err))
	}

	
	if m.sysProxy != nil {
		if err := m.sysProxy.Disable(); err != nil {
			m.log.Warning(fmt.Sprintf("[СИСТЕМА] Ошибка отключения прокси: %v", err))
		} else {
			m.log.Info("[СИСТЕМА] Системный прокси отключен")
		}
	}

	m.connected = false
	m.proxy = nil

	
	m.emitStatus()

	return nil
}


func (m *Manager) SetMode(mode ProxyMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.mode == mode {
		return nil
	}

	wasConnected := m.connected
	proxy := m.proxy
	killSwitch := m.killSwitch
	adBlock := m.adBlock
	routingMode := m.routingMode
	whitelist := append([]string(nil), m.whitelist...)
	appWhitelist := append([]string(nil), m.appWhitelist...)

	
	if wasConnected {
		m.disconnectLocked()
	}

	m.mode = mode
	m.log.Info(fmt.Sprintf("[PROXY] Режим изменен: %s", mode))

	
	if wasConnected && proxy != nil {
		res := m.connectLocked(
			m.ctx,
			*proxy,
			mode,
			routingMode,
			whitelist,
			appWhitelist,
			killSwitch,
			adBlock,
			m.localPort,
			m.listenLAN,
			m.dnsServers,
			m.tunIPv4,
		)
		if !res.Success {
			return fmt.Errorf("reconnect after mode switch failed: %s", res.Message)
		}
	}

	return nil
}



func (m *Manager) ReconnectWithRoutingRules(ctx context.Context, routingMode RoutingMode, whitelist, appWhitelist []string) ConnectResultDTO {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected || m.proxy == nil {
		return ConnectResultDTO{Success: true, Message: "not connected"}
	}

	p := *m.proxy
	mode := m.mode
	killSwitch := m.killSwitch
	adBlock := m.adBlock
	lPort := m.localPort
	listenLAN := m.listenLAN
	dServers := m.dnsServers
	tIPv4 := m.tunIPv4

	return m.connectLocked(ctx, p, mode, routingMode, whitelist, appWhitelist, killSwitch, adBlock, lPort, listenLAN, dServers, tIPv4)
}


func (m *Manager) GetStatus() StatusDTO {
	m.mu.Lock()
	defer m.mu.Unlock()

	var uptime int64
	var bytesUp, bytesDown int64
	var speedUp, speedDown int64

	if m.connected {
		uptime = int64(time.Since(m.connectedAt).Seconds())
		bytesUp, bytesDown = m.engine.GetTrafficStats()

		now := time.Now()
		elapsed := now.Sub(m.lastTick).Seconds()
		if elapsed > 0 && !m.lastTick.IsZero() {
			speedDown = int64(float64(bytesDown-m.prevDown) / elapsed)
			speedUp = int64(float64(bytesUp-m.prevUp) / elapsed)
			if speedDown < 0 {
				speedDown = 0
			}
			if speedUp < 0 {
				speedUp = 0
			}
		}
		m.prevDown = bytesDown
		m.prevUp = bytesUp
		m.lastTick = now
	}

	return StatusDTO{
		IsConnected:      m.connected,
		IsProxyDead:      false,
		CurrentProxy:     m.proxy,
		Mode:             m.mode,
		Uptime:           uptime,
		BytesReceived:    bytesDown,
		BytesSent:        bytesUp,
		SpeedReceived:    speedDown,
		SpeedSent:        speedUp,
		KillSwitchActive: m.killSwitch,
	}
}


func (m *Manager) GetMode() ProxyMode {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mode
}



func (m *Manager) Ping(ip string, port int, proxyType string) PingResultDTO {
	m.mu.Lock()
	mode := m.mode
	connected := m.connected
	activeProxy := m.proxy
	m.mu.Unlock()

	var latency int64
	var reachable bool
	var reason string
	var checkType string

	ptUpper := strings.ToUpper(strings.TrimSpace(proxyType))

	
	isActiveProxy := false
	if activeProxy != nil &&
		strings.EqualFold(strings.TrimSpace(activeProxy.IP), strings.TrimSpace(ip)) &&
		activeProxy.Port == port {
		isActiveProxy = true
	}

	isHysteria2 := ptUpper == "HYSTERIA2"
	isWireGuard := ptUpper == "WIREGUARD" || ptUpper == "AMNEZIAWG"
	isUDPProtocol := isHysteria2 || isWireGuard

	
	if connected && isActiveProxy && isUDPProtocol {
		if m.engine != nil && m.engine.IsRunning() {
			reachable = true
			reason = "session_active"
			
			latency = -1
			if isHysteria2 {
				checkType = "hysteria2_session"
			} else {
				checkType = "wireguard_session"
			}
		} else {
			
			if isHysteria2 {
				
				latency, reachable, reason, checkType = pingHysteria2Probe(ip, port)
			} else {
				
				latency, reachable, reason = pingTCPProbe(ip, port)
				checkType = "tcp"
			}
		}
		if !reachable {
			m.log.Warning(fmt.Sprintf("[PING] %s check failed: %s:%d reason=%s", ptUpper, ip, port, reason))
		}
	} else if isHysteria2 {
		
		latency, reachable, reason, checkType = pingHysteria2Probe(ip, port)
	} else if isWireGuard {
		
		latency, reachable, reason = pingTCPProbe(ip, port)
		checkType = "tcp"
	} else if connected && mode == ProxyModeTunnel {
		latency, reachable, reason = pingLANProbe(ip, port)
		checkType = "tcp_lan_bind"
	} else {
		latency, reachable, reason = pingTCPProbe(ip, port)
		checkType = "tcp"
	}

	return PingResultDTO{
		Reachable: reachable,
		LatencyMs: latency,
		Reason:    reason,
		CheckType: checkType,
	}
}


func (m *Manager) ToggleKillSwitch(enable bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.killSwitch = enable

	if enable && !m.connected && m.sysProxy != nil {
		
		if err := m.sysProxy.ApplyKillSwitch(); err != nil {
			return fmt.Errorf("applying kill switch: %w", err)
		}
		m.log.Warning("[KILL SWITCH] Активирована полная блокировка интернета!")
	} else if !enable && m.sysProxy != nil {
		
		if !m.connected {
			if err := m.sysProxy.Disable(); err != nil {
				return fmt.Errorf("disabling kill switch: %w", err)
			}
		}
		m.log.Info("[KILL SWITCH] Деактивирован")
	}

	return nil
}


func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connected {
		m.engine.Stop()
	}

	
	if m.sysProxy != nil {
		m.sysProxy.DisableSync()
	}
}


func (m *Manager) GetRouter() *Router {
	return m.router
}

func (m *Manager) emitStatus() {
	if m.ctx == nil {
		return
	}
	var uptime int64
	if m.connected && !m.connectedAt.IsZero() {
		uptime = int64(time.Since(m.connectedAt).Seconds())
	}
	status := StatusDTO{
		IsConnected:      m.connected,
		IsProxyDead:      false,
		CurrentProxy:     m.proxy,
		Mode:             m.mode,
		Uptime:           uptime,
		KillSwitchActive: m.killSwitch,
	}
	wailsRuntime.EventsEmit(m.ctx, "status:update", status)
}
