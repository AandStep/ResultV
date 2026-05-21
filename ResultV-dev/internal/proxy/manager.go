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
	"resultproxy-wails/internal/system/processtree"
)

type StatusDTO struct {
	IsConnected    bool `json:"isConnected"`
	IsEstablishing bool `json:"isEstablishing"`
	IsProxyDead    bool `json:"isProxyDead"`
	// KillSwitchEmergency is true only for real kill-switch incidents:
	// upstream is dead while kill switch is armed.
	KillSwitchEmergency bool         `json:"killSwitchEmergency"`
	CurrentProxy        *ProxyConfig `json:"currentProxy"`
	Mode                ProxyMode    `json:"mode"`
	Uptime              int64        `json:"uptime"`
	BytesReceived       int64        `json:"bytesReceived"`
	BytesSent           int64        `json:"bytesSent"`
	SpeedReceived       int64        `json:"speedReceived"`
	SpeedSent           int64        `json:"speedSent"`
	KillSwitchActive    bool         `json:"killSwitchActive"`
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
	sysDNS   SystemDNS

	connected    bool
	mode         ProxyMode
	proxy        *ProxyConfig
	pendingProxy *ProxyConfig
	pendingMode  ProxyMode
	killSwitch   bool
	adBlock      bool
	routingMode  RoutingMode
	whitelist    []string
	appWhitelist []string
	connectedAt  time.Time

	prevUp   int64
	prevDown int64
	lastTick time.Time

	localPort         int
	listenLAN         bool
	dnsServers        []string
	tunIPv4           string
	tunIPv6           string
	tunStack          string
	dnsLeakProtection bool

	// connect cancellation — guarded by connectCancelMu (separate from mu
	// so Disconnect/GetStatus can call CancelConnect without deadlock)
	connectCancelMu sync.Mutex
	connectCancel   context.CancelFunc

	// procTracker watches the OS process tree and feeds child-process exe
	// names back into the engine's app whitelist whenever the user has
	// excluded a parent app (Steam, Discord, etc.). Lifecycle is bound to
	// the connection: started on Connect, stopped on Disconnect.
	procTracker *processtree.Monitor

	// proxyDead is set by the health watchdog after consecutive probe
	// failures against the proxy server. It surfaces "VPN is dead but the
	// sing-box process is still alive" to the UI so it can offer to drop
	// the connection + clear firewall rules. Reset to false on Connect and
	// on probe recovery.
	proxyDead    bool
	healthCancel context.CancelFunc

	// Optional OS firewall hooks: engaged only when the watchdog first marks the
	// upstream dead while kill switch is armed — not on routine Connect.
	KillSwitchFirewallEngage    func(ProxyConfig, []string)
	KillSwitchFirewallDisengage func()

	adBlockCoord AdBlockCoordinator
	mitmPort     int
}

var pingTCPProbe = PingProxy
var pingLANProbe = PingProxyLANBind
var pingHysteria2Probe = PingHysteria2QUIC
var pingHysteria2LANProbe = PingHysteria2QUICLANBind
var pingWireGuardProbe = PingProxyUDP
var pingWireGuardLANProbe = PingProxyUDPLANBind
var probeTunnelHTTPProbe = probeHTTPDirect
var probeHTTPThroughProxyProbe = probeHTTPThroughProxy
var isAdminCheck = sys.IsAdmin

// tunnelProbeDomains are the hostnames used by post-start HTTP probes.
// They're exported so buildRoute can force them through the proxy outbound,
// overriding the self-direct rule — otherwise the probe from the app's own
// process would bypass the tunnel and falsely report success.
var tunnelProbeDomains = []string{
	"connectivitycheck.gstatic.com",
	"www.msftconnecttest.com",
	"cp.cloudflare.com",
}

func tunnelProbeURLs() []string {
	out := make([]string, 0, len(tunnelProbeDomains))
	for _, d := range tunnelProbeDomains {
		path := "/generate_204"
		if d == "www.msftconnecttest.com" {
			path = "/connecttest.txt"
		}
		out = append(out, "http://"+d+path)
	}
	return out
}

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
	m.sysDNS = NewSystemDNS()
	m.procTracker = processtree.New(nil)
	m.procTracker.OnChange(m.onProcessTreeChange)

	// Crash recovery: if a prior run crashed mid-session with the system DNS
	// pointing at our resolvers, the snapshot on disk is the only way to give
	// the user back their original DNS. Best-effort — failure here mustn't
	// block app startup.
	if err := m.sysDNS.Restore(); err != nil {
		m.log.Warning(fmt.Sprintf("[СИСТЕМА] Не удалось восстановить DNS из снапшота: %v", err))
	}
}

// effectiveAppWhitelist merges user-specified roots with currently-running
// descendants discovered by the process tree scan. Returns a deduped, ordered
// list. Used at engine boot so the initial config already excludes the full
// process family — without it the user would see one immediate hot-reload
// right after every connect.
func (m *Manager) effectiveAppWhitelist(userRoots []string) []string {
	if len(userRoots) == 0 {
		return nil
	}
	snap := processtree.Scan(userRoots)
	return mergeAppWhitelist(userRoots, snap.Descendants)
}

// warnProbeDomainOverlap logs a one-line notice when the user's
// domain-whitelist intersects with the tunnel health-probe domains.
// Probe-domain rules in buildRoute precede the whitelist rules, so
// e.g. ".gstatic.com" in the whitelist won't actually route
// connectivitycheck.gstatic.com via direct — that one host is forced
// through the proxy for the post-start probe. Logging keeps the user
// from blaming the whitelist for "not working".
func (m *Manager) warnProbeDomainOverlap(mode ProxyMode, proxyTypeLower string, whitelist []string) {
	if m == nil || m.log == nil {
		return
	}
	if mode != ProxyModeTunnel {
		return
	}
	if proxyTypeLower == "wireguard" || proxyTypeLower == "amneziawg" {
		// Endpoint protocols skip the probe-domain route override, so
		// there's nothing to warn about.
		return
	}
	hits := OverlappingProbeDomains(whitelist)
	if len(hits) == 0 {
		return
	}
	m.log.Warning(fmt.Sprintf(
		"[PROXY] Whitelist %v совпадает с probe-доменами (%v). Они принудительно идут через прокси первые ~1-2 сек для health-check — это не баг whitelist.",
		hits, tunnelProbeDomains,
	))
}

