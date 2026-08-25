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
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/include"
	sblog "github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/bufio"
	singjson "github.com/sagernet/sing/common/json"
	N "github.com/sagernet/sing/common/network"

	"resultproxy-wails/internal/logger"
)


// isTunIPv6Error reports whether a tunnel start failed specifically because the
// Wintun adapter would not take an IPv6 address ("configure tun interface: set
// ipv6 address: ..."). This is NOT the same class as a wedged adapter: no amount
// of device removal fixes it and a retry with the same config reproduces it
// exactly, so the only remedy is to rebuild the config without IPv6
// (EngineConfig.TunDisableIPv6).
//
// Deliberately narrow — "set ipv4 address" failures of the identical shape must
// not match, because dropping IPv6 cannot fix them and would only hide the real
// cause behind a silently degraded tunnel.
func isTunIPv6Error(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "set ipv6 address")
}

func ClassifyEngineStartError(mode ProxyMode, err error) (tunnelFailed bool, reason string, errorCode string) {
	if err == nil {
		return false, "", ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	// "set ipv6 address: Element not found" comes from sing-tun on hosts where
	// the IPv6 stack is disabled. It superficially matches "configure tun
	// interface" but isn't a privileges problem — surface it as its own code
	// so the UI doesn't send the user on a futile run-as-admin loop and so
	// callers (BuildTunnelModeConfig) can retry with IPv4-only.
	if mode == ProxyModeTunnel &&
		strings.Contains(lower, "set ipv6 address") {
		return true, extractErrorReason(msg), "tun_ipv6_unavailable"
	}
	if mode == ProxyModeTunnel &&
		(strings.Contains(lower, "configure tun interface") ||
			strings.Contains(lower, "inbound/tun") ||
			strings.Contains(lower, "access is denied")) {
		return true, extractErrorReason(msg), ConnectErrorTunPrivileges
	}
	if strings.Contains(lower, "dns") {
		return false, extractErrorReason(msg), "dns_error"
	}
	if strings.Contains(lower, "route") || strings.Contains(lower, "outbound") || strings.Contains(lower, "endpoint") {
		return false, extractErrorReason(msg), "route_error"
	}
	if strings.Contains(lower, "handshake") || strings.Contains(lower, "tls") || strings.Contains(lower, "quic") {
		return false, extractErrorReason(msg), "handshake_error"
	}
	return false, extractErrorReason(msg), ConnectErrorEngineStart
}

func extractErrorReason(msg string) string {
	if idx := strings.LastIndex(msg, ": "); idx >= 0 && idx+2 < len(msg) {
		return msg[idx+2:]
	}
	return msg
}



type SingBoxEngine struct {
	mu         sync.Mutex
	running    atomic.Bool
	log        *logger.Logger
	cancel     context.CancelFunc
	configPath string
	instance   *box.Box

	// savedCfg / savedCtx are the original Start args, kept so ApplyAppWhitelist
	// can rebuild the sing-box config in-place without reconstructing the
	// caller's intent. They are only meaningful while running.
	savedCfg EngineConfig
	savedCtx context.Context


	uploadBytes   atomic.Int64
	downloadBytes atomic.Int64

	// proxyUploadBytes/proxyDownloadBytes count ONLY traffic carried by the
	// proxy/endpoint outbound (the tracker increments them only when the matched
	// outbound is not direct/block). The watchdog's traffic veto reads these so
	// direct/split-tunnel traffic cannot make a dead upstream look alive.
	proxyUploadBytes   atomic.Int64
	proxyDownloadBytes atomic.Int64
}


type singBoxLogWriter struct {
	log *logger.Logger
	// redact holds server identifiers (domain + resolved IP) that must never
	// surface in logs for subscription servers — sing-box errors like
	// "lookup <domain>: ..." or "open connection ... using outbound" would
	// otherwise leak the provider's backend address. Empty for manual servers.
	redact []string
}

// newSingBoxLogWriter builds a log writer that hides the server's domain/IP when
// the active proxy comes from a subscription. Manual servers keep full detail —
// the user owns them and the address is already visible in the UI.
func newSingBoxLogWriter(log *logger.Logger, proxy ProxyConfig) *singBoxLogWriter {
	w := &singBoxLogWriter{log: log}
	if proxy.SubscriptionURL == "" {
		return w
	}
	toks := []string{proxy.IP, proxy.ResolvedIP}
	toks = append(toks, proxy.ResolvedIPs...)
	for _, tok := range toks {
		tok = strings.TrimSpace(tok)
		if tok != "" {
			w.redact = append(w.redact, tok)
		}
	}
	return w
}

func (w *singBoxLogWriter) redactServer(msg string) string {
	for _, tok := range w.redact {
		if strings.Contains(msg, tok) {
			msg = strings.ReplaceAll(msg, tok, "<сервер>")
		}
	}
	return msg
}

var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// engineSecretRE matches the UAPI-style "key=value" pairs that carry secret
// material. sing-box-extended reports IpcSet failures by dumping the entire
// ipcConf into the error text (transport/wireguard/endpoint.go), so a single
// failed WireGuard/AmneziaWG setup would otherwise write the private key —
// and the AWG 3.0 header-protection key — straight into the user-visible log.
// public_key is deliberately absent: it is not secret and keeps the dump
// diagnosable.
var engineSecretRE = regexp.MustCompile(`(?i)\b(private_key|pre_shared_key|preshared_key|header_protection_key)=\S*`)

func redactEngineSecrets(msg string) string {
	if !strings.Contains(msg, "_key=") {
		return msg
	}
	return engineSecretRE.ReplaceAllString(msg, "$1=<скрыто>")
}

func (w *singBoxLogWriter) WriteMessage(level sblog.Level, message string) {
	if level > sblog.LevelWarn {
		return
	}

	// Fast filter on the raw message first — strings.Contains is cheap and
	// dropped messages dominate during connection storms, so we avoid the
	// regex + ToLower allocation entirely for them.
	lowerRaw := strings.ToLower(message)
	if strings.Contains(lowerRaw, "dns: exchange failed") ||
		strings.Contains(lowerRaw, "process dns packet") {
		return
	}
	if strings.Contains(lowerRaw, "outbound/direct") &&
		(strings.Contains(lowerRaw, "i/o timeout") ||
			strings.Contains(lowerRaw, "connectex") ||
			strings.Contains(lowerRaw, "actively refused")) {
		return
	}
	// probe-in is our own loopback inbound (see BuildTunnelModeConfig): the only client
	// is the kill-switch watchdog, whose http.Client hangs up at its 4s timeout
	// and closes the body without draining it. sing-box then fails the write
	// back and reports WSAECONNABORTED / "http2: response body closed" at ERROR.
	// The probe's verdict is already logged by the watchdog itself
	// ("[KILL SWITCH] Проба не прошла"), so this half is pure self-inflicted
	// wreckage — it told the user an error had occurred every time a health
	// check ran slow.
	if strings.Contains(lowerRaw, "[probe-in]") {
		return
	}
	// A peer that resets an idle session instead of closing it — Google does
	// this constantly, and a phone hotspot's NAT adds more. Windows renders
	// ECONNRESET as "forcibly closed by the remote host". The transfer already
	// completed; the reset is bookkeeping, and at ERROR level it drowned the
	// log. Scoped to the copy-loop's own wording so a reset reported by any
	// other stage still surfaces.
	if strings.Contains(lowerRaw, "connection download closed") ||
		strings.Contains(lowerRaw, "connection upload closed") {
		if strings.Contains(lowerRaw, "forcibly closed by the remote host") ||
			strings.Contains(lowerRaw, "connection reset by peer") {
			return
		}
	}

	// Only strip ANSI escapes when actually present.
	clean := message
	if strings.IndexByte(message, '\x1b') >= 0 {
		clean = ansiEscapeRE.ReplaceAllString(message, "")
	}

	msg := "[SING-BOX] " + redactEngineSecrets(w.redactServer(clean))
	if level <= sblog.LevelError {
		w.log.Error(msg)
	} else if level == sblog.LevelWarn {
		w.log.Warning(msg)
	}
}



type trafficTracker struct {
	upload        *atomic.Int64
	download      *atomic.Int64
	proxyUpload   *atomic.Int64
	proxyDownload *atomic.Int64
	log           *logger.Logger
	server        string
	protocol      string
	mode          ProxyMode
	logged        sync.Map
	count         atomic.Int32
	rotateMu      sync.Mutex
	isSub         bool
}

// loggedRotateThreshold is how many unique host→outbound pairs we remember
// before clearing the dedup map. Without rotation the map grew unbounded
// over multi-hour sessions (every new domain a browser hits added an entry),
// adding measurable latency to sing-box's connection hot path. With rotation
// we keep at most ~threshold entries in flight; the cost is that a host
// already seen but rotated out will produce a fresh "connected" log line —
// acceptable trade-off, and arguably useful for diagnosing long sessions.
const loggedRotateThreshold = 1000

type trackedConn struct {
	net.Conn
	host   string
	dest   string
	server string
	protocol string
	mode     ProxyMode
	log    *logger.Logger
	start  time.Time
	up     atomic.Int64
	down   atomic.Int64
	closed atomic.Bool
	isSub  bool
}

func (c *trackedConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.down.Add(int64(n))
	}
	return n, err
}

