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
	"errors"
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
	// KillSwitchEmergency is true only for real, *enforceable* kill-switch
	// incidents: the upstream is dead, kill switch is armed, AND the app has the
	// admin rights the OS firewall needs to actually block traffic. Without admin
	// there is nothing to "kill", so we report IsProxyDead but not an emergency —
	// otherwise the UI raised a blocking alarm in proxy mode that blocked nothing.
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
	// opMu serializes the heavy connection-lifecycle operations — Connect,
	// Disconnect, SetMode and ReconnectWithRoutingRules — so only one of them
	// ever drives the single sing-box engine at a time. Without it the two
	// frontend hot-reload paths (selectAndConnect → Connect, and the routing-
	// rules / adblock effects → ReconnectWithRoutingRules) overlap and race on
	// engine.Start, producing "engine already running" and wedging the manager
	// in a half-connected "establishing" state the UI shows as an endless spin.
	//
	// Lock ordering: opMu is the OUTER lock — always take it before mu, and
	// never take it while holding mu. Disconnect deliberately calls
	// CancelConnect() BEFORE acquiring opMu so it can abort an in-flight Connect
	// (whose probe is cancellable) instead of blocking behind it.
	opMu sync.Mutex

	mu       sync.Mutex
	ctx      context.Context
	log      *logger.Logger
	engine   Engine
	router   *Router
	sysProxy SystemProxy
	sysDNS   SystemDNS

	// isAdmin is captured once at Init and never changes for a running process.
	// It gates the kill-switch *emergency*: the OS firewall needs admin rights to
	// actually drop traffic, so without admin a dead upstream is surfaced as
	// information rather than a blocking alarm the app cannot back up.
	isAdmin bool

	connected    bool
	mode         ProxyMode
	proxy        *ProxyConfig
	pendingProxy *ProxyConfig
	pendingMode  ProxyMode
	killSwitch   bool
	routingMode  RoutingMode
	whitelist    []string
	appWhitelist []string
	appForceVPN  []string
	routingLists []RoutingListSpec
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
	// healthGen invalidates in-flight watchdog goroutines whose probe is
	// still in progress when a reconnect spawns a fresh watchdog. Cancelling
	// the old context isn't enough on its own — pingTCPProbe / pingHysteria2Probe
	// can block for up to 5s on a dead network, and during that window the
	// old goroutine still holds stale `proxy`/`mode` arguments. Each
	// watchdog snapshots its gen at start and bails out if m.healthGen has
	// moved on by the time it reacquires m.mu.
	healthGen uint64

	// Optional OS firewall hooks: engaged only when the watchdog first marks the
	// upstream dead while kill switch is armed — not on routine Connect.
	KillSwitchFirewallEngage    func(ProxyConfig, []string)
	KillSwitchFirewallDisengage func()


	// secrets encrypts the persistent server-IP pin cache (server_pins.json)
	// with the app's hardware-keyed CryptoService — those hostname→backend-IP
	// entries are exactly what a censor needs, so they never touch disk in the
	// clear. Nil disables persistence entirely (the in-session live capture
	// still works). Wired by SetSecretCodec at startup.
	secrets SecretCodec
}

// SetSecretCodec injects the encryptor used for the persistent server-IP pin
// cache. Must be called before Connect for cross-session pins to persist.
func (m *Manager) SetSecretCodec(c SecretCodec) {
	m.mu.Lock()
	m.secrets = c
	m.mu.Unlock()
}

var pingTCPProbe = PingProxy
var pingLANProbe = PingProxyLANBind
var pingHysteria2Probe = PingHysteria2QUIC
var pingHysteria2LANProbe = PingHysteria2QUICLANBind
var pingHysteria2StrictProbe = PingHysteria2QUICStrict
var pingHysteria2StrictLANProbe = PingHysteria2QUICStrictLANBind
var pingWireGuardProbe = PingWireGuard
var pingWireGuardLANProbe = PingWireGuardLANBind
var probeHTTPThroughProxyProbe = probeHTTPThroughProxy
var probeProxyHealthProbe = probeProxyHealth
var probeTunnelHealthProbe = probeTunnelHealth
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
	m.isAdmin = isAdminCheck()
	m.sysProxy = newSystemProxy(m.router)
	m.sysDNS = NewSystemDNS()
	m.procTracker = processtree.New(nil)
	m.procTracker.OnChange(m.onProcessTreeChange)

	// Leftover system state from a crashed / force-killed prior run (DNS
	// override, system proxy, kill-switch firewall) is intentionally NOT
	// reverted here. It is detected via HasSystemLeftovers and the user is
	// asked at startup whether to remove it (App.CheckLeftovers → dialog →
	// RemoveSystemLeftovers). This keeps the user in control rather than
	// silently changing their network settings on every launch.
}

func (m *Manager) logDNSRestoreWarning(err error) {
	if errors.Is(err, ErrDNSRequiresAdmin) {
		m.log.Warning("[СИСТЕМА] Не удалось восстановить DNS: нужны права администратора. Запустите приложение от имени администратора или подтвердите запрос UAC при восстановлении.")
		return
	}
	m.log.Warning(fmt.Sprintf("[СИСТЕМА] Не удалось восстановить DNS из снапшота: %v", err))
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
	snap := processtree.Scan(basenameRoots(userRoots))
	return mergeAppWhitelist(userRoots, snap.Descendants)
}

// basenameRoots drops path-qualified entries from the process-tree root set.
// processtree.normalizeRoots strips a root down to its basename, so passing
// "Battle.net\Agent\Agent.exe" through would widen it back into a bare
// "agent.exe" and match every unrelated agent on the machine — exactly the
// ambiguity the path-qualified form exists to avoid. Such entries still work as
// route and DNS rules; their parent process is normally a root already, so the
// scan discovers the descendants regardless.
func basenameRoots(entries []string) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.ContainsAny(e, `\/`) {
			continue
		}
		out = append(out, e)
	}
	return out
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

// tunRetryDelay is the settle pause between a failed tunnel-mode engine start and
// the single automatic retry. Between the two we delete the wedged Wintun adapter
// device (removeStaleTunAdapterFn) on top of the failed start releasing its
// partially-created adapter / WFP filters (see SingBoxEngine.bootLocked); this
// brief pause lets Windows finish tearing the device down before the retry
// recreates a fresh adapter. A var, not a const, so tests can zero it out.
var tunRetryDelay = 700 * time.Millisecond

// isTransientTunError reports whether a tunnel-mode engine-start failure is the
// recoverable kind — a stale Wintun adapter or leftover WFP filters from an
// unclean prior exit — rather than a genuine config error. These mirror the
// substrings ClassifyEngineStartError maps to tun_privileges; tunnel mode is
// gated on admin up front, so when they appear post-start they are never about
// privileges, only about a contended/half-torn-down adapter.
func isTransientTunError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "configure tun interface") ||
		strings.Contains(lower, "access is denied") ||
		strings.Contains(lower, "inbound/tun")
}

// startEngine starts the engine for a connect attempt and returns the DTO fields
// a caller needs on failure. In tunnel mode it transparently retries once when
// the first attempt fails with a transient TUN-setup error: before retrying it
// deletes the wedged Wintun adapter device (removeStaleTunAdapterFn) so the
// retry's CreateAdapter gets a clean slate instead of reopening the dead husk —
// the latter is what made the failure stick until a reboot. This recovers the
// common "launch again and it works" case the field reports describe. On
// permanent failure the error code is downgraded from tun_privileges to
// engine_start whenever we are actually elevated — popping a "restart as admin"
// prompt at an already-admin user is the bug being fixed, not the cure.
func (m *Manager) startEngine(ctx context.Context, cfg EngineConfig) (err error, tunnelFailed bool, reason, errorCode string) {
	err = m.engine.Start(ctx, cfg)
	if err != nil && cfg.Mode == ProxyModeTunnel && isTransientTunError(err) {
		m.log.Warning(fmt.Sprintf("[PROXY] TUN не сконфигурировался (%s) — удаление залипшего адаптера и повтор", extractErrorReason(err.Error())))
		if rmErr := removeStaleTunAdapterFn(); rmErr != nil {
			m.log.Warning(fmt.Sprintf("[PROXY] Не удалось удалить залипший TUN-адаптер: %v", rmErr))
		}
		time.Sleep(tunRetryDelay)
		if err = m.engine.Start(ctx, cfg); err == nil {
			m.log.Success("[PROXY] TUN поднялся со второй попытки")
		} else {
			m.log.Warning(fmt.Sprintf("[PROXY] Повторный старт TUN тоже не удался: %s", extractErrorReason(err.Error())))
		}
	}
	if err == nil {
		return nil, false, "", ""
	}
	tunnelFailed, reason, errorCode = ClassifyEngineStartError(cfg.Mode, err)
	if errorCode == ConnectErrorTunPrivileges && isAdminCheck() {
		errorCode = ConnectErrorEngineStart
	}
	return err, tunnelFailed, reason, errorCode
}