// mergeAppWhitelist returns user roots followed by any descendants that
// aren't already in the roots list. Descendants are matched case-insensitively
// against root basenames, so "Steam.exe" + "steam.exe" descendant collapses
// to one entry.
func mergeAppWhitelist(roots, descendants []string) []string {
	if len(roots) == 0 && len(descendants) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(roots)+len(descendants))
	out := make([]string, 0, len(roots)+len(descendants))
	for _, r := range roots {
		key := strings.ToLower(strings.TrimSpace(r))
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r)
	}
	for _, d := range descendants {
		key := strings.ToLower(strings.TrimSpace(d))
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, d)
	}
	return out
}

// onProcessTreeChange is the tracker callback. We deliberately do NOT
// hot-reload sing-box from here: every reload tears down the TUN device
// and breaks established TLS/QUIC sessions (Steam game traffic, Discord
// voice, etc.), which is more disruptive than the missed exclusion.
//
// Instead we just log discovered descendants for diagnostics. The Connect
// pre-scan already captures any process running at connection time, which
// is the common case (user starts Steam, then connects). For descendants
// that spawn after connect, the user reconnects to apply — same UX as
// every other VPN client.
//
// We keep the tracker running so the data is available for a future
// "Apply discovered exclusions" UI button if we ever want one.
func (m *Manager) onProcessTreeChange(snap processtree.Snapshot) {
	if len(snap.Descendants) == 0 {
		return
	}
	m.mu.Lock()
	known := m.appWhitelist
	connected := m.connected
	m.mu.Unlock()

	if !connected {
		return
	}

	known_set := make(map[string]struct{}, len(known))
	for _, k := range known {
		known_set[strings.ToLower(strings.TrimSpace(k))] = struct{}{}
	}
	var fresh []string
	for _, d := range snap.Descendants {
		if _, ok := known_set[strings.ToLower(d)]; ok {
			continue
		}
		fresh = append(fresh, d)
	}
	if len(fresh) == 0 {
		return
	}
	m.log.Info(fmt.Sprintf("[PROXY] Обнаружены новые дочерние процессы (не применены): %s. Переподключитесь чтобы добавить их в исключения.",
		strings.Join(fresh, ", ")))
}

// startProcessTrackerLocked configures the tracker for the current user
// whitelist and starts the watcher goroutine. Caller must hold m.mu.
// No-op if user whitelist is empty (nothing to track).
func (m *Manager) startProcessTrackerLocked() {
	if m.procTracker == nil {
		return
	}
	if len(m.appWhitelist) == 0 {
		m.procTracker.Stop()
		return
	}
	m.procTracker.SetRoots(m.appWhitelist)
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	m.procTracker.Start(ctx)
}

func (m *Manager) stopProcessTrackerLocked() {
	if m.procTracker == nil {
		return
	}
	m.procTracker.Stop()
}

func (m *Manager) LoadBlockedLists(paths ...string) {
	m.router.LoadBlockedLists(paths...)
}

// SetAdBlockCoordinator wires HTTPS MITM filter lifecycle (optional).
func (m *Manager) SetAdBlockCoordinator(c AdBlockCoordinator) {
	m.mu.Lock()
	m.adBlockCoord = c
	m.mu.Unlock()
}

func (m *Manager) prepareAdBlock(cfg *EngineConfig, adBlock bool, upstreamPort int) error {
	m.stopAdBlockMITM()
	cfg.MITMPort = 0
	if !adBlock {
		cfg.AdBlock = false
		return nil
	}
	cfg.AdBlock = true
	// HTTPS MITM is not wired through sing-box outbounds: routing browser TLS to a
	// local http outbound breaks tunnel mode (Steam, games, WebView) with
	// "unexpected EOF" and is fragile with process detection on Windows.
	// Network blocking via sing-box rule_set reject remains active.
	_ = upstreamPort
	return nil
}

func (m *Manager) stopAdBlockMITM() {
	if m.adBlockCoord != nil {
		m.adBlockCoord.StopMITM()
	}
	m.mitmPort = 0
}

// setConnectCancel stores the cancel func for the active Connect operation.
func (m *Manager) setConnectCancel(cancel context.CancelFunc) {
	m.connectCancelMu.Lock()
	m.connectCancel = cancel
	m.connectCancelMu.Unlock()
}

// CancelConnect aborts an in-progress Connect call. Safe to call from any goroutine.
// Also stops the engine: Connect() starts sing-box with the app-level ctx (not connectCtx),
// so cancelling the context alone leaves the engine running — the next Connect() would
// fail with "engine already running". Only stops engine if a Connect is actually active
// (connectCancel != nil), to avoid killing an already-established connection.
func (m *Manager) CancelConnect() {
	m.connectCancelMu.Lock()
	cancel := m.connectCancel
	m.connectCancelMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	_ = m.engine.Stop()
}