func (c *trackedConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.up.Add(int64(n))
	}
	return n, err
}

func (c *trackedConn) Close() error {
	if c.closed.Swap(true) {
		return c.Conn.Close()
	}
	err := c.Conn.Close()
	up := c.up.Load()
	down := c.down.Load()
	ageMs := time.Since(c.start).Milliseconds()
	viaStr := fmt.Sprintf(" | via %s", c.server)
	if c.isSub {
		viaStr = ""
	}

	if down == 0 && up < 512 && ageMs < 1200 {
		msg := fmt.Sprintf("[CONN] %s -> %s%s | protocol=%s mode=%s | status: closed_early age=%dms", c.host, c.dest, viaStr, c.protocol, c.mode, ageMs)
		c.log.LogWithSource(msg, logger.TypeWarning, c.host, "", c.host)
		return err
	}
	msg := fmt.Sprintf("[CONN] %s -> %s%s | status: closed", c.host, c.dest, viaStr)
	c.log.LogWithSource(msg, logger.TypeInfo, c.host, "", c.host)
	return err
}


func NewSingBoxEngine(log *logger.Logger) *SingBoxEngine {
	return &SingBoxEngine{log: log}
}


func (e *SingBoxEngine) Start(ctx context.Context, cfg EngineConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running.Load() {
		return fmt.Errorf("engine already running")
	}

	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = resultProxyDataDir()
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("data directory: %w", err)
	}

	if err := e.bootLocked(ctx, cfg, true); err != nil {
		return err
	}

	e.savedCfg = cfg
	e.savedCtx = ctx
	e.running.Store(true)

	if cfg.Proxy.SubscriptionURL != "" {
		e.log.Success(fmt.Sprintf("[SING-BOX] Конфигурация готова (%s)", cfg.Mode))
	} else {
		e.log.Success(fmt.Sprintf("[SING-BOX] Конфигурация готова (%s → %s:%d)",
			cfg.Mode, cfg.Proxy.IP, cfg.Proxy.Port))
	}
	return nil
}