// Connect establishes the proxy session. It wraps connectOnce with the
// persistent server-IP pin: for a domain server the OS resolver can't resolve
// (censored DNS), connectOnce's own resolvePinnedServerIP comes back empty and
// sing-box would re-resolve the server per connection via the fragile `local`
// resolver — the cause of false kill-switch trips. If a previous successful
// connect cached the server's real IP (learned from the live socket, see
// captureLiveServerIP), we pin it here so sing-box never re-resolves.
//
// A cached pin can go stale if the server's IP changes (CDN/failover): the
// engine still starts on a dead IP, but the post-start probe fails. We detect
// that, drop the stale entry, and retry once on the bare domain so sing-box
// re-resolves fresh — seamless to the user.
func (m *Manager) Connect(ctx context.Context, proxy ProxyConfig, mode ProxyMode,
	routingMode RoutingMode, whitelist, appWhitelist, appForceVPN []string,
	killSwitch bool,
	localPort int, listenLAN bool, dnsServers []string, tunIPv4, tunIPv6 string,
	dnsLeakProtection bool) ConnectResultDTO {

	dataDir := resultProxyDataDir()
	usedCachedPin := false
	if proxy.ResolvedIP == "" && proxy.IP != "" && net.ParseIP(proxy.IP) == nil {
		if cached := loadServerPin(dataDir, m.secrets, proxy.IP); cached != "" {
			proxy.ResolvedIP = cached
			usedCachedPin = true
		}
	}

	res := m.connectOnce(ctx, proxy, mode, routingMode, whitelist, appWhitelist, appForceVPN,
		killSwitch, localPort, listenLAN, dnsServers, tunIPv4, tunIPv6,
		dnsLeakProtection)

	if shouldRetryWithoutPin(usedCachedPin, res.ErrorCode) {
		clearServerPin(dataDir, m.secrets, proxy.IP)
		m.log.Warning("[PROXY] Закэшированный IP сервера устарел — переподключение по домену")
		proxy.ResolvedIP = ""
		res = m.connectOnce(ctx, proxy, mode, routingMode, whitelist, appWhitelist, appForceVPN,
			killSwitch, localPort, listenLAN, dnsServers, tunIPv4, tunIPv6,
			dnsLeakProtection)
	}
	return res
}