func (m *Manager) Connect(ctx context.Context, proxy ProxyConfig, mode ProxyMode,
	routingMode RoutingMode, whitelist, appWhitelist []string,
	killSwitch, adBlock bool,
	localPort int, listenLAN bool, dnsServers []string, tunIPv4, tunIPv6 string,
	dnsLeakProtection bool) ConnectResultDTO {

	// ── Phase 1: quick setup under lock ──────────────────────────────────────
	m.mu.Lock()

	if m.connected {
		m.disconnectLocked()
	}

	if proxy.SubscriptionURL != "" {
		m.log.Info(fmt.Sprintf("[PROXY] Подключение (%s)...", proxy.Type))
	} else {
		m.log.Info(fmt.Sprintf("[PROXY] Подключение к %s:%d (%s)...", proxy.IP, proxy.Port, proxy.Type))
	}

	proxyTypeLower := strings.ToLower(strings.TrimSpace(proxy.Type))
	m.log.Info(fmt.Sprintf("[PROXY] Параметры подключения: mode=%s proxyType=%s", mode, proxyTypeLower))

	if proxyTypeLower == "section" {
		m.mu.Unlock()
		return ConnectResultDTO{
			Success:   false,
			Message:   "Эта запись — разделитель в подписке, подключение к ней невозможно.",
			Reason:    "subscription section",
			ErrorCode: ConnectErrorInvalidConfig,
		}
	}

	isEndpointProtocol := proxyTypeLower == "wireguard" || proxyTypeLower == "amneziawg"
	if isEndpointProtocol && mode == ProxyModeProxy {
		m.mu.Unlock()
		return ConnectResultDTO{
			Success:   false,
			Message:   "Протоколы WireGuard и AmneziaWG не поддерживают Proxy-режим. Пожалуйста, включите Tunnel режим.",
			Reason:    "proxy mode not supported for udp endpoints",
			ErrorCode: "proxy_not_supported",
		}
	}

	if mode == ProxyModeTunnel && !isAdminCheck() {
		m.mu.Unlock()
		return ConnectResultDTO{
			Success:      false,
			Message:      "Для tunnel режима нужны права администратора",
			TunnelFailed: true,
			Reason:       "administrator privileges required",
			ErrorCode:    ConnectErrorTunPrivileges,
		}
	}
	if proxyTypeLower != "wireguard" && proxyTypeLower != "amneziawg" && proxyTypeLower != "hysteria2" {
		m.mu.Unlock()
		latency, reachable, _ := PingProxy(proxy.IP, proxy.Port)
		m.mu.Lock()
		if !reachable {
			m.mu.Unlock()
			if proxy.SubscriptionURL != "" {
				m.log.Error("[PROXY] Сервер недоступен")
				return ConnectResultDTO{Success: false, Message: "Сервер недоступен"}
			}
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

	// Pre-scan the OS process tree so the engine boots with all currently-
	// running descendants of the user's excluded apps already in the
	// whitelist. Without this we'd start the engine, then immediately hot-
	// reload as soon as the tracker's first scan discovers existing children.
	effectiveAppWhitelist := m.effectiveAppWhitelist(appWhitelist)

	m.warnProbeDomainOverlap(mode, proxyTypeLower, whitelist)

	engineCfg := EngineConfig{
		Proxy:             proxy,
		Mode:              mode,
		ListenAddr:        fmt.Sprintf("%s:%d", listenHost, actualLocalPort),
		RoutingMode:       routingMode,
		Whitelist:         whitelist,
		AppWhitelist:      effectiveAppWhitelist,
		KillSwitch:        killSwitch,
		LocalPort:         actualLocalPort,
		DNSServers:        dnsServers,
		TunIPv4:           tunIPv4,
		TunIPv6:           tunIPv6,
		TunStack:          m.tunStack,
		DNSLeakProtection: dnsLeakProtection,
		DataDir:           resultProxyDataDir(),
	}
	if err := m.prepareAdBlock(&engineCfg, adBlock, actualLocalPort); err != nil {
		m.mu.Unlock()
		return ConnectResultDTO{
			Success: false,
			Message: fmt.Sprintf("Блокировка рекламы: %v", err),
			Reason:  err.Error(),
		}
	}
	if code, err := validateEngineConfig(engineCfg); err != nil {
		m.stopAdBlockMITM()
		m.mu.Unlock()
		return ConnectResultDTO{
			Success:   false,
			Message:   err.Error(),
			Reason:    err.Error(),
			ErrorCode: code,
		}
	}

	// Release lock before slow engine start + probes so Disconnect/GetStatus
	// remain responsive while the connection is being established.
	m.setPendingLocked(proxy, mode)
	m.mu.Unlock()

	// ── Phase 2: slow operations — no lock held ───────────────────────────────
	// Wrap ctx with a 60-second hard timeout and store the cancel so
	// CancelConnect() (and Disconnect) can abort mid-flight.
	connectCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	m.setConnectCancel(cancel)
	defer func() {
		cancel()
		m.setConnectCancel(nil)
	}()

	// Engine запускается с долгоживущим ctx (контекст приложения), НЕ с connectCtx.
	// connectCtx отменяется когда Connect() возвращается — если передать его движку,
	// sing-box начнёт умирать сразу после установки соединения (DNS context canceled).
	if err := m.engine.Start(ctx, engineCfg); err != nil {
		m.stopAdBlockMITM()
		m.mu.Lock()
		m.clearPendingLocked()
		m.mu.Unlock()
		m.emitStatus()
		tunnelFailed, reason, errorCode := ClassifyEngineStartError(mode, err)
		m.log.Warning(fmt.Sprintf("[PROXY] sing-box не запустился: %v", err))

		proxyType := strings.ToLower(proxy.Type)
		if (proxyType == "http" || proxyType == "https" || proxyType == "socks5" || proxyType == "socks") && m.sysProxy != nil {
			directAddr := fmt.Sprintf("%s:%d", proxy.IP, proxy.Port)
			if setErr := m.sysProxy.Set(directAddr, whitelist); setErr == nil {
				m.log.Info(fmt.Sprintf("[PROXY] Fallback: системный прокси → %s (sing-box недоступен)", directAddr))
				m.mu.Lock()
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
				m.tunIPv6 = tunIPv6
				m.dnsLeakProtection = dnsLeakProtection
				m.clearPendingLocked()
				m.startHealthWatchdogLocked(proxy, mode)
				m.emitStatusLocked()
				m.mu.Unlock()
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

	m.emitStatus()

	proxyExtra := parseExtra(proxy)
	if code, reason := runPostStartProbe(connectCtx, proxyTypeLower, proxy.IP, proxy.Port, actualLocalPort, mode, proxyExtra); code != "" {
		_ = m.engine.Stop()
		m.mu.Lock()
		m.clearPendingLocked()
		m.mu.Unlock()
		m.emitStatus()
		if code == "cancelled" {
			return ConnectResultDTO{
				Success:   false,
				Message:   "Подключение отменено",
				Reason:    reason,
				ErrorCode: code,
			}
		}
		return ConnectResultDTO{
			Success:   false,
			Message:   reason,
			Reason:    reason,
			ErrorCode: code,
		}
	}

	// ── Phase 3: commit state under lock ─────────────────────────────────────
	// Acquire the lock BEFORE clearing connectCancel and BEFORE applying
	// system proxy, so an in-flight Disconnect either runs entirely before
	// us (and we observe engine.IsRunning() == false → bail) or entirely
	// after us (so its engine.Stop() and connected=false win).
	m.mu.Lock()
	if !m.engine.IsRunning() {
		// Disconnect/CancelConnect stopped the engine after the probe passed
		// but before we acquired the lock — abort the commit, treat as cancelled.
		m.mu.Unlock()
		return ConnectResultDTO{
			Success:   false,
			Message:   "Подключение отменено",
			Reason:    "disconnect during commit",
			ErrorCode: "cancelled",
		}
	}
	m.setConnectCancel(nil)

	var gpoConflict bool
	if mode == ProxyModeProxy && m.sysProxy != nil {
		proxyAddr := fmt.Sprintf("127.0.0.1:%d", actualLocalPort)
		if err := m.sysProxy.Set(proxyAddr, whitelist); err != nil {
			m.log.Warning(fmt.Sprintf("[PROXY] Ошибка установки системного прокси: %v", err))
		} else {
			m.log.Success("[СИСТЕМА] Системный прокси применён успешно")
		}
	} else if mode == ProxyModeTunnel && proxyTypeLower == "amneziawg" && m.sysProxy != nil {
		if err := m.sysProxy.Disable(); err != nil {
			m.log.Warning(fmt.Sprintf("[СИСТЕМА] Ошибка сброса системного прокси для туннеля AMNEZIAWG: %v", err))
		}
	}

	// Plug DNS leaks for both modes (Windows-side adapter DNS override):
	//
	//   Proxy mode — apps that don't honor the system HTTP/SOCKS proxy
	//   (WinSCP, ssh, native TCP) keep resolving via the DHCP DNS — the
	//   ISP. Repoint the OS resolver at our upstreams.
	//
	//   Tunnel mode — Windows DNS Client uses Smart Multi-Homed Name
	//   Resolution: queries are sent from every adapter in parallel with
	//   that adapter's configured DNS as destination. Windows binds those
	//   sockets to the originating adapter, bypassing the routing table —
	//   so auto_route's 0.0.0.0/0-via-TUN doesn't pull them in, and the
	//   hijack-dns rule never fires for those packets. Result: queries
	//   leak to whatever DNS the LAN adapter has (ISP). Unifying every
	//   adapter's DNS to our resolvers fixes this: even if a parallel
	//   query escapes via the LAN adapter, it now reaches 1.1.1.1 instead
	//   of Yandex/Rostelecom.
	//
	// WG/AWG endpoint protocols are skipped — wireguard manages DNS via
	// peer config and would race with us. Health-probe domains intentionally
	// keep going through the proxy regardless.
	skipDNSOverride := isEndpointProtocol
	if !skipDNSOverride && m.sysDNS != nil {
		if err := m.sysDNS.Override(dnsOverrideServers(dnsServers)); err != nil {
			m.log.Warning(fmt.Sprintf("[СИСТЕМА] DNS leak protection не применён: %v", err))
		} else {
			m.log.Success("[СИСТЕМА] Системный DNS перенаправлен на защищённые резолверы")
		}
	}

	m.clearPendingLocked()
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
	m.tunIPv6 = tunIPv6
	m.dnsLeakProtection = dnsLeakProtection
	m.startProcessTrackerLocked()
	m.startHealthWatchdogLocked(proxy, mode)
	m.emitStatusLocked()
	m.mu.Unlock()

	if proxy.SubscriptionURL != "" {
		m.log.Success(fmt.Sprintf("[PROXY] Подключено (%s)", proxy.Type))
	} else {
		m.log.Success(fmt.Sprintf("[PROXY] Подключено к %s:%d (%s)", proxy.IP, proxy.Port, proxy.Type))
	}

	return ConnectResultDTO{
		Success:     true,
		Message:     "Подключено",
		GPOConflict: gpoConflict,
	}
}

// dnsOverrideServers picks resolver IPs to push into the OS resolver list
// during a Proxy-mode session. User-configured DNS wins; otherwise default
// to Cloudflare + Google so the user at least leaks to neutral resolvers
// instead of their ISP. Returns IP literals only (the override layer
// rejects anything that isn't a numeric address).
func dnsOverrideServers(custom []string) []string {
	out := make([]string, 0, 2)
	seen := make(map[string]struct{}, len(custom))
	for _, s := range custom {
		host, _ := splitDNSServer(s)
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if _, dup := seen[host]; dup {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	if len(out) == 0 {
		return []string{"1.1.1.1", "8.8.8.8"}
	}
	return out
}

// connectLocked is the internal reconnect path used by SetMode/ReconnectWithRoutingRules.
// Caller must hold m.mu.
func (m *Manager) connectLocked(ctx context.Context, proxy ProxyConfig, mode ProxyMode,
	routingMode RoutingMode, whitelist, appWhitelist []string,
	killSwitch, adBlock bool,
	localPort int, listenLAN bool, dnsServers []string, tunIPv4, tunIPv6 string,
	dnsLeakProtection bool) ConnectResultDTO {
	if m.connected {
		m.disconnectLocked()
	}

	if proxy.SubscriptionURL != "" {
		m.log.Info(fmt.Sprintf("[PROXY] Подключение (%s)...", proxy.Type))
	} else {
		m.log.Info(fmt.Sprintf("[PROXY] Подключение к %s:%d (%s)...", proxy.IP, proxy.Port, proxy.Type))
	}

	proxyTypeLower := strings.ToLower(strings.TrimSpace(proxy.Type))
	isEndpointProtocol := proxyTypeLower == "wireguard" || proxyTypeLower == "amneziawg"

	if isEndpointProtocol && mode == ProxyModeProxy {
		return ConnectResultDTO{
			Success:   false,
			Message:   "Протоколы WireGuard и AmneziaWG не поддерживают Proxy-режим. Пожалуйста, включите Tunnel режим.",
			Reason:    "proxy mode not supported for udp endpoints",
			ErrorCode: "proxy_not_supported",
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

	actualLocalPort := localPort
	if actualLocalPort == 0 {
		actualLocalPort = getFreeLocalPort(14081)
	}
	listenHost := "127.0.0.1"
	if listenLAN {
		listenHost = "0.0.0.0"
	}

	effectiveAppWhitelist := m.effectiveAppWhitelist(appWhitelist)

	m.warnProbeDomainOverlap(mode, proxyTypeLower, whitelist)

	engineCfg := EngineConfig{
		Proxy:             proxy,
		Mode:              mode,
		ListenAddr:        fmt.Sprintf("%s:%d", listenHost, actualLocalPort),
		RoutingMode:       routingMode,
		Whitelist:         whitelist,
		AppWhitelist:      effectiveAppWhitelist,
		KillSwitch:        killSwitch,
		LocalPort:         actualLocalPort,
		DNSServers:        dnsServers,
		TunIPv4:           tunIPv4,
		TunIPv6:           tunIPv6,
		TunStack:          m.tunStack,
		DNSLeakProtection: dnsLeakProtection,
		DataDir:           resultProxyDataDir(),
	}
	if err := m.prepareAdBlock(&engineCfg, adBlock, actualLocalPort); err != nil {
		return ConnectResultDTO{
			Success: false,
			Message: fmt.Sprintf("Блокировка рекламы: %v", err),
			Reason:  err.Error(),
		}
	}
	if code, err := validateEngineConfig(engineCfg); err != nil {
		m.stopAdBlockMITM()
		return ConnectResultDTO{
			Success:   false,
			Message:   err.Error(),
			Reason:    err.Error(),
			ErrorCode: code,
		}
	}

	m.setPendingLocked(proxy, mode)

	if err := m.engine.Start(ctx, engineCfg); err != nil {
		m.stopAdBlockMITM()
		m.clearPendingLocked()
		m.emitStatusLocked()
		tunnelFailed, reason, errorCode := ClassifyEngineStartError(mode, err)
		m.log.Error(fmt.Sprintf("[PROXY] Ошибка запуска движка: %v", err))
		return ConnectResultDTO{
			Success:      false,
			Message:      fmt.Sprintf("Ошибка запуска: %v", err),
			TunnelFailed: tunnelFailed,
			Reason:       reason,
			ErrorCode:    errorCode,
		}
	}

	m.emitStatusLocked()

	probeCtxLocked := ctx
	if probeCtxLocked == nil {
		probeCtxLocked = context.Background()
	}
	proxyExtraLocked := parseExtra(proxy)
	if code, reason := runPostStartProbe(probeCtxLocked, proxyTypeLower, proxy.IP, proxy.Port, actualLocalPort, mode, proxyExtraLocked); code != "" {
		_ = m.engine.Stop()
		m.clearPendingLocked()
		m.emitStatusLocked()
		return ConnectResultDTO{
			Success:   false,
			Message:   reason,
			Reason:    reason,
			ErrorCode: code,
		}
	}

	var gpoConflict bool
	if mode == ProxyModeProxy && m.sysProxy != nil {
		proxyAddr := fmt.Sprintf("127.0.0.1:%d", actualLocalPort)
		if err := m.sysProxy.Set(proxyAddr, whitelist); err != nil {
			m.log.Warning(fmt.Sprintf("[PROXY] Ошибка установки системного прокси: %v", err))
		} else {
			m.log.Success("[СИСТЕМА] Системный прокси применён успешно")
		}
	} else if mode == ProxyModeTunnel && proxyTypeLower == "amneziawg" && m.sysProxy != nil {
		if err := m.sysProxy.Disable(); err != nil {
			m.log.Warning(fmt.Sprintf("[СИСТЕМА] Ошибка сброса системного прокси для туннеля AMNEZIAWG: %v", err))
		}
	}

	// See main Connect() for the full rationale. Tunnel + non-WG/AWG also
	// needs adapter DNS unified to neutralize Smart Multi-Homed Resolution.
	if !isEndpointProtocol && m.sysDNS != nil {
		if err := m.sysDNS.Override(dnsOverrideServers(dnsServers)); err != nil {
			m.log.Warning(fmt.Sprintf("[СИСТЕМА] DNS leak protection не применён: %v", err))
		} else {
			m.log.Success("[СИСТЕМА] Системный DNS перенаправлен на защищённые резолверы")
		}
	}

	m.clearPendingLocked()
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
	m.tunIPv6 = tunIPv6
	m.dnsLeakProtection = dnsLeakProtection
	m.startProcessTrackerLocked()
	m.startHealthWatchdogLocked(proxy, mode)
	m.emitStatusLocked()

	if proxy.SubscriptionURL != "" {
		m.log.Success(fmt.Sprintf("[PROXY] Подключено (%s)", proxy.Type))
	} else {
		m.log.Success(fmt.Sprintf("[PROXY] Подключено к %s:%d (%s)", proxy.IP, proxy.Port, proxy.Type))
	}

	return ConnectResultDTO{
		Success:     true,
		Message:     "Подключено",
		GPOConflict: gpoConflict,
	}
}

func sleepOrCancel(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

func runPostStartProbe(ctx context.Context, proxyTypeLower, ip string, port, localPort int, mode ProxyMode, extra ...map[string]interface{}) (errorCode, reason string) {
	var ex map[string]interface{}
	if len(extra) > 0 {
		ex = extra[0]
	}
	switch proxyTypeLower {
	case "vless", "vmess":
		if mode != ProxyModeProxy {
			return "", ""
		}
		network := getStringField(ex, "network", "tcp")
		if network != "xhttp" && network != "splithttp" {
			return "", ""
		}
		proxyAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
		delays := []time.Duration{400 * time.Millisecond, 800 * time.Millisecond}
		var ok bool
		var r string
		for i := 0; i < 3; i++ {
			if ctx.Err() != nil {
				return "cancelled", "connect cancelled"
			}
			ok, r = probeHTTPThroughProxyProbe(proxyAddr)
			if ok {
				break
			}
			if i < len(delays) {
				if !sleepOrCancel(ctx, delays[i]) {
					return "cancelled", "connect cancelled"
				}
			}
		}
		if !ok {
			if r == "" {
				r = proxyTypeLower + " xhttp e2e probe failed"
			}
			return "post_start_probe_failed", r
		}
	case "hysteria2":
		if mode == ProxyModeProxy {
			proxyAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
			var ok bool
			var r string
			for i := 0; i < 3; i++ {
				if ctx.Err() != nil {
					return "cancelled", "connect cancelled"
				}
				ok, r = probeHTTPThroughProxy(proxyAddr)
				if ok {
					break
				}
				if !sleepOrCancel(ctx, 500*time.Millisecond) {
					return "cancelled", "connect cancelled"
				}
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
		} else if mode == ProxyModeTunnel {
			var ok bool
			var r string
			delays := []time.Duration{300 * time.Millisecond, 600 * time.Millisecond}
			for i := 0; i < 3; i++ {
				if ctx.Err() != nil {
					return "cancelled", "connect cancelled"
				}
				ok, r = probeTunnelHTTPProbe()
				if ok {
					break
				}
				if i < len(delays) {
					if !sleepOrCancel(ctx, delays[i]) {
						return "cancelled", "connect cancelled"
					}
				}
			}
			if !ok {
				_, quicOK, quicR, _ := pingHysteria2Probe(ip, port)
				if quicOK {
					if r == "" {
						r = "tunnel e2e probe failed"
					}
					return "post_start_probe_failed", "proxy outbound misconfigured: " + r
				}
				if quicR == "" {
					quicR = r
				}
				if quicR == "" {
					quicR = "hysteria2 tunnel e2e probe failed"
				}
				return "post_start_probe_failed", quicR
			}
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
			// AmneziaWG scrambles handshake packets (jitter + junk), so the initial
			// handshake takes noticeably longer than plain WireGuard. Give it one
			// extra attempt with a longer final delay.
			isAmnezia := proxyTypeLower == "amneziawg"
			waitDur := 2 * time.Second
			if isAmnezia {
				waitDur = 3 * time.Second
			}
			if !sleepOrCancel(ctx, waitDur) {
				return "cancelled", "connect cancelled"
			}
			attempts := 3
			delays := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second}
			if isAmnezia {
				attempts = 4
				delays = []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second}
			}
			defaultReason := "wireguard e2e probe failed"
			if isAmnezia {
				defaultReason = "amneziawg e2e probe failed"
			}
			var httpOK bool
			var httpReason string
			for i := 0; i < attempts; i++ {
				if ctx.Err() != nil {
					return "cancelled", "connect cancelled"
				}
				httpOK, httpReason = probeTunnelHTTPProbe()
				if httpOK {
					break
				}
				if i < len(delays) {
					if !sleepOrCancel(ctx, delays[i]) {
						return "cancelled", "connect cancelled"
					}
				}
			}
			if !httpOK {
				if httpReason == "" {
					httpReason = defaultReason
				}
				return "post_start_probe_failed", httpReason
			}
		}
		// WG/AWG handled their own tunnel probe above; skip the general one.
		return "", ""
	case "trojan":
		if mode == ProxyModeProxy {
			proxyAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
			var ok bool
			var r string
			// Trojan требует TLS-рукопожатие при первом соединении — это занимает время.
			// Даём 3 попытки с нарастающей паузой чтобы sing-box успел инициализироваться.
			delays := []time.Duration{300 * time.Millisecond, 600 * time.Millisecond}
			for i := 0; i < 3; i++ {
				if ctx.Err() != nil {
					return "cancelled", "connect cancelled"
				}
				ok, r = probeHTTPThroughProxyProbe(proxyAddr)
				if ok {
					break
				}
				if i < len(delays) {
					if !sleepOrCancel(ctx, delays[i]) {
						return "cancelled", "connect cancelled"
					}
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
				if ctx.Err() != nil {
					return "cancelled", "connect cancelled"
				}
				ok, r = probeTunnelHTTPProbe()
				if ok {
					break
				}
				if i < len(delays) {
					if !sleepOrCancel(ctx, delays[i]) {
						return "cancelled", "connect cancelled"
					}
				}
			}
			if !ok {
				if r == "" {
					r = "trojan e2e probe failed"
				}
				return "post_start_probe_failed", r
			}
		}
	case "naiveproxy", "naive":
		if mode == ProxyModeProxy {
			proxyAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
			var ok bool
			var r string
			delays := []time.Duration{300 * time.Millisecond, 600 * time.Millisecond}
			for i := 0; i < 3; i++ {
				if ctx.Err() != nil {
					return "cancelled", "connect cancelled"
				}
				ok, r = probeHTTPThroughProxyProbe(proxyAddr)
				if ok {
					break
				}
				if i < len(delays) {
					if !sleepOrCancel(ctx, delays[i]) {
						return "cancelled", "connect cancelled"
					}
				}
			}
			if !ok {
				if r == "" {
					r = "naiveproxy proxy e2e probe failed"
				}
				return "post_start_probe_failed", r
			}
		} else if mode == ProxyModeTunnel {
			var ok bool
			var r string
			delays := []time.Duration{300 * time.Millisecond, 600 * time.Millisecond}
			for i := 0; i < 3; i++ {
				if ctx.Err() != nil {
					return "cancelled", "connect cancelled"
				}
				ok, r = probeTunnelHTTPProbe()
				if ok {
					break
				}
				if i < len(delays) {
					if !sleepOrCancel(ctx, delays[i]) {
						return "cancelled", "connect cancelled"
					}
				}
			}
			if !ok {
				if r == "" {
					r = "naiveproxy e2e probe failed"
				}
				return "post_start_probe_failed", r
			}
		}
	}

	// General tunnel probe: verify internet works through the TUN before claiming
	// success. Applies to all protocols that don't return early above (SS, VLESS,
	// VMESS, xhttp, etc.) when in tunnel mode.  WG/AWG return "", "" above and
	// never reach this point.  Trojan handles both modes in its own case.
	//
	// SS with AEAD ciphers needs a TCP+key-exchange round-trip on the very first
	// request, which is noticeably slower than subsequent ones. Give the probe
	// 4 attempts (~8s total) instead of 3 to avoid false post_start failures
	// while the first connection warms up.
	if mode == ProxyModeTunnel {
		if !sleepOrCancel(ctx, 2*time.Second) {
			return "cancelled", "connect cancelled"
		}
		delays := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second}
		attempts := 4
		var httpOK bool
		var httpReason string
		for i := 0; i < attempts; i++ {
			if ctx.Err() != nil {
				return "cancelled", "connect cancelled"
			}
			httpOK, httpReason = probeTunnelHTTPProbe()
			if httpOK {
				break
			}
			if i < len(delays) {
				if !sleepOrCancel(ctx, delays[i]) {
					return "cancelled", "connect cancelled"
				}
			}
		}
		if !httpOK {
			if httpReason == "" {
				httpReason = "tunnel e2e probe failed"
			}
			return "post_start_probe_failed", httpReason
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

	targets := tunnelProbeURLs()
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
	targets := tunnelProbeURLs()
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
	// Abort any in-progress Connect so its goroutines stop.
	m.CancelConnect()

	// Stop engine unconditionally before acquiring the lock.
	// During Phase 2 of Connect(), the engine may already be running while
	// m.connected is still false.  disconnectLocked() only stops the engine
	// when m.connected==true, so without this explicit call a mid-connect
	// Disconnect() would leave the engine alive, causing the next Connect()
	// to fail with "engine already running".
	_ = m.engine.Stop()
	m.stopAdBlockMITM()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopProcessTrackerLocked()
	m.stopHealthWatchdogLocked()

	if m.sysProxy != nil {
		if err := m.sysProxy.Disable(); err != nil {
			m.log.Warning(fmt.Sprintf("[СИСТЕМА] Ошибка отключения прокси: %v", err))
		} else if m.connected {
			m.log.Info("[СИСТЕМА] Системный прокси отключен")
		}
	}

	if m.sysDNS != nil {
		if err := m.sysDNS.Restore(); err != nil {
			m.log.Warning(fmt.Sprintf("[СИСТЕМА] Ошибка восстановления DNS: %v", err))
		}
	}

	if m.connected {
		m.log.Info("[PROXY] Отключение...")
	}
	m.connected = false
	m.proxy = nil
	m.clearPendingLocked()
	m.emitStatusLocked()
	return nil
}

func (m *Manager) disconnectLocked() error {
	if !m.connected {
		return nil
	}

	m.log.Info("[PROXY] Отключение...")

	m.stopProcessTrackerLocked()
	m.stopHealthWatchdogLocked()
	m.stopAdBlockMITM()

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

	if m.sysDNS != nil {
		if err := m.sysDNS.Restore(); err != nil {
			m.log.Warning(fmt.Sprintf("[СИСТЕМА] Ошибка восстановления DNS: %v", err))
		}
	}

	m.connected = false
	m.proxy = nil
	m.clearPendingLocked()

	m.emitStatusLocked()

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
			m.tunIPv6,
			m.dnsLeakProtection,
		)
		if !res.Success {
			return fmt.Errorf("reconnect after mode switch failed: %s", res.Message)
		}
	}

	return nil
}

func (m *Manager) SetTunStack(stack string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tunStack = stack
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
	tIPv6 := m.tunIPv6
	dnsLeak := m.dnsLeakProtection

	return m.connectLocked(ctx, p, mode, routingMode, whitelist, appWhitelist, killSwitch, adBlock, lPort, listenLAN, dServers, tIPv4, tIPv6, dnsLeak)
}

// IsAdBlockActive reports whether the running sing-box engine has ad blocking enabled.
func (m *Manager) IsAdBlockActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.adBlock
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

	return m.buildStatusLocked(uptime, bytesDown, bytesUp, speedDown, speedUp)
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
	} else if connected && mode == ProxyModeTunnel {
		if isHysteria2 {
			latency, reachable, reason, checkType = pingHysteria2LANProbe(ip, port)
		} else if isWireGuard {
			latency, reachable, reason = pingWireGuardLANProbe(ip, port)
			checkType = "udp_lan_bind"
		} else {
			latency, reachable, reason = pingLANProbe(ip, port)
			checkType = "tcp_lan_bind"
		}
	} else if isHysteria2 {
		latency, reachable, reason, checkType = pingHysteria2Probe(ip, port)
	} else if isWireGuard {
		latency, reachable, reason = pingWireGuardProbe(ip, port)
		checkType = "udp"
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

// startHealthWatchdogLocked starts a goroutine that pings the proxy server
// every 5 seconds. After two consecutive failures it flips m.proxyDead and
// emits a status event so the frontend can show the emergency modal — sing-box
// can keep its local listener alive long after the upstream is gone, so
// m.connected by itself is a bad signal.
//
// Caller must hold m.mu. Any in-flight watchdog is cancelled first.
func (m *Manager) startHealthWatchdogLocked(proxy ProxyConfig, mode ProxyMode) {
	if m.healthCancel != nil {
		m.healthCancel()
		m.healthCancel = nil
	}
	m.proxyDead = false

	parentCtx := m.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	healthCtx, cancel := context.WithCancel(parentCtx)
	m.healthCancel = cancel

	go m.runHealthWatchdog(healthCtx, proxy, mode)
}

// stopHealthWatchdogLocked stops the watchdog goroutine and clears the dead
// flag. Caller must hold m.mu.
func (m *Manager) stopHealthWatchdogLocked() {
	if m.healthCancel != nil {
		m.healthCancel()
		m.healthCancel = nil
	}
	m.proxyDead = false
}

func (m *Manager) runHealthWatchdog(ctx context.Context, proxy ProxyConfig, mode ProxyMode) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	const failuresBeforeDead = 2
	consecutiveFails := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		// Snapshot under lock and bail early if disconnected.
		m.mu.Lock()
		if !m.connected {
			m.mu.Unlock()
			return
		}
		ks := m.killSwitch
		m.mu.Unlock()

		// Probe runs without the lock — TCP/UDP dial can block for seconds.
		alive := m.probeProxyAlive(proxy, mode)

		m.mu.Lock()
		// Re-check after the probe: Disconnect may have run while we waited.
		if !m.connected {
			m.mu.Unlock()
			return
		}
		wasDead := m.proxyDead
		if alive {
			consecutiveFails = 0
			var disengageFn func()
			if wasDead {
				m.proxyDead = false
				m.log.Success("[KILL SWITCH] VPN-сервер снова доступен")
				m.emitStatusLocked()
				disengageFn = m.KillSwitchFirewallDisengage
			}
			m.mu.Unlock()
			if disengageFn != nil {
				disengageFn()
			}
			continue
		}
		consecutiveFails++
		var shouldEngage bool
		var engageFn func(ProxyConfig, []string)
		var engageProxy ProxyConfig
		var engageDNS []string
		if consecutiveFails >= failuresBeforeDead && !wasDead {
			m.proxyDead = true
			if ks {
				m.log.Warning(fmt.Sprintf("[KILL SWITCH] VPN-сервер %s:%d недоступен — kill switch блокирует весь трафик", proxy.IP, proxy.Port))
				if m.KillSwitchFirewallEngage != nil {
					shouldEngage = true
					engageFn = m.KillSwitchFirewallEngage
					engageProxy = proxy
					engageDNS = append([]string(nil), m.dnsServers...)
				}
			} else {
				m.log.Warning(fmt.Sprintf("[PROXY] VPN-сервер %s:%d недоступен", proxy.IP, proxy.Port))
			}
			m.emitStatusLocked()
		}
		m.mu.Unlock()
		if shouldEngage && engageFn != nil {
			engageFn(engageProxy, engageDNS)
		}
	}
}

// probeProxyAlive picks the right probe for the proxy's transport. HYSTERIA2
// and WireGuard speak UDP, the rest TCP — a plain TCP connect would falsely
// pass/fail for UDP endpoints.
func (m *Manager) probeProxyAlive(proxy ProxyConfig, mode ProxyMode) bool {
	pt := strings.ToUpper(strings.TrimSpace(proxy.Type))
	if mode == ProxyModeTunnel {
		switch pt {
		case "HYSTERIA2":
			_, reachable, _, _ := pingHysteria2LANProbe(proxy.IP, proxy.Port)
			return reachable
		case "WIREGUARD", "AMNEZIAWG":
			_, reachable, _ := pingWireGuardLANProbe(proxy.IP, proxy.Port)
			return reachable
		default:
			_, reachable, _ := pingLANProbe(proxy.IP, proxy.Port)
			return reachable
		}
	}

	switch pt {
	case "HYSTERIA2":
		_, reachable, _, _ := pingHysteria2Probe(proxy.IP, proxy.Port)
		return reachable
	case "WIREGUARD", "AMNEZIAWG":
		_, reachable, _ := pingWireGuardProbe(proxy.IP, proxy.Port)
		return reachable
	default:
		_, reachable, _ := pingTCPProbe(proxy.IP, proxy.Port)
		return reachable
	}
}

func (m *Manager) ToggleKillSwitch(enable bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.killSwitch = enable

	if !enable && m.sysProxy != nil {

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

	m.stopProcessTrackerLocked()
	m.stopHealthWatchdogLocked()

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

func (m *Manager) setPendingLocked(proxy ProxyConfig, mode ProxyMode) {
	p := proxy
	m.pendingProxy = &p
	m.pendingMode = mode
}

func (m *Manager) clearPendingLocked() {
	m.pendingProxy = nil
	m.pendingMode = ""
}

func (m *Manager) buildStatusLocked(uptime, bytesDown, bytesUp, speedDown, speedUp int64) StatusDTO {
	isEstablishing := m.engine != nil && m.engine.IsRunning() && !m.connected
	currentProxy := m.proxy
	if currentProxy == nil && isEstablishing {
		currentProxy = m.pendingProxy
	}
	mode := m.mode
	if isEstablishing && m.pendingMode != "" {
		mode = m.pendingMode
	}
	return StatusDTO{
		IsConnected:         m.connected,
		IsEstablishing:      isEstablishing,
		IsProxyDead:         m.proxyDead,
		KillSwitchEmergency: m.proxyDead && m.killSwitch,
		CurrentProxy:        currentProxy,
		Mode:                mode,
		Uptime:              uptime,
		BytesReceived:       bytesDown,
		BytesSent:           bytesUp,
		SpeedReceived:       speedDown,
		SpeedSent:           speedUp,
		KillSwitchActive:    m.killSwitch,
	}
}

func (m *Manager) emitStatusLocked() {
	if m.ctx == nil {
		return
	}
	var uptime int64
	if m.connected && !m.connectedAt.IsZero() {
		uptime = int64(time.Since(m.connectedAt).Seconds())
	}
	wailsRuntime.EventsEmit(m.ctx, "status:update", m.buildStatusLocked(uptime, 0, 0, 0, 0))
}

func (m *Manager) emitStatus() {
	if m.ctx == nil {
		return
	}
	m.mu.Lock()
	var uptime int64
	var bytesUp, bytesDown int64
	var speedUp, speedDown int64
	if m.connected {
		uptime = int64(time.Since(m.connectedAt).Seconds())
		bytesUp, bytesDown = m.engine.GetTrafficStats()
	}
	status := m.buildStatusLocked(uptime, bytesDown, bytesUp, speedUp, speedDown)
	m.mu.Unlock()
	wailsRuntime.EventsEmit(m.ctx, "status:update", status)
}