// bootLocked builds the sing-box config from cfg, parses it, and starts a fresh
// instance. Caller must hold e.mu. Stores instance/cancel/configPath in the
// engine on success. Used by both Start (initial boot) and ApplyAppWhitelist
// (in-place reload). When announceMode is true an info line about the chosen
// mode is logged — silenced during reload to avoid noise.
func (e *SingBoxEngine) bootLocked(ctx context.Context, cfg EngineConfig, announceMode bool) error {
	var sbConfig SingBoxConfig
	var buildErr error
	switch cfg.Mode {
	case ProxyModeTunnel:
		sbConfig, buildErr = BuildTunnelModeConfig(cfg)
		if announceMode {
			e.log.Info("[SING-BOX] Режим: Туннелирование (TUN)")
		}
	default:
		sbConfig, buildErr = BuildProxyModeConfig(cfg)
		if announceMode {
			e.log.Info("[SING-BOX] Режим: Системный прокси (mixed)")
		}
	}
	if buildErr != nil {
		return fmt.Errorf("sing-box config: %w", buildErr)
	}

	configJSON, err := json.MarshalIndent(sbConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling sing-box config: %w", err)
	}

	tmpDir := os.TempDir()
	configPath := filepath.Join(tmpDir, "resultproxy-singbox.json")
	if err := os.WriteFile(configPath, configJSON, 0o600); err != nil {
		return fmt.Errorf("writing sing-box config: %w", err)
	}

	boxCtx, cancel := context.WithCancel(ctx)
	boxCtx = include.Context(boxCtx)

	var options option.Options
	if err := singjson.UnmarshalContext(boxCtx, configJSON, &options); err != nil {
		cancel()
		return fmt.Errorf("parsing options: %w", err)
	}

	instance, err := box.New(box.Options{
		Context:           boxCtx,
		Options:           options,
		PlatformLogWriter: newSingBoxLogWriter(e.log, cfg.Proxy),
	})
	if err != nil {
		cancel()
		return fmt.Errorf("creating sing-box instance: %w", err)
	}

	if announceMode {
		// Counters reset on first start; preserved across reloads.
		e.uploadBytes.Store(0)
		e.downloadBytes.Store(0)
		e.proxyUploadBytes.Store(0)
		e.proxyDownloadBytes.Store(0)
	}

	tracker := &trafficTracker{
		upload:        &e.uploadBytes,
		download:      &e.downloadBytes,
		proxyUpload:   &e.proxyUploadBytes,
		proxyDownload: &e.proxyDownloadBytes,
		log:           e.log,
		server:        fmt.Sprintf("%s:%d", cfg.Proxy.IP, cfg.Proxy.Port),
		protocol:      strings.ToLower(strings.TrimSpace(cfg.Proxy.Type)),
		mode:          cfg.Mode,
		isSub:         cfg.Proxy.SubscriptionURL != "",
	}
	instance.Router().AppendTracker(tracker)

	if err := instance.Start(); err != nil {
		// Start can partially bring up the TUN inbound (Wintun adapter + WFP
		// filters for strict_route) before failing. Releasing it here lets the
		// caller's retry get a clean slate: cancel() unwinds goroutines but not
		// the OS-level adapter/filters, which would otherwise linger and make the
		// next CreateAdapter fail with "configure tun interface / access denied".
		closeInstanceBounded(instance, 5*time.Second, e.log)
		cancel()
		return fmt.Errorf("starting sing-box: %w", err)
	}

	e.configPath = configPath
	e.instance = instance
	e.cancel = cancel
	return nil
}