func (m *Manager) connectOnce(ctx context.Context, proxy ProxyConfig, mode ProxyMode,
	routingMode RoutingMode, whitelist, appWhitelist, appForceVPN []string,
	killSwitch bool,
	localPort int, listenLAN bool, dnsServers []string, tunIPv4, tunIPv6 string,
	dnsLeakProtection bool) ConnectResultDTO {

	// Per-phase timing — emitted as one summary line on the success path so the
	// connect budget is measured, not estimated.
	connectStart := time.Now()
	var resolveDur, startDur, probeDur, dnsDur time.Duration

	// Serialize against any other connect/disconnect/reconnect (see opMu). Held
	// for the whole call — including the lock-free slow phase — so two operations
	// can never both reach engine.Start.
	m.opMu.Lock()
	defer m.opMu.Unlock()

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
	// Resolve a domain server to a stable IP up-front, while the OS resolver is
	// still intact (before any system-DNS override). The pinned IP flows into the
	// engine config + watchdog so sing-box never re-resolves the server mid-session.
	// No pre-connect TCP reachability probe: a bare TCP ping is a poor health check
	// for censored/CDN-fronted transports (it false-times-out on a slow-but-alive
	// SYN path and only added latency), so reachability is left to sing-box's own
	// handshake. Off-lock: a flaky resolver must not stall Disconnect/GetStatus.
	m.mu.Unlock()
	tResolve := time.Now()
	// Skip the OS resolve when the IP is already pinned (Connect applied a cached
	// pin, or the caller passed a literal): for a censored server the OS resolve
	// only burns its full timeout to fail, so re-running it on every cached
	// connect would re-introduce the connect-latency the pin exists to avoid.
	if proxy.ResolvedIP == "" {
		if resolved := resolvePinnedServerIP(proxy.IP); resolved != "" {
			proxy.ResolvedIP = resolved
		}
	}
	// Resolve the full backend set for the hosts-pin (see ProxyConfig.ResolvedIPs)
	// so a CDN/multi-IP server can fail over across backends mid-session. Empty for
	// literal IPs or a censored resolver — buildDNS then falls back to the single
	// pin / local rule, never worse than the pre-fix single-IP behaviour.
	if len(proxy.ResolvedIPs) == 0 {
		proxy.ResolvedIPs = resolveAllServerIPs(proxy.IP)
	}
	// DoH fallback: when the OS resolver returns nothing for a domain server (the
	// censored UDP/53 case), resolve the server's own domain via DoH-over-IP on
	// the physical interface — before the system-DNS override / TUN is up — so the
	// route-exclude + server-pin can still be built instead of looping into the
	// TUN. Runs only when the cheap OS path already failed, so a healthy network
	// never pays its latency. Best-effort: empty leaves behaviour unchanged.
	if len(proxy.ResolvedIPs) == 0 && proxy.IP != "" && net.ParseIP(proxy.IP) == nil {
		if doh := resolveServerIPsViaDoH(proxy.IP); len(doh) > 0 {
			proxy.ResolvedIPs = doh
			if proxy.ResolvedIP == "" {
				proxy.ResolvedIP = doh[0]
			}
			m.log.Info("[PROXY] Адрес сервера получен через DoH (OS-резолвер не ответил)")
		}
	}
	resolveDur = time.Since(tResolve)

	// Connect-time diagnostics for the domain-server-in-TUN failure class
	// (github-issue 2026-06): record how the server endpoint resolved so a
	// reporter's logs reveal whether the route-exclude/server-pin could be built.
	if mode == ProxyModeTunnel {
		endpointKind := "ip"
		if net.ParseIP(proxy.IP) == nil {
			endpointKind = "domain"
		}
		m.log.Info(fmt.Sprintf("[PROXY] Эндпоинт сервера: тип=%s пинов=%d tls=%s", endpointKind, len(serverPinnedIPs(proxy)), outboundTLSDiagnostic(proxy)))
	}

	// Fail fast on an unresolvable domain server in TUN: without a pinned IP the
	// route-exclude is empty and sing-box must dial the server via the censored OS
	// resolver — which loops the server's packets back into the TUN (EOF flood) or
	// resolves to a poisoned/CDN-fronted IP (x509-github). Aborting here with a
	// clear reason beats an unrecoverable connection that floods the log and falsely
	// trips the kill switch. mu is unlocked at this point — return directly.
	if serverEndpointUnresolvable(proxy, mode) {
		m.log.Error("[PROXY] Не удалось определить IP сервера (домен, DNS вероятно цензурируется) — подключение отменено")
		return ConnectResultDTO{
			Success:   false,
			Message:   "Не удалось определить IP-адрес сервера. Возможно, DNS блокируется провайдером — попробуйте другой сервер.",
			Reason:    "server domain unresolved (DNS likely censored)",
			ErrorCode: ConnectErrorDNSCensored,
		}
	}

	m.mu.Lock()

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
		AppForceVPN:       append([]string(nil), appForceVPN...),
		KillSwitch:        killSwitch,
		LocalPort:         actualLocalPort,
		DNSServers:        dnsServers,
		TunIPv4:           tunIPv4,
		TunIPv6:           tunIPv6,
		TunStack:          m.tunStack,
		DNSLeakProtection: dnsLeakProtection,
		DataDir:           resultProxyDataDir(),
	}
	engineCfg.RoutingLists = m.routingListSpecsLocked()
	// Smart mode needs the censored block-list in the engine config so
	// buildRoute can tunnel those domains/ranges while everything else goes
	// direct. Only populated for Smart — Global/Whitelist ignore it.
	if routingMode == ModeSmart && m.router != nil {
		engineCfg.BlockedDomains = m.router.GetBlockedDomains()
		engineCfg.BlockedCIDRs = m.router.GetBlockedCIDRs()
		// Pre-compile the block-list into a binary rule-set so the engine does
		// not have to parse and index ~78k domain_suffix entries out of the
		// config on every connect. Not fatal: buildRoute falls back to inline.
		if path, err := CompileSmartRuleSet(engineCfg.DataDir, engineCfg.BlockedDomains); err != nil {
			m.log.Warning(fmt.Sprintf("[SMART] Rule-set не скомпилирован, используется инлайн-список: %v", err))
		} else {
			engineCfg.SmartRuleSetPath = path
		}
	}
	if code, err := validateEngineConfig(engineCfg); err != nil {
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

	// A force-kill of a prior elevated tunnel session leaves an orphan
	// "sing-tun Tunnel" adapter holding a stale default route. sing-box reuses
	// that adapter on the next connect but does NOT clear the stale route, and
	// startup recoverLeftovers may have bailed (it skips while a session is
	// active — the auto-connect-vs-recovery race). Clear it here, before the
	// engine reclaims the adapter: tunnel mode is already admin-gated (checked
	// above), so this runs elevated with no extra UAC prompt. No-op when there is
	// no orphan; the log line doubles as a diagnostic that a leftover was present.
	if mode == ProxyModeTunnel && hasLeftoverTunFn() {
		if err := clearLeftoverTunFn(); err != nil {
			m.log.Warning(fmt.Sprintf("[СИСТЕМА] Не удалось снять остаточный туннель-адаптер перед подключением: %v", err))
		} else {
			m.log.Info("[СИСТЕМА] Снят остаточный туннель-адаптер прошлого сеанса перед подключением")
		}
	}

	// Engine запускается с долгоживущим ctx (контекст приложения), НЕ с connectCtx.
	// connectCtx отменяется когда Connect() возвращается — если передать его движку,
	// sing-box начнёт умирать сразу после установки соединения (DNS context canceled).
	tStart := time.Now()
	if startErr, tunnelFailed, reason, errorCode := m.startEngine(ctx, engineCfg); startErr != nil {
		m.mu.Lock()
		m.clearPendingLocked()
		m.mu.Unlock()
		m.emitStatus()
		m.log.Warning(fmt.Sprintf("[PROXY] sing-box не запустился: %v", startErr))

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
				m.routingMode = routingMode
				m.whitelist = append([]string(nil), whitelist...)
				m.appWhitelist = append([]string(nil), appWhitelist...)
				m.appForceVPN = append([]string(nil), appForceVPN...)
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

		m.log.Error(fmt.Sprintf("[PROXY] Ошибка запуска движка: %v", startErr))
		return ConnectResultDTO{
			Success:      false,
			Message:      fmt.Sprintf("Ошибка запуска: %v", startErr),
			TunnelFailed: tunnelFailed,
			Reason:       reason,
			ErrorCode:    errorCode,
		}
	}

	startDur = time.Since(tStart)
	m.emitStatus()

	// The DNS override and the post-start probe are independent: the probe
	// dials the local inbound (127.0.0.1) and never touches the OS resolver.
	// Running them one after the other added the full DNS cost (~1.5s of a
	// ~3.5s connect) to every connect, so the override goes to its own
	// goroutine and we join it on both the success and the abort path below.
	// Neither applySystemDNSOverride nor applyTunnelAdapterDNS touches manager
	// state beyond m.sysDNS and the logger, so running them off the lock is safe.
	tDNS := time.Now()
	dnsDone := make(chan struct{})
	go func() {
		defer close(dnsDone)
		m.applySystemDNSOverride(isEndpointProtocol, dnsServers)
		m.applyTunnelAdapterDNS(mode, tunIPv4)
	}()

	tProbe := time.Now()
	proxyExtra := parseExtra(proxy)
	if code, reason := runPostStartProbe(connectCtx, proxyTypeLower, proxy.IP, proxy.Port, actualLocalPort, mode, proxyExtra); code != "" {
		// The override is already in flight — let it finish, then undo it.
		// Without this an aborted connect leaves the machine pointing at our
		// resolvers with no session behind them.
		<-dnsDone
		m.revertDNSOverride()
		_ = m.takeDNSTimings()
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

	probeDur = time.Since(tProbe)

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
	// Started right after the engine came up, in parallel with the probe above.
	<-dnsDone
	dnsDur = time.Since(tDNS)
	dnsTimings := m.takeDNSTimings()

	m.captureLiveServerIP(&proxy)
	m.clearPendingLocked()
	m.connected = true
	m.mode = mode
	m.proxy = &proxy
	m.killSwitch = killSwitch
	m.routingMode = routingMode
	m.whitelist = append([]string(nil), whitelist...)
	m.appWhitelist = append([]string(nil), appWhitelist...)
	m.appForceVPN = append([]string(nil), appForceVPN...)
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
	// Harden the in-process sing-box engine against CPU starvation for the
	// lifetime of the session: under full-core load (dev `go test`/builds) a
	// Normal-priority engine stops draining its sockets and the OS aborts every
	// tunneled connection at once. Restored in disconnectLocked. Best-effort —
	// a failed bump only forfeits hardening, never blocks the connect.
	if err := sys.RaiseProcessPriority(); err != nil {
		m.log.Warning(fmt.Sprintf("[СИСТЕМА] Не удалось повысить приоритет процесса: %v", err))
	}
	m.emitStatusLocked()
	m.mu.Unlock()

	if proxy.SubscriptionURL != "" {
		m.log.Success(fmt.Sprintf("[PROXY] Подключено (%s)", proxy.Type))
	} else {
		m.log.Success(fmt.Sprintf("[PROXY] Подключено к %s:%d (%s)", proxy.IP, proxy.Port, proxy.Type))
	}
	m.log.Info(fmt.Sprintf("[PROXY] Тайминг подключения: resolve=%dms start=%dms probe=%dms dns=%dms total=%dms",
		resolveDur.Milliseconds(), startDur.Milliseconds(), probeDur.Milliseconds(),
		dnsDur.Milliseconds(), time.Since(connectStart).Milliseconds()))
	if dnsTimings != "" {
		m.log.Info("[PROXY] Тайминг DNS: " + dnsTimings)
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
func (m *Manager) applySystemDNSOverride(skip bool, dnsServers []string) {
	if skip || m.sysDNS == nil {
		return
	}
	if !isAdminCheck() {
		m.log.Warning("[СИСТЕМА] Смена системного DNS (защита от утечек) требует прав администратора — запустите приложение от имени администратора")
		return
	}
	if err := m.sysDNS.Override(dnsOverrideServers(dnsServers)); err != nil {
		if errors.Is(err, ErrDNSRequiresAdmin) {
			m.log.Warning("[СИСТЕМА] Смена системного DNS требует прав администратора")
			return
		}
		m.log.Warning(fmt.Sprintf("[СИСТЕМА] DNS leak protection не применён: %v", err))
		return
	}
	m.log.Success("[СИСТЕМА] Системный DNS перенаправлен на защищённые резолверы")
}

// tunAdapterDNSAddrs derives (adapterIP, dnsIP) from the TUN interface CIDR.
// dnsIP is the next host after the interface address — an address inside the
// TUN subnet, so DNS queries to it are routed into the TUN and answered by
// sing-box's hijack-dns rule through the encrypted tunnel. An empty cidr falls
// back to the BuildTunnelModeConfig default. Returns ("", "") on a CIDR that
// can't yield a second host.
func tunAdapterDNSAddrs(tunIPv4CIDR string) (adapterIP, dnsIP string) {
	cidr := strings.TrimSpace(tunIPv4CIDR)
	if cidr == "" {
		cidr = "172.19.0.1/30"
	}
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", ""
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return "", ""
	}
	next := make(net.IP, len(ip4))
	copy(next, ip4)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}
	if !ipnet.Contains(next) {
		return "", ""
	}
	return ip4.String(), next.String()
}

// applyTunnelAdapterDNS points the TUN adapter's resolver at an address inside
// the TUN subnet so the Windows resolver keeps working through the tunnel.
// applySystemDNSOverride pins the *physical* adapters to neutral public
// resolvers — right against leaks, but plain UDP/53 to them is unreachable
// outside the tunnel (ISP blackholing; strict_route WFP drops off-TUN
// packets), so after the override the OS resolver survived only on cache and
// app-level DoH. Routing queries into the TUN (hijack-dns → sing-box DNS,
// detour=proxy) makes the system resolver actually work mid-session. The
// adapter vanishes on disconnect, so there is nothing to restore.
func (m *Manager) applyTunnelAdapterDNS(mode ProxyMode, tunIPv4 string) {
	if mode != ProxyModeTunnel || m.sysDNS == nil {
		return
	}
	if !isAdminCheck() {
		// Tunnel mode is admin-gated upstream; without admin the adapter DNS
		// call would only fail noisily.
		return
	}
	adapterIP, dnsIP := tunAdapterDNSAddrs(tunIPv4)
	if dnsIP == "" {
		return
	}
	if err := m.sysDNS.OverrideTunnelAdapter(adapterIP, dnsIP); err != nil {
		m.log.Warning(fmt.Sprintf("[СИСТЕМА] Не удалось направить системный DNS в туннель: %v", err))
		return
	}
	m.log.Success("[СИСТЕМА] Системный DNS направлен в туннель (резолв через VPN)")
}

// revertDNSOverride undoes applySystemDNSOverride/applyTunnelAdapterDNS after a
// connect aborts past the point where the override was already applied. Since
// the override now runs in parallel with the post-start probe, a failed probe
// can leave the machine pointing at our resolvers with no session behind them.
// The tunnel adapter needs no separate reset — it disappears with the engine —
// so restoring the snapshot covers everything that outlives the failed attempt.
func (m *Manager) revertDNSOverride() {
	if m.sysDNS == nil {
		return
	}
	if err := m.sysDNS.Restore(); err != nil {
		m.log.Warning(fmt.Sprintf("[СИСТЕМА] Не удалось вернуть системный DNS после неудачного подключения: %v", err))
	}
}

// takeDNSTimings returns the per-step breakdown of the last system DNS override
// for the connect log, or "" when the platform impl doesn't record one.
func (m *Manager) takeDNSTimings() string {
	src, ok := m.sysDNS.(dnsTimingSource)
	if !ok {
		return ""
	}
	return src.TakeDNSTimings()
}

// resolvePinnedServerIP resolves a domain server address to a single IP once,
// at connect time, while the host OS resolver still works (before we redirect
// system DNS for leak protection). The returned IP is pinned into the sing-box
// outbound and the kill switch so neither depends on a live resolver mid-session
// — see ProxyConfig.ResolvedIP. Returns "" when host is empty, already an IP
// literal, or resolution fails (callers then keep the original domain behaviour,
// so this can only improve robustness, never regress it). IPv4 is preferred
// because the tunnel DNS strategy is ipv4_only.
//
// The timeout is deliberately short. This is a best-effort optimization: a
// healthy OS resolver answers in ~100-150ms, so 500ms is ample for any domain
// that *can* be resolved locally. Censored/DPI-blocked VPN-server domains can't
// be resolved by the OS at all (that's what the tunnel is for) — sing-box
// resolves them via its own bootstrap DNS — so blocking the whole connect on a
// 3s timeout there was pure waste. Capping at 500ms means we still grab the pin
// when it's cheaply available and otherwise get out of the way fast.
const pinnedResolveTimeout = 500 * time.Millisecond

func resolvePinnedServerIP(host string) string {
	host = strings.TrimSpace(host)
	if host == "" || net.ParseIP(host) != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), pinnedResolveTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return ""
	}
	for _, a := range addrs {
		if v4 := a.IP.To4(); v4 != nil {
			return v4.String()
		}
	}
	return addrs[0].IP.String()
}

