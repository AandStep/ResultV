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


func ClassifyEngineStartError(mode ProxyMode, err error) (tunnelFailed bool, reason string, errorCode string) {
	if err == nil {
		return false, "", ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
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

	
	uploadBytes   atomic.Int64
	downloadBytes atomic.Int64
}


type singBoxLogWriter struct {
	log *logger.Logger
}

var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func (w *singBoxLogWriter) WriteMessage(level sblog.Level, message string) {
	
	
	if level > sblog.LevelWarn {
		return
	}

	clean := ansiEscapeRE.ReplaceAllString(message, "")
	lower := strings.ToLower(clean)

	
	if strings.Contains(lower, "dns: exchange failed") ||
		strings.Contains(lower, "process dns packet") {
		return
	}

	
	
	
	if strings.Contains(lower, "outbound/direct") && 
		(strings.Contains(lower, "i/o timeout") || strings.Contains(lower, "connectex") || strings.Contains(lower, "actively refused")) {
		return
	}

	msg := "[SING-BOX] " + clean
	if level <= sblog.LevelError {
		w.log.Error(msg)
	} else if level == sblog.LevelWarn {
		w.log.Warning(msg)
	}
}



type trafficTracker struct {
	upload   *atomic.Int64
	download *atomic.Int64
	log      *logger.Logger
	server   string
	protocol string
	mode     ProxyMode
	logged   sync.Map 
	count    atomic.Int32
	capped   atomic.Bool
	isSub    bool
}

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

func isVideoDiagnosticsHost(s string) bool {
	h := strings.ToLower(strings.TrimSpace(s))
	if h == "" {
		return false
	}
	return strings.Contains(h, "youtube") ||
		strings.Contains(h, "googlevideo") ||
		strings.Contains(h, "ytimg") ||
		strings.Contains(h, "gvt1.com") ||
		strings.Contains(h, "gvt2.com")
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

	if isVideoDiagnosticsHost(c.host) || isVideoDiagnosticsHost(c.dest) {
		msg := fmt.Sprintf("[CONN-DIAG] %s -> %s%s | status: closed | up=%dB down=%dB age=%dms", c.host, c.dest, viaStr, up, down, ageMs)
		c.log.LogWithSource(msg, logger.TypeInfo, c.host, "", c.host)
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

	
	var sbConfig SingBoxConfig
	switch cfg.Mode {
	case ProxyModeTunnel:
		sbConfig = BuildTunnelModeConfig(cfg)
		e.log.Info("[SING-BOX] Режим: Туннелирование (TUN)")
	default:
		sbConfig = BuildProxyModeConfig(cfg)
		e.log.Info("[SING-BOX] Режим: Системный прокси (mixed)")
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
	e.configPath = configPath
	e.log.Info(fmt.Sprintf("[SING-BOX] Конфиг записан: %s", configPath))

	
	
	
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
		PlatformLogWriter: &singBoxLogWriter{log: e.log},
	})
	if err != nil {
		cancel()
		return fmt.Errorf("creating sing-box instance: %w", err)
	}

	e.uploadBytes.Store(0)
	e.downloadBytes.Store(0)

	
	tracker := &trafficTracker{
		upload:   &e.uploadBytes,
		download: &e.downloadBytes,
		log:      e.log,
		server:   fmt.Sprintf("%s:%d", cfg.Proxy.IP, cfg.Proxy.Port),
		protocol: strings.ToLower(strings.TrimSpace(cfg.Proxy.Type)),
		mode:     cfg.Mode,
		isSub:    cfg.Proxy.SubscriptionURL != "",
	}
	instance.Router().AppendTracker(tracker)

	if err := instance.Start(); err != nil {
		cancel()
		return fmt.Errorf("starting sing-box: %w", err)
	}
	e.instance = instance
	e.cancel = cancel

	e.running.Store(true)

	if cfg.Proxy.SubscriptionURL != "" {
		e.log.Success(fmt.Sprintf("[SING-BOX] Конфигурация готова (%s)", cfg.Mode))
	} else {
		e.log.Success(fmt.Sprintf("[SING-BOX] Конфигурация готова (%s → %s:%d)",
			cfg.Mode, cfg.Proxy.IP, cfg.Proxy.Port))
	}

	return nil
}


func (e *SingBoxEngine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running.Load() {
		return nil
	}

	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}

	if e.instance != nil {
		inst := e.instance
		e.instance = nil
		// instance.Close() can block indefinitely when sing-box goroutines
		// are stuck on network I/O (e.g. after system sleep/resume). Run it
		// in a goroutine with a hard timeout so Shutdown never hangs.
		closeDone := make(chan struct{}, 1)
		go func() {
			_ = inst.Close()
			closeDone <- struct{}{}
		}()
		select {
		case <-closeDone:
		case <-time.After(5 * time.Second):
			e.log.Warning("[SING-BOX] Close() timeout — принудительное завершение")
		}
	}

	if e.configPath != "" {
		os.Remove(e.configPath)
		e.configPath = ""
	}

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

func (t *trafficTracker) RoutedConnection(
	_ context.Context,
	conn net.Conn,
	metadata adapter.InboundContext,
	_ adapter.Rule,
	matchOutbound adapter.Outbound,
) net.Conn {
	host, dest, shouldTrack := t.logConnection(metadata, matchOutbound)
	wrapped := bufio.NewInt64CounterConn(conn, []*atomic.Int64{t.download}, []*atomic.Int64{t.upload})
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
	t.logConnection(metadata, matchOutbound)
	return bufio.NewInt64CounterPacketConn(conn, []*atomic.Int64{t.download}, nil, []*atomic.Int64{t.upload}, nil)
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
	if t.count.Add(1) > 1000 {
		if !t.capped.Swap(true) {
			t.log.Warning("[CONN] Достигнут лимит детализации (1000 доменов). Показываются только ошибки и ключевые события.")
		}
		return host, dest, false
	}

	if outTag == "direct" || outTag == "block" {
		if isVideoDiagnosticsHost(host) {
			msg := fmt.Sprintf("[ROUTE-DIAG] %s -> %s | outbound=%s", host, dest, outTag)
			t.log.LogWithSource(msg, logger.TypeWarning, host, "", metadata.Domain)
		}
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