// closeInstanceBounded closes a sing-box instance with a hard ceiling, returning
// once Close finishes or the ceiling elapses.
//
// Close runs in a goroutine. Synchronous Close (which we briefly used to avoid a
// goroutine leak under TUN/DNS handles) deadlocked the disconnect path for
// WireGuard / AmneziaWG sessions: the upstream wireguard endpoint's Close blocks
// on UDP socket teardown for many seconds, and while it ran the caller held e.mu
// — so the next manager.Disconnect() call (which also goes through engine.Stop →
// e.mu) never returned, freezing the UI until the process was killed. On timeout
// the goroutine is left running: a single stale sing-box instance is
// GC-collected eventually; a frozen disconnect button is not.
func closeInstanceBounded(inst *box.Box, ceiling time.Duration, log *logger.Logger) {
	closeDone := make(chan struct{}, 1)
	started := time.Now()
	go func() {
		_ = inst.Close()
		closeDone <- struct{}{}
	}()
	select {
	case <-closeDone:
		if elapsed := time.Since(started); elapsed > 3*time.Second {
			log.Warning(fmt.Sprintf("[SING-BOX] Close занял %s", elapsed.Round(100*time.Millisecond)))
		}
	case <-time.After(ceiling):
		log.Warning("[SING-BOX] Close() timeout — продолжаем без ожидания (goroutine завершится позже)")
	}
}