// resolveAllServerIPs resolves a domain server to ALL of its IPv4 addresses at
// connect time, while the OS resolver still works. The set seeds the static
// `hosts` DNS record (see buildDNS + ProxyConfig.ResolvedIPs) so sing-box can
// fail over across a CDN's backends within a live session instead of dying when
// the single pinned IP's transport resets. IPv4 only — the tunnel DNS strategy
// is ipv4_only and the outbound dials v4. Returns nil for empty hosts, literal
// IPs, or a censored/failed resolver (callers then fall back to the single pin
// or the `local` rule, never worse than before).
func resolveAllServerIPs(host string) []string {
	host = strings.TrimSpace(host)
	if host == "" || net.ParseIP(host) != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), pinnedResolveTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(addrs))
	var out []string
	for _, a := range addrs {
		v4 := a.IP.To4()
		if v4 == nil {
			continue
		}
		s := v4.String()
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// captureLiveServerIP fills proxy.ResolvedIP from the live OS socket when the
// connect-time pin (resolvePinnedServerIP) came back empty for a domain server.
// That is the censored-DNS case: the OS resolver cannot resolve the server
// hostname at all, but sing-box resolved it via its own DNS and connected — so
// the OS holds a live socket from our process to the server's real IP. Reading
// it gives the watchdog and the kill-switch firewall the actual address in use
// (instead of "no proxy IP to allow"), which is also more correct than a fresh
// lookup: a CDN-fronted server has many A records and a re-resolve could return
// a pool member the live connection never touches.
//
// Best-effort and TCP-only (establishedServerIP returns "" for UDP transports
// and on non-Windows builds); an empty result leaves the domain behaviour
// unchanged, never a regression. Subscription servers keep their backend IP out
// of the log (see newSingBoxLogWriter).
func (m *Manager) captureLiveServerIP(proxy *ProxyConfig) {
	if proxy.ResolvedIP != "" || proxy.IP == "" || net.ParseIP(proxy.IP) != nil {
		return
	}
	ip := establishedServerIP(proxy.Port)
	if ip == "" {
		return
	}
	proxy.ResolvedIP = ip
	// Persist so the NEXT connect pins this IP up front (see Manager.Connect),
	// eliminating the per-connection re-resolve that trips the false kill switch.
	// Encrypted via m.secrets; no-op when no codec is wired.
	saveServerPin(resultProxyDataDir(), m.secrets, proxy.IP, ip)
	if proxy.SubscriptionURL == "" {
		m.log.Info(fmt.Sprintf("[PROXY] IP сервера получен из активного соединения: %s", ip))
	} else {
		m.log.Info("[PROXY] IP сервера получен из активного соединения")
	}
}

// shouldRetryWithoutPin reports whether a failed connect that used a cached
// server-IP pin should be retried on the bare domain. A stale pin (the server's
// IP changed) lets the engine start but fails the post-start probe — that exact
// code is the signal. Other failures (cancelled, config errors, engine start)
// are not pin-related and must surface as-is.
func shouldRetryWithoutPin(usedCachedPin bool, errorCode string) bool {
	return usedCachedPin && errorCode == "post_start_probe_failed"
}

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
	routingMode RoutingMode, whitelist, appWhitelist, appForceVPN []string,
	killSwitch bool,
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
		AppForceVPN:       append([]string(nil), appForceVPN...),
		KillSwitch:        killSwitch,
		LocalPort:         actualLocalPort,
		DNSServers:        dnsServers,
		TunIPv4:           tunIPv4,
		TunIPv6:           tunIPv6,
		TunStack:          m.tunStack,
		DNSLeakProtection: dnsLeakProtection,
		DataDir:           resultProxyDataDir(),
	}
	engineCfg.RoutingLists = m.routingListSpecsLocked()
	// Smart mode needs the censored block-list in the engine config so
	// buildRoute can tunnel those domains/ranges while everything else goes
	// direct. Only populated for Smart — Global/Whitelist ignore it.
	if routingMode == ModeSmart && m.router != nil {
		engineCfg.BlockedDomains = m.router.GetBlockedDomains()
		engineCfg.BlockedCIDRs = m.router.GetBlockedCIDRs()
		// Pre-compile the block-list into a binary rule-set so the engine does
		// not have to parse and index ~78k domain_suffix entries out of the
		// config on every connect. Not fatal: buildRoute falls back to inline.
		if path, err := CompileSmartRuleSet(engineCfg.DataDir, engineCfg.BlockedDomains); err != nil {
			m.log.Warning(fmt.Sprintf("[SMART] Rule-set не скомпилирован, используется инлайн-список: %v", err))
		} else {
			engineCfg.SmartRuleSetPath = path
		}
	}
	if code, err := validateEngineConfig(engineCfg); err != nil {
		return ConnectResultDTO{
			Success:   false,
			Message:   err.Error(),
			Reason:    err.Error(),
			ErrorCode: code,
		}
	}

	m.setPendingLocked(proxy, mode)

	// Pin the resolved server IP (see Connect for rationale) so sing-box and the
	// kill switch never depend on a live resolver mid-session. Skip when already
	// pinned (carried over from the prior connect via m.proxy) — a censored
	// server's OS resolve only burns its timeout to fail.
	if proxy.ResolvedIP == "" {
		if resolved := resolvePinnedServerIP(proxy.IP); resolved != "" {
			proxy.ResolvedIP = resolved
			engineCfg.Proxy.ResolvedIP = resolved
		}
	}
	// Full backend set for the hosts-pin (see ProxyConfig.ResolvedIPs) — lets a
	// CDN/multi-IP server fail over across backends mid-session instead of dying
	// with one. Empty for literals / censored resolver → falls back to the pin.
	if len(proxy.ResolvedIPs) == 0 {
		if all := resolveAllServerIPs(proxy.IP); len(all) > 0 {
			proxy.ResolvedIPs = all
			engineCfg.Proxy.ResolvedIPs = all
		}
	}

	if startErr, tunnelFailed, reason, errorCode := m.startEngine(ctx, engineCfg); startErr != nil {
		m.clearPendingLocked()
		m.emitStatusLocked()
		m.log.Error(fmt.Sprintf("[PROXY] Ошибка запуска движка: %v", startErr))
		return ConnectResultDTO{
			Success:      false,
			Message:      fmt.Sprintf("Ошибка запуска: %v", startErr),
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
	m.applySystemDNSOverride(isEndpointProtocol, dnsServers)
	m.applyTunnelAdapterDNS(mode, tunIPv4)

	m.captureLiveServerIP(&proxy)
	m.clearPendingLocked()
	m.connected = true
	m.mode = mode
	m.proxy = &proxy
	m.killSwitch = killSwitch
	m.routingMode = routingMode
	m.whitelist = append([]string(nil), whitelist...)
	m.appWhitelist = append([]string(nil), appWhitelist...)
	m.appForceVPN = append([]string(nil), appForceVPN...)
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

// connectProbeInterval is how often a post-start probe re-checks while a freshly
// started link warms up. Short enough that a link coming up early is confirmed
// within a fraction of a second.
const connectProbeInterval = 250 * time.Millisecond

// connectProbeDeadline* is the overall budget for one post-start probe phase.
// Two values, both >= the old per-case sleep+backoff totals so a genuinely dead
// link fails no sooner than before (no new false-failure risk) — we only make
// the success path faster by not sleeping a fixed amount up front:
//
//   - Proxy: the local mixed listener is up the moment sing-box starts, so a
//     working upstream answers fast; a shorter budget keeps a genuine failure
//     from dragging the UI for many seconds.
//   - Tunnel: the TUN device + routes need a moment, and SS AEAD does a
//     key-exchange round-trip on the first request, so the budget matches the
//     old general-tunnel total (~8s) to avoid false failures during warm-up.
const (
	connectProbeDeadlineProxy  = 5 * time.Second
	connectProbeDeadlineTunnel = 8 * time.Second
)

// pollProbe calls attempt every interval until it succeeds, ctx is cancelled, or
// the deadline elapses. Returns (ok, cancelled, lastReason). Unlike the old
// sleep-then-retry-with-backoff, a link that's ready early is detected within one
// interval instead of waiting out a worst-case backoff, while the deadline keeps
// the worst case unchanged.
func pollProbe(ctx context.Context, deadline, interval time.Duration, attempt func() (bool, string)) (ok, cancelled bool, reason string) {
	end := time.Now().Add(deadline)
	for {
		if ctx.Err() != nil {
			return false, true, "connect cancelled"
		}
		ok, reason = attempt()
		if ok {
			return true, false, ""
		}
		if !time.Now().Before(end) {
			return false, false, reason
		}
		if !sleepOrCancel(ctx, interval) {
			return false, true, "connect cancelled"
		}
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
		ok, cancelled, r := pollProbe(ctx, connectProbeDeadlineProxy, connectProbeInterval, func() (bool, string) {
			return probeHTTPThroughProxyProbe(proxyAddr)
		})
		if cancelled {
			return "cancelled", "connect cancelled"
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
			ok, cancelled, r := pollProbe(ctx, connectProbeDeadlineProxy, connectProbeInterval, func() (bool, string) {
				return probeHTTPThroughProxyProbe(proxyAddr)
			})
			if cancelled {
				return "cancelled", "connect cancelled"
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
			proxyAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
			ok, cancelled, r := pollProbe(ctx, connectProbeDeadlineTunnel, connectProbeInterval, func() (bool, string) {
				return probeHTTPThroughProxyProbe(proxyAddr)
			})
			if cancelled {
				return "cancelled", "connect cancelled"
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
			wgProxyAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
			var httpOK bool
			var httpReason string
			for i := 0; i < attempts; i++ {
				if ctx.Err() != nil {
					return "cancelled", "connect cancelled"
				}
				httpOK, httpReason = probeHTTPThroughProxyProbe(wgProxyAddr)
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
			// Trojan требует TLS-рукопожатие при первом соединении — это занимает время.
			// Поллинг даёт sing-box успеть инициализироваться, не тратя фиксированную паузу.
			ok, cancelled, r := pollProbe(ctx, connectProbeDeadlineProxy, connectProbeInterval, func() (bool, string) {
				return probeHTTPThroughProxyProbe(proxyAddr)
			})
			if cancelled {
				return "cancelled", "connect cancelled"
			}
			if !ok {
				if r == "" {
					r = "trojan proxy e2e probe failed"
				}
				return "post_start_probe_failed", r
			}
		} else if mode == ProxyModeTunnel {
			proxyAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
			ok, cancelled, r := pollProbe(ctx, connectProbeDeadlineTunnel, connectProbeInterval, func() (bool, string) {
				return probeHTTPThroughProxyProbe(proxyAddr)
			})
			if cancelled {
				return "cancelled", "connect cancelled"
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
			ok, cancelled, r := pollProbe(ctx, connectProbeDeadlineProxy, connectProbeInterval, func() (bool, string) {
				return probeHTTPThroughProxyProbe(proxyAddr)
			})
			if cancelled {
				return "cancelled", "connect cancelled"
			}
			if !ok {
				if r == "" {
					r = "naiveproxy proxy e2e probe failed"
				}
				return "post_start_probe_failed", r
			}
		} else if mode == ProxyModeTunnel {
			proxyAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
			ok, cancelled, r := pollProbe(ctx, connectProbeDeadlineTunnel, connectProbeInterval, func() (bool, string) {
				return probeHTTPThroughProxyProbe(proxyAddr)
			})
			if cancelled {
				return "cancelled", "connect cancelled"
			}
			if !ok {
				if r == "" {
					r = "naiveproxy e2e probe failed"
				}
				return "post_start_probe_failed", r
			}
		}
	}

	// General tunnel probe: verify the engine→upstream→exit path carries HTTP
	// before claiming success. Applies to all protocols that don't return early
	// above (SS, VLESS, VMESS, xhttp, etc.) when in tunnel mode.  WG/AWG return
	// "", "" above and never reach this point.  Trojan handles both modes in its
	// own case.
	//
	// The probe goes through the loopback probe inbound (NOT the TUN default
	// route): the hostname is resolved remotely by sing-box, so the probe does
	// not depend on the OS resolver — which a reconnect mid-session would hit
	// with the system DNS already overridden.
	//
	// SS with AEAD ciphers needs a TCP+key-exchange round-trip on the very first
	// request, which is noticeably slower than subsequent ones. Polling to the
	// deadline tolerates that warm-up, while a tunnel that's already routable is
	// confirmed within one interval instead of after a fixed 2s sleep.
	if mode == ProxyModeTunnel {
		proxyAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
		httpOK, cancelled, httpReason := pollProbe(ctx, connectProbeDeadlineTunnel, connectProbeInterval, func() (bool, string) {
			return probeHTTPThroughProxyProbe(proxyAddr)
		})
		if cancelled {
			return "cancelled", "connect cancelled"
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
	// Short per-attempt timeouts: this runs only on the connect warm-up path,
	// driven by pollProbe which re-tries every 250ms up to an 8s deadline. A
	// working link answers generate_204 in well under a second; a stalled
	// attempt should yield to the next poll quickly rather than block for 10s.
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			DialContext: (&net.Dialer{
				Timeout: 2 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 2 * time.Second,
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

// probeProxyHealth checks the proxy-mode data path by sending short HTTP requests
// to connectivity-check endpoints THROUGH the local listener (127.0.0.1:localPort),
// instead of opening a bare TCP connection to the proxy server port.
//
// The old watchdog dialed the server port directly every few seconds, which had
// two failure modes that tripped the kill switch on a perfectly working link:
//   - a raw connect-then-close looks like port scanning, so servers behind
//     nginx/fail2ban/SYN-cookies start dropping the probe SYNs after a while;
//   - when the server is addressed by domain, every probe re-resolved it through
//     whatever local resolver was active (the ISP one, since the DNS-leak
//     override needs admin) — flaky resolvers produced false "dead" verdicts.
//
// Routing the probe through the local listener fixes both: it is indistinguishable
// from ordinary proxied traffic (no rate-limit trigger), the target hostname is
// resolved remotely by sing-box (not the local resolver), and a success actually
// proves sing-box → upstream → exit carries traffic. Timeouts are kept short so a
// genuine outage is still caught within a few ticks.
func probeProxyHealth(localPort int) (bool, string) {
	proxyURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", localPort))
	if err != nil {
		return false, "bad proxy url"
	}
	client := &http.Client{
		Timeout: 4 * time.Second,
		Transport: &http.Transport{
			Proxy:               http.ProxyURL(proxyURL),
			DialContext:         (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
			TLSHandshakeTimeout: 3 * time.Second,
			DisableKeepAlives:   true,
		},
	}
	lastReason := ""
	for _, target := range tunnelProbeURLs() {
		resp, err := client.Get(target)
		if err != nil {
			lastReason = probeFailureReason(err)
			continue
		}
		_ = resp.Body.Close()
		// Any HTTP response (even 5xx) means the tunnel carried the request.
		// Only 407 means the local proxy itself rejected it.
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

// probeTunnelHealth checks the tunnel-mode data path by sending a short HTTP
// request through the system default route, which in tunnel mode goes through
// the TUN interface. The probe domains are forced through the proxy outbound by
// sing-box routing rules (see buildRoute / tunnelProbeDomains), so the request
// actually traverses the tunnel rather than the self-direct shortcut used for
// the app's own traffic. A successful response proves the data path works end-
// to-end. Using this instead of a raw QUIC handshake eliminates false kill-
// switch trips: the handshake opens a *new* connection that can be rate-limited
// or dropped transiently even when the existing session is healthy.
func probeTunnelHealth() (bool, string) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
			TLSHandshakeTimeout: 3 * time.Second,
			DisableKeepAlives:   true,
		},
	}
	lastReason := ""
	for _, target := range tunnelProbeURLs() {
		resp, err := client.Get(target)
		if err != nil {
			lastReason = probeFailureReason(err)
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

// probeFailureReason classifies a health-probe error for the watchdog log.
// A failed lookup in the LOCAL resolver gets the "local_dns:" prefix — it
// proves nothing about the VPN server (the session itself degrades the OS
// resolver: the system DNS override pins physical adapters to resolvers that
// are unreachable outside the tunnel), so the watchdog must not count it as a
// server-dead strike. See isLocalDNSProbeFailure.
func probeFailureReason(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "local_dns: " + dnsErr.Err
	}
	return pingReasonFromError(err)
}

func isLocalDNSProbeFailure(reason string) bool {
	return strings.HasPrefix(reason, "local_dns:")
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
	disconnectStart := time.Now()
	// Abort any in-progress Connect so its goroutines stop. This MUST run before
	// taking opMu: an in-flight Connect holds opMu across its slow phase, and
	// cancelling its (cancellable) probe is what lets it release opMu promptly
	// instead of making Disconnect wait out the full probe budget.
	m.CancelConnect()

	// Serialize against connect/reconnect (see opMu).
	m.opMu.Lock()
	defer m.opMu.Unlock()

	// Stop engine unconditionally before acquiring the lock.
	// During Phase 2 of Connect(), the engine may already be running while
	// m.connected is still false.  disconnectLocked() only stops the engine
	// when m.connected==true, so without this explicit call a mid-connect
	// Disconnect() would leave the engine alive, causing the next Connect()
	// to fail with "engine already running".
	//
	// Run the three slow teardown steps concurrently instead of serially:
	// stopping the engine, disabling the system proxy, and restoring system DNS
	// are independent subsystems (sysProxy/sysDNS are set once at Init and never
	// mutated here, so they need no lock). Serially their latencies stacked;
	// concurrently the cost is max(), not sum().
	m.mu.Lock()
	wasConnected := m.connected
	m.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		_ = m.engine.Stop()
	}()
	go func() {
		defer wg.Done()
		if m.sysProxy != nil {
			if err := m.sysProxy.Disable(); err != nil {
				m.log.Warning(fmt.Sprintf("[СИСТЕМА] Ошибка отключения прокси: %v", err))
			} else if wasConnected {
				m.log.Info("[СИСТЕМА] Системный прокси отключен")
			}
		}
	}()
	go func() {
		defer wg.Done()
		if m.sysDNS != nil {
			if err := m.sysDNS.Restore(); err != nil {
				m.logDNSRestoreWarning(err)
			}
		}
	}()
	wg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopProcessTrackerLocked()
	m.stopHealthWatchdogLocked()

	if m.connected {
		m.log.Info("[PROXY] Отключение...")
	}
	m.connected = false
	m.proxy = nil
	m.clearPendingLocked()
	m.emitStatusLocked()
	m.log.Info(fmt.Sprintf("[PROXY] Тайминг отключения: total=%dms", time.Since(disconnectStart).Milliseconds()))
	return nil
}

func (m *Manager) disconnectLocked() error {
	if !m.connected {
		return nil
	}

	m.log.Info("[PROXY] Отключение...")

	m.stopProcessTrackerLocked()
	m.stopHealthWatchdogLocked()

	// Same rationale as Disconnect(): tear down engine, system proxy and system
	// DNS concurrently — independent subsystems, cost max() not sum(). This also
	// speeds up the reconnect path (SetMode / connectLocked call this first).
	// Safe under m.mu: none of these three take m.mu.
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		if err := m.engine.Stop(); err != nil {
			m.log.Error(fmt.Sprintf("[PROXY] Ошибка остановки движка: %v", err))
		}
	}()
	go func() {
		defer wg.Done()
		if m.sysProxy != nil {
			if err := m.sysProxy.Disable(); err != nil {
				m.log.Warning(fmt.Sprintf("[СИСТЕМА] Ошибка отключения прокси: %v", err))
			} else {
				m.log.Info("[СИСТЕМА] Системный прокси отключен")
			}
		}
	}()
	go func() {
		defer wg.Done()
		if m.sysDNS != nil {
			if err := m.sysDNS.Restore(); err != nil {
				m.logDNSRestoreWarning(err)
			}
		}
	}()
	wg.Wait()

	m.connected = false
	m.proxy = nil
	m.clearPendingLocked()

	// Drop the session-scoped priority bump raised on connect (see
	// sys.RaiseProcessPriority). A reconnect re-raises it on the next success.
	if err := sys.RestoreProcessPriority(); err != nil {
		m.log.Warning(fmt.Sprintf("[СИСТЕМА] Не удалось вернуть приоритет процесса: %v", err))
	}

	m.emitStatusLocked()

	return nil
}

func (m *Manager) SetMode(mode ProxyMode) error {
	// Serialize against Connect/Disconnect/ReconnectWithRoutingRules (see opMu).
	m.opMu.Lock()
	defer m.opMu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.mode == mode {
		return nil
	}

	wasConnected := m.connected
	proxy := m.proxy
	killSwitch := m.killSwitch
	routingMode := m.routingMode
	whitelist := append([]string(nil), m.whitelist...)
	appWhitelist := append([]string(nil), m.appWhitelist...)
	appForceVPN := append([]string(nil), m.appForceVPN...)

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
			appForceVPN,
			killSwitch,
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

// SetRoutingLists replaces the resolved routing-list specs used by the next
// engine start/reload. Stored under m.mu; a copy is taken so the caller may
// reuse its slice.
func (m *Manager) SetRoutingLists(specs []RoutingListSpec) {
	cp := make([]RoutingListSpec, len(specs))
	copy(cp, specs)
	m.mu.Lock()
	m.routingLists = cp
	m.mu.Unlock()
}

// routingListSpecsLocked returns a copy of the current specs. Callers must
// hold m.mu — both EngineConfig build sites in Connect/connectLocked already
// do.
func (m *Manager) routingListSpecsLocked() []RoutingListSpec {
	cp := make([]RoutingListSpec, len(m.routingLists))
	copy(cp, m.routingLists)
	return cp
}

func (m *Manager) ReconnectWithRoutingRules(ctx context.Context, routingMode RoutingMode, whitelist, appWhitelist, appForceVPN []string) ConnectResultDTO {
	// Serialize against Connect/Disconnect/SetMode (see opMu) — acquired before
	// mu to preserve the opMu→mu lock ordering.
	m.opMu.Lock()
	defer m.opMu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected || m.proxy == nil {
		return ConnectResultDTO{Success: true, Message: "not connected"}
	}

	p := *m.proxy
	mode := m.mode
	killSwitch := m.killSwitch
	lPort := m.localPort
	listenLAN := m.listenLAN
	dServers := m.dnsServers
	tIPv4 := m.tunIPv4
	tIPv6 := m.tunIPv6
	dnsLeak := m.dnsLeakProtection

	return m.connectLocked(ctx, p, mode, routingMode, whitelist, appWhitelist, appForceVPN, killSwitch, lPort, listenLAN, dServers, tIPv4, tIPv6, dnsLeak)
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
	m.mu.Unlock()

	var latency int64
	var reachable bool
	var reason string
	var checkType string

	ptUpper := strings.ToUpper(strings.TrimSpace(proxyType))

	isHysteria2 := ptUpper == "HYSTERIA2"
	isWireGuard := ptUpper == "WIREGUARD" || ptUpper == "AMNEZIAWG"

	if connected && mode == ProxyModeTunnel {
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
// Caller must hold m.mu. Any in-flight watchdog is cancelled first; if its
// probe is still blocking, the bumped generation will make it exit on the
// next lock acquisition without touching any shared state with the new one.
func (m *Manager) startHealthWatchdogLocked(proxy ProxyConfig, mode ProxyMode) {
	if m.healthCancel != nil {
		m.healthCancel()
		m.healthCancel = nil
	}
	m.healthGen++
	gen := m.healthGen
	m.proxyDead = false

	parentCtx := m.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	healthCtx, cancel := context.WithCancel(parentCtx)
	m.healthCancel = cancel

	go m.runHealthWatchdog(healthCtx, gen, proxy, mode)
}

// stopHealthWatchdogLocked stops the watchdog goroutine and clears the dead
// flag. Caller must hold m.mu.
func (m *Manager) stopHealthWatchdogLocked() {
	if m.healthCancel != nil {
		m.healthCancel()
		m.healthCancel = nil
	}
	m.healthGen++
	m.proxyDead = false
}

// watchdogTrafficAliveBytes is the per-interval proxy-outbound byte delta above
// which the watchdog treats the upstream as demonstrably alive and VETOES a
// kill-switch engage even if the HTTP probe failed. The probe opens a fresh
// connection each tick (new gRPC/XHTTP stream, or a captive-portal domain that
// can hiccup at the exit) and can transiently fail while the established session
// keeps carrying traffic — the false trips the user hit. ~16 KB/interval is far
// above failed-probe noise (a failed probe transfers ~0 proxy bytes) yet well
// below any real usage, so an idle-and-truly-dead tunnel (≈0 delta) still trips
// correctly. Counted on the proxy outbound only (GetProxyTrafficStats), so
// direct/split traffic can never mask a genuinely dead upstream.
const watchdogTrafficAliveBytes int64 = 16 * 1024

// maxConsecutiveVetoes bounds how long the traffic-veto (see
// watchdogTrafficAliveBytes) may suppress a kill-switch trip. A truly alive
// upstream recovers its HTTP probe within a tick or two, so a healthy session
// never approaches this cap; a wedged transport that keeps spraying retry bytes
// while every probe fails hits it and the veto yields, letting the outage be
// detected. In tunnel mode (5s interval) this is ~30s of grace; in proxy mode
// (10s) ~60s — long enough to ride out a transient exit hiccup, short enough
// that a dead session doesn't stay silently masked.
const maxConsecutiveVetoes = 6

func (m *Manager) runHealthWatchdog(ctx context.Context, gen uint64, proxy ProxyConfig, mode ProxyMode) {
	// Both modes now probe the data path (see probeHealthy): proxy mode through
	// the local listener, tunnel mode through the TUN default route. Direct
	// server probes (QUIC handshake, UDP/TCP connect) were opening *new*
	// connections that could be rate-limited or dropped transiently even when
	// the existing session was healthy — causing false kill-switch trips.
	//
	// Proxy mode gets a slower cadence and one extra strike because it has no
	// admin-enforced firewall, so false positives are pure noise. Tunnel mode
	// keeps tighter thresholds since the firewall actually blocks traffic there.
	interval := 10 * time.Second
	failuresBeforeDead := 3
	if mode == ProxyModeTunnel {
		interval = 5 * time.Second
		failuresBeforeDead = 2
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	consecutiveFails := 0
	// consecutiveVetoes counts how many ticks in a row the traffic-veto has
	// suppressed a would-be kill-switch trip. A wedged transport (e.g. a
	// half-open gRPC session, or the engine thrashing retries after a CPU-
	// starvation event) keeps emitting failed probes *and* churns enough retry
	// bytes on the proxy outbound to clear watchdogTrafficAliveBytes every tick,
	// so the veto would otherwise mask a genuinely dead session forever. After
	// maxConsecutiveVetoes the veto yields and the normal engage path runs.
	consecutiveVetoes := 0
	// Proxy-outbound byte counters at the previous tick, for the traffic veto.
	// Start at 0: the engage check needs failuresBeforeDead consecutive failures,
	// so the first tick's inflated delta (bytes since connect) never gates a trip.
	var lastProxyUp, lastProxyDown int64

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		// Snapshot under lock and bail early if disconnected OR if this
		// generation has been superseded by a newer watchdog.
		m.mu.Lock()
		if !m.connected || m.healthGen != gen {
			m.mu.Unlock()
			return
		}
		ks := m.killSwitch
		localPort := m.localPort
		engineRunning := m.engine != nil && m.engine.IsRunning()
		var proxyUp, proxyDown int64
		if engineRunning {
			proxyUp, proxyDown = m.engine.GetProxyTrafficStats()
		}
		m.mu.Unlock()

		// Per-interval proxy-outbound traffic delta — the liveness veto signal.
		proxyDelta := (proxyUp - lastProxyUp) + (proxyDown - lastProxyDown)
		lastProxyUp, lastProxyDown = proxyUp, proxyDown

		// Probe runs without the lock — the HTTP/TCP dial can block for seconds.
		alive, failReason := m.probeHealthy(proxy, mode, localPort, engineRunning)

		// Opportunistic live-IP capture (see captureLiveServerIP): the connect-
		// time capture can miss for XHTTP, whose server connections churn, so
		// retry after a healthy probe — traffic just reached the server, so an
		// ESTABLISHED socket almost certainly exists now. No-op once pinned or for
		// non-domain servers; the gen-guarded sync below feeds the real IP into
		// m.proxy so a later firewall engage isn't left with "no proxy IP".
		if alive {
			m.captureLiveServerIP(&proxy)
		}

		m.mu.Lock()
		// Re-check after the probe: Disconnect or a reconnect may have run
		// while we waited. The gen check is what blocks an old watchdog
		// from acting on stale probe results after a server switch.
		if !m.connected || m.healthGen != gen {
			m.mu.Unlock()
			return
		}
		if proxy.ResolvedIP != "" && m.proxy != nil && m.proxy.ResolvedIP == "" {
			m.proxy.ResolvedIP = proxy.ResolvedIP
		}
		if len(proxy.ResolvedIPs) > 0 && m.proxy != nil && len(m.proxy.ResolvedIPs) == 0 {
			m.proxy.ResolvedIPs = proxy.ResolvedIPs
		}
		wasDead := m.proxyDead
		if alive {
			consecutiveFails = 0
			consecutiveVetoes = 0
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
		if isLocalDNSProbeFailure(failReason) {
			// The LOCAL resolver failed the lookup — that proves nothing about
			// the VPN server (the session itself degrades the OS resolver: the
			// system DNS override pins physical adapters to resolvers that are
			// unreachable outside the tunnel). Neither a strike nor a recovery:
			// keep the current count and try again next tick. Only the legacy
			// fallback probes can produce this; the loopback-listener probe
			// resolves hostnames remotely via sing-box.
			if !wasDead {
				m.log.Warning(fmt.Sprintf("[KILL SWITCH] Проба не выполнена: локальный DNS не ответил (%s) — не считается отказом сервера", failReason))
			}
			m.mu.Unlock()
			continue
		}
		consecutiveFails++
		if !wasDead {
			if failReason == "" {
				failReason = "нет ответа"
			}
			m.log.Warning(fmt.Sprintf("[KILL SWITCH] Проба не прошла (%d/%d): %s", consecutiveFails, failuresBeforeDead, failReason))
		}
		var shouldEngage bool
		var engageFn func(ProxyConfig, []string)
		var engageProxy ProxyConfig
		var engageDNS []string
		if consecutiveFails >= failuresBeforeDead && !wasDead && proxyDelta >= watchdogTrafficAliveBytes && consecutiveVetoes < maxConsecutiveVetoes {
			// Probe failed, but the proxy outbound moved real traffic this
			// interval → the upstream is alive (a transient new-connection/route
			// hiccup, not a dead server). Hold off the kill switch; keep counting
			// so a genuine outage (traffic actually stops) still trips next tick.
			// Bounded by maxConsecutiveVetoes so a wedged transport that keeps
			// churning retry bytes while every probe fails can't mask a dead
			// session indefinitely.
			consecutiveVetoes++
			m.log.Warning(fmt.Sprintf("[KILL SWITCH] Проба не прошла, но прокси несёт трафик (Δ=%d КБ за интервал) — блокировка отложена (%d/%d)", proxyDelta/1024, consecutiveVetoes, maxConsecutiveVetoes))
			m.mu.Unlock()
			continue
		}
		if consecutiveFails >= failuresBeforeDead && !wasDead {
			m.proxyDead = true
			// Subscription servers must never expose the provider's backend
			// address in logs (see newSingBoxLogWriter); a manual server keeps
			// "host:port" since the user owns it and already sees it in the UI.
			srv := "VPN-сервер"
			if proxy.SubscriptionURL == "" {
				srv = fmt.Sprintf("VPN-сервер %s:%d", proxy.IP, proxy.Port)
			}
			switch {
			case ks && m.isAdmin && m.KillSwitchFirewallEngage != nil:
				m.log.Warning(fmt.Sprintf("[KILL SWITCH] %s недоступен — kill switch блокирует весь трафик", srv))
				shouldEngage = true
				engageFn = m.KillSwitchFirewallEngage
				engageProxy = proxy
				engageDNS = append([]string(nil), m.dnsServers...)
			case ks && !m.isAdmin:
				// Kill switch armed but unenforceable — the OS firewall needs admin.
				// Report the outage without raising KillSwitchEmergency (see
				// buildStatusLocked): a blocking alarm that blocks nothing is
				// exactly the "fires for no reason" the user hit in proxy mode.
				m.log.Warning(fmt.Sprintf("[KILL SWITCH] %s не отвечает. Блокировка трафика недоступна без прав администратора — перезапустите приложение от имени администратора.", srv))
			default:
				m.log.Warning(fmt.Sprintf("[PROXY] %s недоступен", srv))
			}
			m.emitStatusLocked()
		}
		m.mu.Unlock()
		if shouldEngage && engageFn != nil {
			engageFn(engageProxy, engageDNS)
		}
	}
}

// probeHealthy decides whether the active session is still carrying traffic.
// Returns the verdict plus a failure reason for the watchdog log.
//
// Both proxy and tunnel modes check the *data path* when the sing-box engine is
// running, not the raw server reachability. A bare connect/handshake to the
// server opens a *new* connection that can be rate-limited or dropped transiently
// even when the existing session is healthy — exactly the false kill-switch trips
// the user observed.
//
// Both modes probe through the local loopback listener (127.0.0.1:localPort;
// tunnel mode gets a dedicated probe inbound, see BuildTunnelModeConfig): the
// target hostname is resolved remotely by sing-box, NOT by the OS resolver.
// Tunnel probes previously went through the TUN default route and died at
// getaddrinfo once the session's own DNS override + strict_route degraded the
// OS resolver — tripping the kill switch on a healthy server.
//
// Falls back to direct server reachability only when the engine is not running
// (sing-box failed to start, system proxy points straight at the server).
func (m *Manager) probeHealthy(proxy ProxyConfig, mode ProxyMode, localPort int, engineRunning bool) (bool, string) {
	if engineRunning && localPort > 0 {
		return probeProxyHealthProbe(localPort)
	}
	if mode == ProxyModeTunnel && engineRunning {
		// No known local port (shouldn't happen on the connect paths) — legacy
		// default-route probe is still better than nothing.
		return probeTunnelHealthProbe()
	}
	return m.probeProxyAlive(proxy, mode)
}

// probeProxyAlive picks the right probe for the proxy's transport. HYSTERIA2
// and WireGuard speak UDP, the rest TCP — a plain TCP connect would falsely
// pass/fail for UDP endpoints.
func (m *Manager) probeProxyAlive(proxy ProxyConfig, mode ProxyMode) (bool, string) {
	pt := strings.ToUpper(strings.TrimSpace(proxy.Type))
	if mode == ProxyModeTunnel {
		switch pt {
		case "HYSTERIA2":
			_, reachable, reason, _ := pingHysteria2LANProbe(proxy.IP, proxy.Port)
			return reachable, reason
		case "WIREGUARD", "AMNEZIAWG":
			_, reachable, reason := pingWireGuardLANProbe(proxy.IP, proxy.Port)
			return reachable, reason
		default:
			_, reachable, reason := pingLANProbe(proxy.IP, proxy.Port)
			return reachable, reason
		}
	}

	switch pt {
	case "HYSTERIA2":
		_, reachable, reason, _ := pingHysteria2Probe(proxy.IP, proxy.Port)
		return reachable, reason
	case "WIREGUARD", "AMNEZIAWG":
		_, reachable, reason := pingWireGuardProbe(proxy.IP, proxy.Port)
		return reachable, reason
	default:
		_, reachable, reason := pingTCPProbe(proxy.IP, proxy.Port)
		return reachable, reason
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

	// Revert OS-level network state FIRST — proxy and DNS cleanup are fast
	// and independent of the engine. If engine.Stop() hangs (sing-box
	// instance.Close() is known to block) and the parent's 10s force-exit
	// fires, proxy/DNS are already restored and the user is not stranded
	// behind a dead local listener or wrong resolver.
	if m.sysProxy != nil {
		m.sysProxy.DisableSync()
	}

	// Always restore system DNS on quit — same rationale as disabling the
	// system proxy above: the in-memory connected flag can be stale, and a
	// clean exit that leaves the OS resolver pointed at our override makes the
	// internet "hang" until the next launch. Restore() is a no-op when no
	// snapshot exists, so this is safe to call unconditionally.
	if m.sysDNS != nil {
		if err := m.sysDNS.Restore(); err != nil {
			m.logDNSRestoreWarning(err)
		}
	}

	// In tunnel mode the sing-tun adapter owns the default route. engine.Stop
	// asks sing-box to tear it down, but instance.Close() is known to hang on
	// WireGuard/AmneziaWG sessions (UDP socket teardown, see closeInstanceBounded
	// comments). If Close() times out, the 5s ceiling lets us continue, but the
	// adapter's auto_route entries are still in the routing table — the
	// internet is dead. Clear the TUN routes explicitly BEFORE relying on
	// engine.Stop(): this is fast (Remove-NetRoute), removes every route bound
	// to the adapter so the physical default route takes over, and is harmless
	// if the engine then cleans up properly (double-delete of a route is a no-op).
	//
	// Run UNCONDITIONALLY rather than gating on (m.connected && tunnel mode):
	// clearLeftoverTun is a no-op when no sing-tun adapter is present (proxy mode,
	// or already torn down), so it is safe in both modes. Critically, it is the
	// only thing that frees the default route when the user quits MID-CONNECT —
	// at that point the tunnel already owns 0.0.0.0/0 but m.connected is still
	// false, so the old gate skipped the cleanup and stranded the internet.
	if err := clearLeftoverTunFn(); err != nil {
		m.log.Warning(fmt.Sprintf("[СИСТЕМА] Не удалось снять маршруты TUN при завершении: %v", err))
	}

	// Stop the engine whenever it is live OR still coming up. Gating on
	// m.connected alone left a running sing-box (and its tun adapter) alive when
	// the user quit during the establishing phase, leaking the process and the
	// adapter that owns the default route.
	if m.engine != nil && (m.connected || m.engine.IsRunning()) {
		m.engine.Stop()
	}
}


func (m *Manager) GetRouter() *Router {
	return m.router
}

// RecoverSystemLeftovers detects AND removes OS-level network state stranded by
// a prior run that exited without cleanup (crash / force-kill). It is the
// reliable, UI-independent recovery path run once at startup. Returns which
// categories were found and cleaned so the caller can notify the user.
//
//   - proxy: stale system-proxy registry pointing at our dead local port
//   - dns:   an un-restored system DNS override (restored from the snapshot)
//   - tun:   a leftover sing-tun adapter still holding the auto_route default
//     route — the dominant tunnel-mode "no internet" cause; its routes are
//     deleted and its DNS reset so traffic falls back to the physical link.
//
// sysProxy/sysDNS are assigned once in Init and never reassigned, so no lock is
// needed for them.
//
// CRITICAL — never revert state while one of OUR OWN sessions is active or
// being established. Recovery runs asynchronously at startup, and its detection
// steps are slow (a PowerShell adapter enumeration is ~1-2s); a fast connect can
// land mid-recovery. At that point the "leftovers" are not leftovers at all —
// they are the system proxy WE just set, the DNS WE overrode, and the sing-tun
// adapter WE created. Reverting them tears the live session down: deleting the
// tunnel's default route killed an active HYSTERIA2 session in the field and
// tripped the kill switch. sessionActive() is therefore re-checked immediately
// before EACH destructive step, so a connect that began during the (slow)
// detection above it still aborts the revert. A genuine prior-crash leftover is
// only ever present when no session of ours is active.
// LeftoverScan is the result of one startup recovery pass.
//
//   - Proxy: a stranded system proxy was found and removed (registry-only, no
//     admin needed — always cleaned immediately).
//   - DNS / Tun: a DNS-override snapshot / orphan sing-tun adapter was found.
//     When the process is elevated they were also removed; otherwise they were
//     only DETECTED and NeedsElevation is set.
//   - NeedsElevation: admin-requiring leftovers exist but the process is not
//     elevated. The app layer surfaces a "restart as admin to clean up" prompt
//     (user-initiated UAC via RestartAsAdmin) instead of the old behaviour of
//     firing a surprise UAC dialog mid-startup — which raced the frontend
//     auto-connect and was routinely dismissed, leaving the orphan adapter's
//     stale default route in place (the false-kill-switch trigger).
type LeftoverScan struct {
	Proxy          bool
	DNS            bool
	Tun            bool
	NeedsElevation bool
}

func (s LeftoverScan) Any() bool { return s.Proxy || s.DNS || s.Tun }

func (m *Manager) RecoverSystemLeftovers() (scan LeftoverScan) {
	if m.sessionActive() {
		return
	}
	if m.sysProxy != nil && m.sysProxy.LeftoverActive() && !m.sessionActive() {
		scan.Proxy = true
		if err := m.sysProxy.Disable(); err != nil {
			m.log.Warning(fmt.Sprintf("[СИСТЕМА] Ошибка снятия остаточного системного прокси: %v", err))
		}
	}

	hasDNSLeft := m.sysDNS != nil && m.sysDNS.SnapshotExists() && !m.sessionActive()
	hasTunLeft := !m.sessionActive() && hasLeftoverTunFn()

	if !hasDNSLeft && !hasTunLeft {
		return
	}
	scan.DNS = hasDNSLeft
	scan.Tun = hasTunLeft

	if !isAdminCheck() {
		// Removing the DNS override / the orphan adapter's default route needs
		// elevated rights (admin on Windows, root on macOS/Linux). Don't fire an
		// elevation prompt here — report up so the user is asked to restart
		// elevated (one explicit click instead of a surprise UAC / password
		// dialog). RestartAsAdmin is implemented on every platform.
		scan.NeedsElevation = true
		return
	}

	if hasDNSLeft && !m.sessionActive() {
		if err := m.sysDNS.Restore(); err != nil {
			m.logDNSRestoreWarning(err)
		}
	}
	if hasTunLeft && !m.sessionActive() {
		if err := clearLeftoverTunFn(); err != nil {
			m.log.Warning(fmt.Sprintf("[СИСТЕМА] Ошибка снятия остаточного туннель-адаптера: %v", err))
		} else {
			m.log.Success("[СИСТЕМА] Снят остаточный туннель-адаптер и его маршрут по умолчанию")
		}
	}
	return
}

// sessionActive reports whether one of our own connections is live or in the
// middle of being established (Connect sets pendingProxy before booting the
// engine, so this is true from the very start of a connect, before the sing-tun
// adapter even exists). Used by RecoverSystemLeftovers to never revert state
// that belongs to a running session.
func (m *Manager) sessionActive() bool {
	m.mu.Lock()
	active := m.connected || m.pendingProxy != nil
	m.mu.Unlock()
	if active {
		return true
	}
	return m.engine != nil && m.engine.IsRunning()
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
		KillSwitchEmergency: m.proxyDead && m.killSwitch && m.isAdmin,
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