// shutdownInstanceLocked cancels the running sing-box instance and removes the
// on-disk config. Caller must hold e.mu. Does not flip e.running — that is the
// caller's job, since Stop and reload have different semantics.
func (e *SingBoxEngine) shutdownInstanceLocked() {
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	if e.instance != nil {
		inst := e.instance
		e.instance = nil
		closeInstanceBounded(inst, 5*time.Second, e.log)
	}
	if e.configPath != "" {
		os.Remove(e.configPath)
		e.configPath = ""
	}
}

// ApplyAppWhitelist replaces the active app whitelist by tearing down and
// rebuilding the sing-box instance with merged paths. The traffic counters
// and saved config are preserved across the reload. If the resulting whitelist
// is identical to the current one, this is a no-op. Returns nil if the engine
// is not running.
//
// There's a brief routing gap (~200-500ms) while the new instance starts —
// existing TCP/UDP flows survive at the OS level but new app-level
// connections during the gap may fail and retry. Acceptable trade-off
// versus leaving exclusion rules stale.
func (e *SingBoxEngine) ApplyAppWhitelist(paths []string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running.Load() {
		return nil
	}

	if appWhitelistEqual(e.savedCfg.AppWhitelist, paths) {
		return nil
	}

	newCfg := e.savedCfg
	newCfg.AppWhitelist = append([]string(nil), paths...)

	e.shutdownInstanceLocked()
	if err := e.bootLocked(e.savedCtx, newCfg, false); err != nil {
		// Reload failed — engine is now stopped. Flip running so callers see
		// a consistent state and don't keep applying changes to a dead engine.
		e.running.Store(false)
		e.log.Error(fmt.Sprintf("[SING-BOX] Hot-reload failed: %v", err))
		return err
	}

	e.savedCfg = newCfg
	e.log.Info(fmt.Sprintf("[SING-BOX] App whitelist обновлён: %d записей", len(paths)))
	return nil
}

func appWhitelistEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]struct{}, len(a))
	for _, x := range a {
		seen[strings.ToLower(strings.TrimSpace(x))] = struct{}{}
	}
	for _, x := range b {
		if _, ok := seen[strings.ToLower(strings.TrimSpace(x))]; !ok {
			return false
		}
	}
	return true
}


func (e *SingBoxEngine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running.Load() {
		return nil
	}

	e.shutdownInstanceLocked()
	e.savedCfg = EngineConfig{}
	e.savedCtx = nil
	e.running.Store(false)
	e.log.Info("[SING-BOX] Остановлен")

	return nil
}


func (e *SingBoxEngine) IsRunning() bool {
	return e.running.Load()
}


func (e *SingBoxEngine) GetTrafficStats() (up, down int64) {
	return e.uploadBytes.Load(), e.downloadBytes.Load()
}

func (e *SingBoxEngine) GetProxyTrafficStats() (up, down int64) {
	return e.proxyUploadBytes.Load(), e.proxyDownloadBytes.Load()
}

// downCounters/upCounters return the byte counters a routed connection should
// increment. Every connection feeds the global counters; only proxy-outbound
// connections (shouldTrack) additionally feed the proxy-only counters used by
// the watchdog traffic veto.
func (t *trafficTracker) downCounters(shouldTrack bool) []*atomic.Int64 {
	if shouldTrack {
		return []*atomic.Int64{t.download, t.proxyDownload}
	}
	return []*atomic.Int64{t.download}
}

func (t *trafficTracker) upCounters(shouldTrack bool) []*atomic.Int64 {
	if shouldTrack {
		return []*atomic.Int64{t.upload, t.proxyUpload}
	}
	return []*atomic.Int64{t.upload}
}

func (t *trafficTracker) RoutedConnection(
	_ context.Context,
	conn net.Conn,
	metadata adapter.InboundContext,
	_ adapter.Rule,
	matchOutbound adapter.Outbound,
) net.Conn {
	host, dest, shouldTrack := t.logConnection(metadata, matchOutbound)
	wrapped := bufio.NewInt64CounterConn(conn, t.downCounters(shouldTrack), t.upCounters(shouldTrack))
	if !shouldTrack {
		return wrapped
	}
	return &trackedConn{
		Conn:   wrapped,
		host:   host,
		dest:   dest,
		server: t.server,
		protocol: t.protocol,
		mode:     t.mode,
		log:    t.log,
		start:  time.Now(),
		isSub:  t.isSub,
	}
}

func (t *trafficTracker) RoutedPacketConnection(
	_ context.Context,
	conn N.PacketConn,
	metadata adapter.InboundContext,
	_ adapter.Rule,
	matchOutbound adapter.Outbound,
) N.PacketConn {
	_, _, shouldTrack := t.logConnection(metadata, matchOutbound)
	return bufio.NewInt64CounterPacketConn(conn, t.downCounters(shouldTrack), nil, t.upCounters(shouldTrack), nil)
}

// rotateLogged drops every entry in the dedup map and resets the counter.
// Called from the hot path once the unique-host count crosses the threshold.
// Safe under concurrent callers: the mutex serializes rotations and the
// re-check inside prevents a stampede where many goroutines simultaneously
// observe count > threshold.
func (t *trafficTracker) rotateLogged() {
	t.rotateMu.Lock()
	defer t.rotateMu.Unlock()
	if t.count.Load() <= loggedRotateThreshold {
		return
	}
	t.logged.Range(func(k, _ any) bool {
		t.logged.Delete(k)
		return true
	})
	t.count.Store(0)
	t.log.Info(fmt.Sprintf("[CONN] Буфер детализации очищен (превышен порог %d уникальных хостов)", loggedRotateThreshold))
}

func (t *trafficTracker) logConnection(metadata adapter.InboundContext, outbound adapter.Outbound) (string, string, bool) {
	dest := metadata.Destination.String()
	if dest == "" {
		return "", "", false
	}

	outTag := "direct"
	if outbound != nil {
		outTag = outbound.Tag()
	}

	host := metadata.Domain
	if host == "" {
		return "", "", false
	}

	key := host + "→" + outTag
	if _, loaded := t.logged.LoadOrStore(key, struct{}{}); loaded {
		return host, dest, false
	}
	if t.count.Add(1) > loggedRotateThreshold {
		t.rotateLogged()
	}

	// Direct and blocked traffic is not a connection "through" anything, so it
	// gets no [CONN] line. It used to get a [ROUTE-DIAG] warning for anything
	// matching a YouTube-shaped host, left over from a routing investigation —
	// but that filter keyed on the substring "gvt2.com", which is Chrome's
	// telemetry and Google Update beacon domain, not video. Every beacon fired
	// a warning about a routing decision that was correct: in Smart mode
	// Final="direct" (buildRoute), so anything off the censored block-list is
	// supposed to go direct.
	if outTag == "direct" || outTag == "block" {
		return host, dest, false
	}

	viaStr := fmt.Sprintf(" | via %s", t.server)
	if t.isSub {
		viaStr = ""
	}
	msg := fmt.Sprintf("[CONN] %s -> %s%s | status: connected", host, dest, viaStr)
	t.log.LogWithSource(msg, logger.TypeInfo, host, "", metadata.Domain)
	return host, dest, true
}


func (e *SingBoxEngine) GetConfigJSON() (string, error) {
	if e.configPath == "" {
		return "", fmt.Errorf("no config available")
	}
	data, err := os.ReadFile(e.configPath)
	if err != nil {
		return "", fmt.Errorf("reading config: %w", err)
	}
	return string(data), nil
}
