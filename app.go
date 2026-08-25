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
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
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
	"syscall"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/logger"
	"resultproxy-wails/internal/proxy"
	"resultproxy-wails/internal/system"
	"resultproxy-wails/internal/updater"
)

var stableHWIDProvider = config.StableHardwareID

// sameHostReferer returns referer when its host matches imageURL's host
// (case-insensitive), else "". Stops the subscription URL (often containing
// an opaque access token in the path) from leaking to third-party icon hosts.
func sameHostReferer(referer, imageURL string) string {
	if strings.TrimSpace(referer) == "" {
		return ""
	}
	ru, err := url.Parse(referer)
	if err != nil || ru.Host == "" {
		return ""
	}
	iu, err := url.Parse(imageURL)
	if err != nil || iu.Host == "" {
		return ""
	}
	if !strings.EqualFold(ru.Host, iu.Host) {
		return ""
	}
	return referer
}

const subscriptionPageUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

type subscriptionRequestMetadata struct {
	UserAgent string
	Platform  string
	OSVersion string
	Model     string
	SendHWID  bool
}

type App struct {
	ctx    context.Context
	cancel context.CancelFunc

	log        *logger.Logger
	crypto     *config.CryptoService
	config     *config.Manager
	proxy      *proxy.Manager
	tray       *system.Tray
	killSwitch system.KillSwitch
	netmon     *system.NetMonitor

	trayIcon []byte

	stateMu       sync.Mutex
	quitRequested bool

	trayHidden    atomic.Uint32
	taskbarUnhook func()
	smartProvider *proxy.HTTPBlockedListProvider

	// lastAutoNodeKey is the AutoNodeKey of the AUTO member currently in use.
	// Passed as previousKey to RankAutoCandidates so the active node is always
	// re-measured on equal footing instead of flapping in and out of the
	// shortlist. Empty until a connection is made.
	//
	// lastAutoNodeKeyMu guards it: every tray click dispatches on its own
	// goroutine (system/tray.go's `go t.safeCall(...)`), so a click reading it
	// via ResolveAutoCandidates can run concurrently with another click's
	// connectFromTray writing it. Use getLastAutoNodeKey/setLastAutoNodeKey,
	// never the field directly.
	lastAutoNodeKeyMu sync.RWMutex
	lastAutoNodeKey   string

	// Ranked candidates from the last ResolveAutoCandidates call, per AUTO head
	// ID. ReportAutoConnectOutcome needs the full member entry to rebuild the
	// same AutoNodeKey the probe used; the UI only knows the address.
	autoCandidatesMu sync.Mutex
	autoCandidates   map[string][]config.ProxyEntry

	startInTray bool

	deepLinkMu      sync.Mutex
	pendingDeepLink string

	updateMu     sync.Mutex
	updateCancel context.CancelFunc

	// dataDirOverride, when non-empty, replaces getUserDataPath() as the
	// routing-list cache directory. Production leaves it empty (so caches land
	// in system.UserDataDir(), exactly where the engine's buildRoute stats
	// them); tests set it to a temp dir. See routingListDataDir.
	dataDirOverride string

	// leftoverReport holds what startup recovery cleaned up after a prior
	// unclean exit, consumed once by the frontend to show an informational
	// notice. Guarded by leftoverMu.
	leftoverMu     sync.Mutex
	leftoverReport LeftoverReport

	// leftoverDone is closed by recoverLeftovers when startup leftover cleanup
	// finishes (DNS override reverted etc.). The background Smart-list refresh
	// waits on it (bounded) so it only hits the network AFTER a post-crash stale
	// DNS override is gone — otherwise the remote fetch times out and the user
	// silently falls back to direct routing. nil until startup wires it.
	leftoverDone chan struct{}
}

func NewApp() *App {
	return &App{
		log:            logger.New(),
		autoCandidates: map[string][]config.ProxyEntry{},
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

func (a *App) subscriptionRequestMetadata() subscriptionRequestMetadata {
	settings := config.DefaultConfig().Settings
	if a.config != nil {
		settings = a.config.GetConfig().Settings
	}
	info := system.SubscriptionDeviceInfo()
	userAgent := strings.TrimSpace(settings.SubscriptionUserAgent)
	if userAgent == "" {
		platform := strings.TrimSpace(info.Platform)
		if platform == "" {
			platform = runtime.GOOS
		}
		uaPlatform := platform
		if strings.EqualFold(platform, "windows") {
			uaPlatform = "Windows"
		}
		uaVersion := strings.TrimSpace(info.OSVersion)
		if uaVersion == "" {
			uaVersion = runtime.GOOS + "_" + runtime.GOARCH
		}
		userAgent = fmt.Sprintf("ResultV/%s/%s/%s", productVersionFromWailsJSON(), uaPlatform, sanitizeUserAgentSegment(uaVersion))
	}
	return subscriptionRequestMetadata{
		UserAgent: userAgent,
		Platform:  firstNonEmpty(strings.TrimSpace(info.Platform), runtime.GOOS),
		OSVersion: firstNonEmpty(strings.TrimSpace(info.OSVersion), runtime.GOOS+"_"+runtime.GOARCH),
		Model:     firstNonEmpty(strings.TrimSpace(info.Model), runtime.GOOS+"_"+runtime.GOARCH),
		SendHWID:  settings.EffectiveSubscriptionSendHWID(),
	}
}

func sanitizeUserAgentSegment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "_", "\t", "_", "\n", "_", "\r", "_", "/", "_")
	return replacer.Replace(s)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// GetUpdateManifest fetches update.json via the Go backend.
// This avoids WebView fetch/CORS/network-policy issues on some Windows setups.
func (a *App) GetUpdateManifest() (*updater.Manifest, error) {
	u := updater.New()
	base := context.Background()
	if a != nil && a.ctx != nil {
		base = a.ctx
	}
	ctx, cancel := context.WithTimeout(base, 20*time.Second)
	defer cancel()
	return u.Check(ctx)
}

// DebugFrontendLog writes a frontend diagnostic line into the shared app log.
// It is intentionally lightweight and best-effort: failures should never
// affect user flows.
func (a *App) DebugFrontendLog(message string) {
	if a == nil || a.log == nil {
		return
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		return
	}
	a.log.Info("[FRONTEND] " + msg)
}

// QueueDeepLink stores a resultv:// URL to be processed once startup finishes.
// Called from main() before Wails has booted.
func (a *App) QueueDeepLink(url string) {
	if !proxy.IsDeepLink(url) {
		return
	}
	a.deepLinkMu.Lock()
	a.pendingDeepLink = url
	a.deepLinkMu.Unlock()
}

// DecodeDeepLink unwraps a resultv:// URL into its underlying subscription
// payload (an http(s) URL, RVSUB1 body, or proxy URI list). Frontend uses this
// for paste flows so the rest of the import path is identical to the
// browser-click flow, which receives the already-decoded payload via the
// "deeplink:received" event.
func (a *App) DecodeDeepLink(url string) (string, error) {
	return proxy.DecodeDeepLink(url)
}

// HandleDeepLink decrypts a resultv:// URL and forwards the decoded payload
// to the frontend, which routes it through the regular "Add subscription"
// flow (preview modal → user confirms → import). Called both from main() at
// startup and from the singleton messenger when a second instance is launched
// with a deep link.
func (a *App) HandleDeepLink(url string) {
	if a == nil || a.config == nil {
		a.QueueDeepLink(url)
		return
	}
	if !proxy.IsDeepLink(url) {
		return
	}
	payload, err := proxy.DecodeDeepLink(url)
	if err != nil {
		a.log.Error(fmt.Sprintf("Не удалось обработать ссылку resultv://: %v", err))
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "deeplink:error", err.Error())
		}
		return
	}

	a.restoreMainWindow()

	if a.ctx == nil {
		a.QueueDeepLink(url)
		return
	}
	source := ""
	if proxy.DeepLinkUsesRvsubPath(url) {
		source = "rvsub"
	}
	wailsRuntime.EventsEmit(a.ctx, "deeplink:received", map[string]interface{}{
		"payload": payload,
		"source":  source,
	})
}

func (a *App) startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)

	a.log.SetEmitter(func(eventName string, data any) {
		wailsRuntime.EventsEmit(a.ctx, eventName, data)
	})

	a.log.Info("ResultV запускается...")

	// Install the persistent node-stat store before anything can resolve an
	// AUTO group; without it the package falls back to an in-memory store and
	// the history is lost on every launch.
	proxy.SetNodeStatStore(proxy.NewNodeStatStore(system.UserDataDir()))

	if err := system.RegisterResultVProtocol(); err != nil {
		a.log.Warning(fmt.Sprintf("[СИСТЕМА] Не удалось зарегистрировать resultv://: %v", err))
	}

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

	// Must run right after config.Init, while WasCreatedFresh still reflects
	// what Init found on disk.
	a.seedChangelogVersionOnFreshInstall()

	a.proxy = proxy.NewManager(a.log)
	a.proxy.Init(a.ctx)
	// Encrypt the persistent server-IP pin cache with the same hardware-keyed
	// CryptoService as the rest of the config — those hostname→backend-IP
	// entries must never sit on disk in the clear.
	a.proxy.SetSecretCodec(cs)

	// Create the kill switch and launch leftover recovery NOW — before the
	// network-bound, multi-second SMART list fetch in initSmartBlockedDomains
	// below (and the rest of startup). Recovery's only dependencies are a.proxy
	// (just initialized) and a.killSwitch; when it was deferred to the end of
	// startup, a force-kill / crash leftover lingered for the entire startup
	// duration (~15s in the field, dominated by the remote list fetch) before the
	// internet was restored. Run async so the window still appears immediately;
	// the slow route/adapter cleanup only runs when a leftover is actually
	// detected. The frontend shows an informational notice via
	// GetLeftoverRecoveryReport + the "leftovers:recovered" event.
	a.killSwitch = system.NewKillSwitch()
	a.leftoverDone = make(chan struct{})
	go a.recoverLeftovers()

	rootDir := a.getAppRootDir()
	a.initSmartBlockedDomains(userDataPath, rootDir)


	// Leftover kill-switch firewall rules from a crashed / force-killed prior
	// run are NOT silently cleared here (the old Disable() call was a no-op on a
	// fresh process anyway — in-memory state starts disabled). They are detected
	// via HasLeftoverRules and removed by recoverLeftovers (launched above),
	// bundled into the single startup UAC prompt when admin rights are needed.

	a.proxy.KillSwitchFirewallEngage = func(p proxy.ProxyConfig, dns []string) {
		// Allow EVERY backend IP pinned at connect time, not the raw domain: the
		// kill switch fires precisely when DNS is failing, so resolving the host
		// now would return nothing and the firewall ("no proxy IP to allow") would
		// never actually block — leaving the user unprotected on a real outage.
		// Allowing all backends (not just one) is required so that when sing-box
		// re-resolves the server domain and fails over to another CDN backend, the
		// firewall doesn't block it — otherwise the health probe through the new
		// backend fails and the kill switch can never disengage.
		seen := map[string]struct{}{}
		var hosts []string
		addHost := func(h string) {
			h = strings.TrimSpace(h)
			if h == "" || net.ParseIP(h) == nil {
				return
			}
			if _, dup := seen[h]; dup {
				return
			}
			seen[h] = struct{}{}
			hosts = append(hosts, fmt.Sprintf("%s:%d", h, p.Port))
		}
		for _, ip := range p.ResolvedIPs {
			addHost(ip)
		}
		addHost(p.ResolvedIP)
		addHost(p.IP) // no-op unless IP is already a literal
		addr := strings.Join(hosts, ",")
		if addr == "" {
			// No pinned literal (domain server, censored resolver) — fall back to
			// the raw host:port so the firewall layer can try to resolve it.
			addr = fmt.Sprintf("%s:%d", p.IP, p.Port)
		}
		if err := a.enableKillSwitchFirewall(addr, dns); err != nil {
			a.log.Warning(fmt.Sprintf("[KILL SWITCH] Не удалось включить фаервол при недоступности узла: %v", err))
		}
	}
	a.proxy.KillSwitchFirewallDisengage = func() {
		if a.killSwitch != nil && a.killSwitch.IsEnabled() {
			if err := a.killSwitch.Disable(); err != nil {
				a.log.Warning(fmt.Sprintf("[KILL SWITCH] Не удалось снять фаервол после восстановления узла: %v", err))
			} else {
				a.log.Info("[KILL SWITCH] Правила фаервола сняты (узел снова доступен)")
			}
		}
		st := a.proxy.GetStatus()
		if st.IsConnected && st.CurrentProxy != nil {
			cp := st.CurrentProxy
			if a.tray != nil {
				a.tray.SetConnectedProxy(a.resolveProxyID(*cp), fmt.Sprintf("%s:%d", cp.IP, cp.Port))
			}
			a.setWindowTitleConnected(*cp)
		}
	}

	a.netmon = system.NewNetMonitor(func(status system.NetworkStatus) {
		wailsRuntime.EventsEmit(a.ctx, "network:status", status)
		if status.Online {
			a.log.Info("[СЕТЬ] Интернет-соединение восстановлено")
		} else {
			a.log.Warning("[СЕТЬ] Интернет-соединение потеряно")
		}
	})
	// A roam (Wi-Fi -> phone hotspot, cable pulled) changes which physical
	// address probes must leave through, but not necessarily whether the
	// machine is online — so the status handler above would never fire. Without
	// this hook the LAN-bind cache keeps the dead address for up to its 30s TTL
	// and the AUTO sweep cache, keyed on that address, keeps serving rankings
	// measured over a link that no longer exists.
	a.netmon.SetInterfaceChangeHandler(func() {
		proxy.InvalidateLANBindCache()
		proxy.ResetAutoSweepCache()
		if a.log != nil {
			a.log.Info("[СЕТЬ] Изменился состав сетевых адресов — кэш bind-адреса и подбора AUTO сброшен")
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
		OnConnect: func() {
			cfg := a.config.GetConfig()
			last := cfg.Settings.LastSelectedProxyID
			if last == "" {
				a.log.Warning("Из трея запрошено подключение, но сервер не выбран")
				return
			}
			if err := a.connectFromTray(last); err != nil {
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
		OnUnexpectedExit: func() {
			// Systray died without Stop() being called. If the window is
			// hidden to tray at this point, the app is unrecoverable
			// (no icon, no window, singleton mutex held). Exit so the
			// user can relaunch cleanly.
			a.stateMu.Lock()
			quit := a.quitRequested
			a.stateMu.Unlock()
			if !quit && a.trayHidden.Load() != 0 {
				a.log.Warning("Трей неожиданно завершился при скрытом окне — завершение процесса")
				time.Sleep(300 * time.Millisecond)
				os.Exit(0)
			}
		},
	})
	a.tray.Start()
	a.refreshTrayProxyList()

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

	// NOTE: leftover recovery is launched ONCE, early in startup (right after the
	// kill switch is created) — see the `go a.recoverLeftovers()` above. A second
	// launch here used to double both the UAC prompt risk and the race surface
	// with the frontend auto-connect.

	if a.startInTray {
		a.trayHidden.Store(1)
		wailsRuntime.WindowHide(a.ctx)
	}

	a.log.Success("ResultV готов к работе")

	a.deepLinkMu.Lock()
	queued := a.pendingDeepLink
	a.pendingDeepLink = ""
	a.deepLinkMu.Unlock()
	if queued != "" {
		go a.HandleDeepLink(queued)
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.log.Info("ResultV завершает работу...")

	// Free the single-instance lock first thing, so a relaunch during this
	// (possibly slow) teardown starts a fresh, working instance instead of
	// bouncing off us. Harmless if markQuitRequested already released it.
	system.ReleaseSingletonLock()

	done := make(chan struct{})
	go func() {
		defer close(done)

		if a.taskbarUnhook != nil {
			a.taskbarUnhook()
			a.taskbarUnhook = nil
		}

		if a.netmon != nil {
			a.netmon.Stop()
		}

		// Revert OS-level network state FIRST, before the slower UI teardown
		// (tray.Stop waits up to 2s). If anything later hangs and trips the 10s
		// force-exit below, the system proxy / DNS / tun are already restored —
		// the user is never left without internet.
		//
		// Use RemoveLeftoverRules (not Disable) — it deletes all firewall rules
		// regardless of in-memory state and self-elevates via UAC when admin
		// rights are needed. This covers stale flags, toggle-off races, and the
		// non-admin relaunch after a kill-switch crash. Gate it on
		// HasLeftoverRules (one netsh query, ~150ms): the unconditional sweep
		// was ~20 serial netsh spawns on EVERY quit — seconds of delay before
		// the network-critical DNS/route cleanup in proxy.Shutdown below — and
		// on a non-admin instance it fired a pointless UAC prompt. _BlockAll is
		// the only rule that severs traffic; stray allow rules are harmless.
		if a.killSwitch != nil && a.killSwitch.HasLeftoverRules() {
			_ = a.killSwitch.RemoveLeftoverRules()
		}

		if a.proxy != nil {
			a.proxy.Shutdown()
		}

		if a.tray != nil {
			a.tray.Stop()
		}

		if a.cancel != nil {
			a.cancel()
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		// Shutdown hung (likely sing-box instance.Close() blocking) — force exit
		// so the singleton mutex is released and the user can relaunch.
		a.log.Warning("Завершение зависло — принудительный выход")
		os.Exit(0)
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
	killSwitch bool) (proxy.ConnectResultDTO, error) {

	if a.proxy == nil {
		return proxy.ConnectResultDTO{Success: false, Message: "Proxy manager not initialized"}, nil
	}

	cfg := a.config.GetConfig()
	mode := proxy.ProxyMode(cfg.Settings.Mode)
	dnsServers := append([]string(nil), cfg.Settings.DNSServers...)
	if fromProxy := dnsServersFromProxyExtra(proxyDTO); len(fromProxy) > 0 {
		dnsServers = fromProxy
	}
	a.proxy.SetTunStack(cfg.Settings.EffectiveTunStack())

	result := a.proxy.Connect(
		a.ctx,
		proxyDTO,
		mode,
		proxy.RoutingMode(rules.Mode),
		rules.Whitelist,
		rules.AppWhitelist,
		rules.AppForceVPN,
		killSwitch,
		cfg.Settings.LocalPort,
		cfg.Settings.ListenLAN,
		dnsServers,
		cfg.Settings.TunIPv4,
		"",
		cfg.Settings.EffectiveDNSLeakProtection(),
		cfg.Settings.EnableIPv6,
	)

	if result.Success {
		serverName := fmt.Sprintf("%s:%d", proxyDTO.IP, proxyDTO.Port)
		if a.tray != nil {
			a.tray.SetConnectedProxy(a.resolveProxyID(proxyDTO), serverName)
		}
		a.setWindowTitleConnected(proxyDTO)
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

func (a *App) CancelConnect() {
	if a.proxy != nil {
		a.proxy.CancelConnect()
	}
}

func (a *App) Disconnect() error {
	if a.proxy == nil {
		return nil
	}
	err := a.proxy.Disconnect()
	if err == nil {
		if a.killSwitch != nil && a.killSwitch.IsEnabled() {
			if derr := a.killSwitch.Disable(); derr != nil {
				a.log.Error(fmt.Sprintf("[KILL SWITCH] Ошибка снятия правил фаервола при отключении: %v", derr))
			}
		}
		if a.tray != nil {
			a.tray.SetDisconnected()
		}
		a.setWindowTitleDisconnected()
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

// LeftoverReport describes OS-level network state left behind by a previous run
// that exited without cleanup (crash / force-kill via Task Manager). When
// NeedsElevation is false the listed leftovers were removed; when true they
// were only DETECTED — removing them needs admin rights the process lacks, and
// the frontend offers a "restart as admin to clean up" button instead of the
// old surprise mid-startup UAC prompt (which raced the auto-connect and was
// routinely dismissed, leaving a stale default route that caused false
// kill-switch trips).
type LeftoverReport struct {
	Proxy          bool `json:"proxy"`
	DNS            bool `json:"dns"`
	Tun            bool `json:"tun"`
	Firewall       bool `json:"firewall"`
	NeedsElevation bool `json:"needsElevation"`
}

// Any reports whether any leftover was detected.
func (r LeftoverReport) Any() bool { return r.Proxy || r.DNS || r.Tun || r.Firewall }

// recoverLeftovers reverts OS-level network state stranded by a prior unclean
// exit (crash / force-kill): a leftover sing-tun adapter still holding the
// default route (the dominant tunnel-mode "no internet" cause), an un-restored
// DNS override, a stale system proxy, and kill-switch firewall rules. It runs
// in Go at startup with NO UI dependency.
//
// Cleanup that fits the current privileges runs immediately (system proxy never
// needs admin; everything runs directly when elevated). Leftovers that need
// admin while we are NOT elevated are no longer cleaned via a surprise UAC —
// they are reported with NeedsElevation=true and the frontend asks the user to
// restart elevated (RestartAsAdmin); the elevated instance then cleans them on
// ITS startup pass and shows the normal notice.
func (a *App) recoverLeftovers() {
	// Signal the background Smart-list refresh that OS network state (notably the
	// DNS override) has been restored, on every return path. Closed even when
	// nothing needed cleaning — the refresh just stops waiting and proceeds.
	if a.leftoverDone != nil {
		defer close(a.leftoverDone)
	}

	rep := LeftoverReport{}

	var scan proxy.LeftoverScan
	if a.proxy != nil {
		scan = a.proxy.RecoverSystemLeftovers()
	}
	rep.Proxy, rep.DNS, rep.Tun = scan.Proxy, scan.DNS, scan.Tun
	rep.NeedsElevation = scan.NeedsElevation

	if a.killSwitch != nil && a.killSwitch.HasLeftoverRules() {
		rep.Firewall = true
		if system.IsAdmin() {
			if err := a.killSwitch.RemoveLeftoverRules(); err != nil {
				a.log.Warning(fmt.Sprintf("[KILL SWITCH] Ошибка снятия остаточных правил фаервола: %v", err))
			}
		} else {
			// Leftover WFP rules block traffic and need admin to delete. Same
			// policy as DNS/tun: ask for an explicit elevated restart instead of
			// auto-firing UAC.
			rep.NeedsElevation = true
		}
	}

	if !rep.Any() {
		return
	}
	if rep.NeedsElevation {
		a.log.Warning(fmt.Sprintf("[СИСТЕМА] Обнаружены остатки прошлого сеанса, для очистки нужны права администратора: proxy=%v dns=%v tun=%v firewall=%v — перезапустите приложение от имени администратора",
			rep.Proxy, rep.DNS, rep.Tun, rep.Firewall))
	} else {
		a.log.Warning(fmt.Sprintf("[СИСТЕМА] Сняты остатки прошлого сеанса (некорректное завершение): proxy=%v dns=%v tun=%v firewall=%v",
			rep.Proxy, rep.DNS, rep.Tun, rep.Firewall))
	}
	a.leftoverMu.Lock()
	a.leftoverReport = rep
	a.leftoverMu.Unlock()
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "leftovers:recovered", rep)
	}
}

// GetLeftoverRecoveryReport returns the report of leftovers that
// startup recovery cleaned. The frontend pulls this on mount to show a one-time
// notice; the "leftovers:recovered" event covers the case where recovery
// finishes after the frontend has already mounted.
func (a *App) GetLeftoverRecoveryReport() map[string]bool {
	a.leftoverMu.Lock()
	defer a.leftoverMu.Unlock()
	return map[string]bool{
		"proxy":          a.leftoverReport.Proxy,
		"dns":            a.leftoverReport.DNS,
		"tun":            a.leftoverReport.Tun,
		"firewall":       a.leftoverReport.Firewall,
		"needsElevation": a.leftoverReport.NeedsElevation,
	}
}

// ResetLeftoverReport clears the report of leftovers that
// startup recovery cleaned. Called by the frontend once the notice is displayed.
func (a *App) ResetLeftoverReport() string {
	a.leftoverMu.Lock()
	defer a.leftoverMu.Unlock()
	a.leftoverReport = LeftoverReport{}
	return "ok"
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

	// Notify popup + main frontend of the final state once the function
	// returns. ApplyMode has multiple terminal branches (success, cancelled,
	// rollback-success, rollback-failed); doing this in defer guarantees
	// neither surface gets stuck on stale mode/state regardless of which
	// branch we exited through. The config emit uses the latest snapshot
	// from disk, which reflects any rollback that happened above.
	defer func() {
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "config:updated", a.config.GetConfig())
		}
	}()

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
		a.proxy.SetTunStack(cfg.Settings.EffectiveTunStack())
		result := a.proxy.Connect(
			a.ctx,
			prevProxy,
			proxy.ProxyMode(mode),
			proxy.RoutingMode(cfg.RoutingRules.Mode),
			cfg.RoutingRules.Whitelist,
			cfg.RoutingRules.AppWhitelist,
			cfg.RoutingRules.AppForceVPN,
			cfg.Settings.KillSwitch,
			cfg.Settings.LocalPort,
			cfg.Settings.ListenLAN,
			modeSwitchDNS,
			cfg.Settings.TunIPv4,
			"",
			cfg.Settings.EffectiveDNSLeakProtection(),
		cfg.Settings.EnableIPv6,
		)
		if result.Success {
			serverName := fmt.Sprintf("%s:%d", status.CurrentProxy.IP, status.CurrentProxy.Port)
			if a.tray != nil {
				a.tray.SetConnectedProxy(a.resolveProxyID(*status.CurrentProxy), serverName)
			}
			a.setWindowTitleConnected(*status.CurrentProxy)
			wailsRuntime.EventsEmit(a.ctx, "proxy:connected", *status.CurrentProxy)
		} else if result.ErrorCode == "cancelled" {
			// User explicitly cancelled the connect (Disconnect/CancelConnect).
			// Do NOT rollback to the previous mode — that would silently
			// reconnect behind the user's back. Restore the saved mode value
			// to keep config consistent with "disconnected" state and emit a
			// disconnect event so the UI/tray stay in sync.
			cfg.Settings.Mode = previousMode
			_ = a.config.SaveConfig(cfg)
			if a.tray != nil {
				a.tray.SetDisconnected()
			}
			a.setWindowTitleDisconnected()
			wailsRuntime.EventsEmit(a.ctx, "proxy:disconnected", nil)
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
				cfg.RoutingRules.AppForceVPN,
				cfg.Settings.KillSwitch,
				cfg.Settings.LocalPort,
				cfg.Settings.ListenLAN,
				modeSwitchDNS,
				cfg.Settings.TunIPv4,
				"",
				cfg.Settings.EffectiveDNSLeakProtection(),
		cfg.Settings.EnableIPv6,
			)
			if rollback.Success {
				if a.tray != nil {
					a.tray.SetConnectedProxy(a.resolveProxyID(prevProxy), fmt.Sprintf("%s:%d", prevProxy.IP, prevProxy.Port))
				}
				a.setWindowTitleConnected(prevProxy)
				wailsRuntime.EventsEmit(a.ctx, "proxy:connected", prevProxy)
			} else {
				if a.tray != nil {
					a.tray.SetDisconnected()
				}
				a.setWindowTitleDisconnected()
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

// enableKillSwitchFirewall installs OS firewall rules for kill switch when a proxy
// address is known. Caller supplies host:port (IP or resolvable name per platform).
func (a *App) enableKillSwitchFirewall(proxyAddr string, dnsServers []string) error {
	if a.killSwitch == nil || strings.TrimSpace(proxyAddr) == "" {
		return nil
	}
	if err := a.killSwitch.Enable(proxyAddr, dnsServers); err != nil {
		return err
	}
	if a.tray != nil {
		a.tray.SetKillSwitchActive()
	}
	a.setWindowTitleKillSwitch()
	a.log.Warning("[KILL SWITCH] Активирована полная блокировка интернета (firewall)")
	return nil
}

func (a *App) ToggleKillSwitch(enable bool) error {
	if a.proxy == nil {
		return fmt.Errorf("proxy manager not initialized")
	}

	if enable && a.killSwitch != nil {
		// OS firewall engages only when the health watchdog detects an unreachable
		// upstream during an active session — not when toggling this setting.
		a.log.Info("[KILL SWITCH] Включено; блокировка фаервола только если узел станет недоступен во время сессии.")
	}

	if !enable && a.killSwitch != nil && a.killSwitch.IsEnabled() {
		if err := a.killSwitch.Disable(); err != nil {
			a.log.Error(fmt.Sprintf("[KILL SWITCH] Ошибка отключения: %v", err))
		}
		a.log.Info("[KILL SWITCH] Деактивирован")
	}

	return a.proxy.ToggleKillSwitch(enable)
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
	// Кастомные «сайты через VPN» живут в Router независимо от состояния
	// подключения: применяем и в отключённом состоянии, чтобы следующий
	// Connect снапшотнул их в route вместе с block-листами.
	if r := a.proxy.GetRouter(); r != nil {
		r.SetCustomBlockedDomains(rules.CustomBlockedDomains)
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
		rules.AppForceVPN,
	)
	if !result.Success {
		a.log.Error(fmt.Sprintf("Ошибка применения правил маршрутизации: %s", result.Message))
		if a.tray != nil {
			a.tray.SetDisconnected()
		}
		a.setWindowTitleDisconnected()
		wailsRuntime.EventsEmit(a.ctx, "proxy:disconnected", nil)
		return fmt.Errorf("%s", result.Message)
	}

	a.log.Info("[PROXY] Правила маршрутизации применены")
	if a.tray != nil {
		a.tray.SetConnectedProxy(a.resolveProxyID(cur), fmt.Sprintf("%s:%d", cur.IP, cur.Port))
	}
	a.setWindowTitleConnected(cur)
	wailsRuntime.EventsEmit(a.ctx, "proxy:connected", cur)
	return nil
}

// ExportConfig returns a password-encrypted RESULTPROXY2: payload. The
// password is enforced server-side (>= config.MinPasswordLength). UI must
// prompt the user; sending an empty / short string returns
// config.ErrPasswordTooShort.
func (a *App) ExportConfig(password string) (string, error) {
	if a.config == nil {
		return "", fmt.Errorf("config manager not initialized")
	}
	cfg := a.config.GetConfig()
	result, err := config.ExportConfig(cfg, password)
	if err != nil {
		// Warn-level: password validation failures are part of normal UX,
		// not bugs. The user-facing error message comes from the sentinel.
		a.log.Warning(fmt.Sprintf("Ошибка экспорта: %v", err))
		return "", err
	}
	a.log.Success("Конфигурация экспортирована (зашифровано)")
	return result, nil
}

// ImportConfig accepts both RESULTPROXY2: (password required) and the legacy
// RESULTPROXY: prefix (no password — surfaced with a warning).
//
//   - For v2 payloads: pass the user-supplied password. Wrong password
//     returns config.ErrWrongPassword.
//   - For legacy payloads: the first call returns config.ErrLegacyPlaintext
//     so the UI can warn the user that the source export was unencrypted.
//     The UI must re-call with allowLegacy=true once the user confirms.
func (a *App) ImportConfig(data, password string, allowLegacy bool) error {
	if a.config == nil {
		return fmt.Errorf("config manager not initialized")
	}
	imported, err := config.ImportConfig(data, password)
	if err != nil {
		if errors.Is(err, config.ErrLegacyPlaintext) {
			if !allowLegacy {
				// Bubble up so the UI can show the "unencrypted export" warning.
				return err
			}
			// User has acknowledged; fall through and apply.
		} else {
			a.log.Warning(fmt.Sprintf("Ошибка импорта: %v", err))
			return err
		}
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

// PickAppForWhitelist opens a native file dialog so the user can choose an
// application to add to the per-app whitelist. The returned string is the
// canonical entry (basename of the executable on Windows/Linux, resolved
// CFBundleExecutable on macOS) ready to be appended to RoutingRules.AppWhitelist.
//
// Returns an empty string if the user cancels the dialog.
func (a *App) PickAppForWhitelist() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("app not started")
	}
	opts := wailsRuntime.OpenDialogOptions{
		Title: "Select application",
	}
	switch runtime.GOOS {
	case "windows":
		opts.Filters = []wailsRuntime.FileFilter{
			{DisplayName: "Executables (*.exe)", Pattern: "*.exe"},
		}
	case "darwin":
		// Cocoa's NSOpenPanel treats .app bundles as files by default, so the
		// user can pick "Brave.app" directly. Don't set a filter — it would
		// hide the bundles from the dialog.
		opts.DefaultDirectory = "/Applications"
	case "linux":
		opts.DefaultDirectory = "/usr/bin"
	}
	path, err := wailsRuntime.OpenFileDialog(a.ctx, opts)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	return system.NormalizeAppEntry(path), nil
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

// DetectCountry resolves an IP to its ISO-3166 alpha-2 country code via the
// project-controlled GeoLite2 API. The previous implementation hit
// http://ip-api.com over plaintext HTTP, leaking the queried IP and being
// MITM-able. Failures yield "Unknown" so the UI can still render a row.
//
// The country is cached on disk (24h TTL) — a UI that renders flags for
// hundreds of subscription servers triggers at most one network call per
// unique IP per day.
func (a *App) DetectCountry(ip string) (string, error) {
	if a.smartProvider == nil || a.smartProvider.Country == nil {
		// Fallback path: smart provider isn't initialised yet (e.g. before
		// engine boot). Build a one-off client; result still goes through
		// the project API, never third-party.
		cc := proxy.NewCountryClient(a.getUserDataPath())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		country, err := cc.LookupCountryByIP(ctx, ip)
		if err != nil {
			return "Unknown", err
		}
		return country, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	country, err := a.smartProvider.Country.LookupCountryByIP(ctx, ip)
	if err != nil {
		return "Unknown", err
	}
	return country, nil
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

// subscriptionHWID returns the HWID header value for a subscription fetch.
// The raw machine-wide HWID is hashed together with the subscription host
// so the same device looks like a DIFFERENT id to provider A vs. provider B
// — removing the cross-correlation channel a hostile provider ecosystem
// could use to track users. Empty subURL (paste flow) falls back to the
// legacy machine-wide HWID so providers that rely on it for billing /
// device-limit still get a stable value within their own scope.
func (a *App) subscriptionHWID(subURL string) string {
	hwid, err := stableHWIDProvider(a.getUserDataPath())
	if err != nil {
		a.log.Warning(fmt.Sprintf("Не удалось получить HWID для запроса подписки: %v", err))
		return ""
	}
	hwid = strings.TrimSpace(hwid)
	if hwid == "" {
		return ""
	}
	host := subscriptionHostFromURL(subURL)
	if host == "" {
		return hwid
	}
	sum := sha256.Sum256([]byte(hwid + "|" + host + "|resultv-sub-hwid-v2"))
	return hex.EncodeToString(sum[:])
}

// subscriptionHostFromURL returns the lowercase host of a subscription URL,
// or "" if parsing fails. We strip the port so foo.com:8080 and foo.com:443
// share the same HWID (it's the same provider).
func subscriptionHostFromURL(subURL string) string {
	u, err := url.Parse(strings.TrimSpace(subURL))
	if err != nil || u == nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	return host
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

// isPrivateOrLoopback reports whether the given IP belongs to a range that
// the icon-fetch must not reach. We block loopback, link-local, multicast,
// unspecified, and all RFC1918 / unique-local v6 ranges. Without this an
// attacker-controlled subscription server could direct icon fetches at the
// user's LAN (router admin panels, internal services) and use response
// timings / sizes as an oracle.
func isPrivateOrLoopback(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}
	// IsPrivate() handles 10/8, 172.16/12, 192.168/16, fc00::/7. Explicitly
	// also block CGNAT (100.64.0.0/10) and broadcast.
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && (v4[1]&0xC0) == 64 {
			return true
		}
		if v4.Equal(net.IPv4bcast) {
			return true
		}
	}
	return false
}

// safeImageDialer returns a net.Dialer whose Control hook blocks connections
// to private/loopback addresses. Using Control (instead of pre-resolving)
// closes the DNS-rebinding race where a hostile DNS returns a public IP to
// our LookupHost and a private IP to the actual dial.
func safeImageDialer() *net.Dialer {
	return &net.Dialer{
		Timeout: 6 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("icon fetch: bad address %q", address)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("icon fetch: cannot parse %q", host)
			}
			if isPrivateOrLoopback(ip) {
				return fmt.Errorf("icon fetch: blocked private/loopback target %s", ip)
			}
			return nil
		},
	}
}

func inlineSmallImageFromURL(client *http.Client, imageURL string, referer string) string {
	if imageURL == "" {
		return ""
	}
	low := strings.ToLower(imageURL)
	// HTTPS-only: subscription icon URLs come from server-controlled headers
	// or HTML, so a hostile server could embed an http:// URL and observe
	// our request unencrypted. Refusing http here costs nothing — almost
	// every modern site serves icons over HTTPS.
	if !strings.HasPrefix(low, "https://") {
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
	// Referer reveals the subscription URL (often containing a token) to
	// every third-party host that serves an icon. Only attach it when the
	// icon host matches the subscription host — that's the legitimate
	// "icon hosted by the provider" case.
	if rh := sameHostReferer(referer, imageURL); rh != "" {
		req.Header.Set("Referer", rh)
	}
	// Sandbox the connection through our SSRF-aware dialer. The default
	// client passed in shares the cookie jar but we override the transport.
	// DisableKeepAlives + CloseIdleConnections on return guarantees this
	// per-call Transport doesn't park idle sockets in a pool that nothing
	// will ever drain — icon fetches are one-shot and the Transport itself
	// is discarded after this function returns, so any kept-alive socket
	// would just dangle in the OS FD table until GC eventually finalised it.
	transport := &http.Transport{
		DialContext:           safeImageDialer().DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 6 * time.Second,
		DisableKeepAlives:     true,
	}
	defer transport.CloseIdleConnections()
	safeClient := *client
	safeClient.Transport = transport
	resp, err := safeClient.Do(req)
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

// resolveEncryptedSubscriptionURL unwraps an RVSUB1-encrypted URL paste. Returns
// the decrypted URL when the input is encrypted; an empty string when the input
// is plaintext (caller keeps the original); an error when the payload decrypts
// to something that isn't a single http(s) URL.
func resolveEncryptedSubscriptionURL(input string) (string, error) {
	if !proxy.IsEncryptedSubscription(input) {
		return "", nil
	}
	plain, err := proxy.DecryptSubscription(input)
	if err != nil {
		return "", fmt.Errorf("decrypting subscription URL: %w", err)
	}
	plain = strings.TrimSpace(plain)
	if strings.ContainsAny(plain, "\r\n") {
		return "", fmt.Errorf("decrypted payload contains multiple lines — paste it in the content field, not the URL field")
	}
	lower := strings.ToLower(plain)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return "", fmt.Errorf("decrypted payload is not a URL — paste it in the content field instead")
	}
	return plain, nil
}

// impioSubscriptionHost is the impio panel host whose subscription endpoint
// historically required a "/json" path suffix. The panel later dropped the
// suffix (the raw URL now serves JSON directly and ".../json" answers HTTP
// 400), so neither form can be assumed. Var so tests can point it at a local
// server.
var impioSubscriptionHost = "my.impio.space"

// impioAlternateSubscriptionURL returns the impio URL with the "/json"
// suffix toggled — stripped when present, appended when absent — so the
// fetcher can retry the other form after a failure. Returns "" for
// non-impio or unparseable URLs (no alternate to try). This keeps both
// legacy stored ".../json" URLs and fresh raw URLs working regardless of
// which form the panel currently accepts.
func impioAlternateSubscriptionURL(subURL string) string {
	u, err := url.Parse(subURL)
	if err != nil || u.Host != impioSubscriptionHost {
		return ""
	}
	trimmed := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(trimmed, "/json") {
		u.Path = strings.TrimSuffix(trimmed, "/json")
	} else {
		u.Path = trimmed + "/json"
	}
	return u.String()
}

// ErrInsecureSubscription is returned by fetchSubscriptionFromURL when the
// URL uses plaintext http:// and the caller did not pass allowInsecure=true.
// The frontend dispatches on this string to show a "this subscription URL
// is unencrypted, continue anyway?" confirmation.
var ErrInsecureSubscription = errors.New("subscription URL uses plaintext HTTP — credentials and HWID would travel unencrypted")

// isInsecureSubURL reports whether subURL would expose the request over
// plaintext. Non-http(s) URLs (e.g. malformed input) are treated as
// insecure too — they'll fail downstream anyway, but the conservative
// answer avoids ever sending HWID to a non-https endpoint by mistake.
func isInsecureSubURL(subURL string) bool {
	low := strings.ToLower(strings.TrimSpace(subURL))
	return !strings.HasPrefix(low, "https://")
}

// fetchSubscriptionFromURL fetches and parses a subscription. allowInsecure
// must be true to accept http:// URLs; when set, we also suppress the
// x-hwid header because sending a stable device identifier in plaintext is
// exactly the leak the warning is opted into.
func (a *App) fetchSubscriptionFromURL(subURL string, allowInsecure bool) ([]config.ProxyEntry, int64, int64, int64, int64, string, string, []config.RoutingList, map[string]proxy.ParsedRoutingList, error) {
	if resolved, err := resolveEncryptedSubscriptionURL(subURL); err != nil {
		return nil, 0, 0, 0, 0, "", "", nil, nil, err
	} else if resolved != "" {
		subURL = resolved
	}
	insecure := isInsecureSubURL(subURL)
	if insecure && !allowInsecure {
		return nil, 0, 0, 0, 0, "", "", nil, nil, ErrInsecureSubscription
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 15 * time.Second, Jar: jar}
	metadata := a.subscriptionRequestMetadata()

	doFetch := func(userAgent string) ([]config.ProxyEntry, int64, int64, int64, int64, string, string, []config.RoutingList, map[string]proxy.ParsedRoutingList, bool, error) {
		req, err := http.NewRequest(http.MethodGet, subURL, nil)
		if err != nil {
			return nil, 0, 0, 0, 0, "", "", nil, nil, false, fmt.Errorf("creating subscription request: %w", err)
		}
		req.Header.Set("User-Agent", userAgent)
		// Remnawave HWID device identification headers.
		req.Header.Set("x-device-os", metadata.Platform)
		req.Header.Set("x-ver-os", metadata.OSVersion)
		req.Header.Set("x-device-model", metadata.Model)
		// Only attach HWID to HTTPS requests. On plaintext http:// the HWID
		// would be sniffable end-to-end and would link the user's device
		// across every network hop and intermediary — the privacy cost
		// outweighs any HWID-based device-limit check the provider does.
		if !insecure && metadata.SendHWID {
			if hwid := a.subscriptionHWID(subURL); hwid != "" {
				req.Header.Set("x-hwid", hwid)
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, 0, 0, 0, 0, "", "", nil, nil, false, fmt.Errorf("fetching subscription: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, 0, 0, 0, 0, "", "", nil, nil, false, fmt.Errorf("subscription returned HTTP %d", resp.StatusCode)
		}

		profileTitle := parseSubscriptionHeaderText(resp.Header.Get("Profile-Title"))
		up, down, tot, exp := parseSubscriptionUserInfoHeader(resp.Header.Get("Subscription-Userinfo"))
		iconURL := resolveSubscriptionIcon(client, subURL, resp.Header)
		routingListsHeader := resp.Header.Get("Routing-Lists")

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, up, down, tot, exp, iconURL, profileTitle, nil, nil, false, fmt.Errorf("reading subscription body: %w", err)
		}
		bodyStr := string(bodyBytes)
		routingLists := proxy.ExtractSubscriptionRoutingLists(routingListsHeader, bodyStr)
		embedded := proxy.ExtractEmbeddedRoutingLists(bodyStr)

		if iconURL == "" && strings.Contains(bodyStr, "<link") {
			if fromBody := pickIconFromSubscriptionHTML(client, subURL, bodyStr); fromBody != "" {
				iconURL = fromBody
			}
		}

		trimmed := strings.TrimSpace(strings.TrimPrefix(bodyStr, "\uFEFF"))
		if trimmed == "" {
			return nil, up, down, tot, exp, iconURL, profileTitle, routingLists, embedded, false, subscriptionEmptyBodyError(resp.Header)
		}

		isJSON := strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")

		entries, err := proxy.ParseSubscriptionBody(bodyStr)
		if err != nil {
			return nil, up, down, tot, exp, iconURL, profileTitle, routingLists, embedded, isJSON, err
		}

		return entries, up, down, tot, exp, iconURL, profileTitle, routingLists, embedded, isJSON, nil
	}

	entries, up, down, tot, exp, iconURL, profileTitle, routingLists, embedded, _, err := doFetch(metadata.UserAgent)
	if err != nil {
		// impio moved its subscription endpoint between ".../json" and the
		// raw path over time; retry the other form once before giving up.
		alt := impioAlternateSubscriptionURL(subURL)
		if alt == "" {
			return nil, 0, 0, 0, 0, "", "", nil, nil, err
		}
		subURL = alt
		var altErr error
		entries, up, down, tot, exp, iconURL, profileTitle, routingLists, embedded, _, altErr = doFetch(metadata.UserAgent)
		if altErr != nil {
			// Report the original failure — it corresponds to the URL the
			// caller actually asked for.
			return nil, 0, 0, 0, 0, "", "", nil, nil, err
		}
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

	// Turn placeholder-host rows (e.g. 0.0.0.0) into SECTION labels; keep order.
	entries = proxy.FinalizeSubscriptionEntries(entries)

	autoCreated := false
	var individualMembers []config.ProxyEntry
	if groups, indv, splitOK := proxy.SplitAutoEntries(entries); splitOK && len(groups) > 0 {
		individualMembers = indv
		heads := make([]config.ProxyEntry, 0, len(groups))
		grouped := make([]config.ProxyEntry, 0, len(entries)-len(indv))
		for _, g := range groups {
			// Rename auto members to "<flag> TYPE #N" labels. N restarts in
			// every group: the numbers are per-pool labels, and members are
			// hidden behind their head anyway.
			memberIDs := make([]string, len(g.Members))
			for i := range g.Members {
				flagEmoji, _ := proxy.StripLeadingFlagEmoji(g.Members[i].Name)
				typeName := strings.ToUpper(g.Members[i].Type)
				if flagEmoji != "" {
					g.Members[i].Name = fmt.Sprintf("%s %s #%d", flagEmoji, typeName, i+1)
				} else {
					g.Members[i].Name = fmt.Sprintf("%s #%d", typeName, i+1)
				}
				memberIDs[i] = g.Members[i].ID
			}

			membersJSON, _ := json.Marshal(map[string]interface{}{"members": memberIDs})
			// Derived from the group name, not from its position: a provider
			// reordering its sections must not shuffle head IDs, which the
			// frontend uses for favourites and last-selected.
			autoID := fmt.Sprintf("%x", crc32.ChecksumIEEE([]byte(subURL+"auto:"+g.Name)))
			heads = append(heads, config.ProxyEntry{
				ID:              autoID,
				Name:            g.Name,
				Type:            "AUTO",
				SubscriptionURL: subURL,
				Provider:        providerName,
				Extra:           json.RawMessage(membersJSON),
			})
			grouped = append(grouped, g.Members...)
		}
		// AUTO entries first, then auto members (hidden), then individual entries.
		entries = make([]config.ProxyEntry, 0, len(heads)+len(grouped)+len(individualMembers))
		entries = append(entries, heads...)
		entries = append(entries, grouped...)
		entries = append(entries, individualMembers...)
		autoCreated = true
	}

	if !autoCreated {
		if sharedName, ok := proxy.ExtractAutoGroupName(entries); len(entries) > 1 && ok {
			memberIDs := make([]string, len(entries))
			for i := range entries {
				flagEmoji, _ := proxy.StripLeadingFlagEmoji(entries[i].Name)
				typeName := strings.ToUpper(entries[i].Type)
				if flagEmoji != "" {
					entries[i].Name = fmt.Sprintf("%s %s #%d", flagEmoji, typeName, i+1)
				} else {
					entries[i].Name = fmt.Sprintf("%s #%d", typeName, i+1)
				}
				memberIDs[i] = entries[i].ID
			}

			membersJSON, _ := json.Marshal(map[string]interface{}{"members": memberIDs})
			autoID := fmt.Sprintf("%x", crc32.ChecksumIEEE([]byte(subURL+"auto")))
			autoEntry := config.ProxyEntry{
				ID:              autoID,
				Name:            sharedName,
				Type:            "AUTO",
				SubscriptionURL: subURL,
				Provider:        providerName,
				Extra:           json.RawMessage(membersJSON),
			}
			entries = append([]config.ProxyEntry{autoEntry}, entries...)
			autoCreated = true
		}
	}
	// One counter for every shape: heads and individuals are visible, members
	// are not. The hand-rolled "1 + len(individualMembers)" assumed a single
	// AUTO head and silently undercounted as soon as there were two.
	visibleCount := visibleSubscriptionCount(entries)

	a.log.Success(fmt.Sprintf("Подписка загружена: %d серверов", visibleCount))
	return entries, up, down, tot, exp, iconURL, profileTitle, routingLists, embedded, nil
}

// FetchSubscription performs a one-off subscription fetch (no persistence).
// allowInsecure must be true for plaintext http:// URLs — see
// ErrInsecureSubscription. The frontend should call this with false first
// and re-call with true only after surfacing the warning to the user.
func (a *App) FetchSubscription(subURL string, allowInsecure bool) (SubscriptionPreview, error) {
	entries, _, _, _, _, _, _, lists, embedded, err := a.fetchSubscriptionFromURL(subURL, allowInsecure)
	lists = append(lists, embeddedRoutingListDeclarations(embedded)...)
	return SubscriptionPreview{Proxies: entries, RoutingLists: lists}, err
}

// ParseSubscriptionText accepts pasted content. When the paste resolves to
// a URL (via RVSUB1 decryption), we still enforce https:// on the resolved
// target — there's no UI-side prompt for paste flows, so plaintext URLs are
// refused outright.
func (a *App) ParseSubscriptionText(text string) (SubscriptionPreview, error) {
	if proxy.IsDeepLink(text) {
		decoded, err := proxy.DecodeDeepLink(text)
		if err != nil {
			return SubscriptionPreview{}, err
		}
		text = decoded
	}
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		entries, _, _, _, _, _, _, lists, embedded, ferr := a.fetchSubscriptionFromURL(text, false)
		lists = append(lists, embeddedRoutingListDeclarations(embedded)...)
		return SubscriptionPreview{Proxies: entries, RoutingLists: lists}, ferr
	}
	if proxy.IsEncryptedSubscription(text) {
		if resolved, err := resolveEncryptedSubscriptionURL(text); err == nil && resolved != "" {
			entries, _, _, _, _, _, _, lists, embedded, ferr := a.fetchSubscriptionFromURL(resolved, false)
			lists = append(lists, embeddedRoutingListDeclarations(embedded)...)
			return SubscriptionPreview{Proxies: entries, RoutingLists: lists}, ferr
		}
	}
	entries, err := proxy.ParseSubscriptionBody(text)
	if err != nil {
		return SubscriptionPreview{}, err
	}
	lists := proxy.ExtractSubscriptionRoutingLists("", text)
	lists = append(lists, embeddedRoutingListDeclarations(proxy.ExtractEmbeddedRoutingLists(text))...)
	return SubscriptionPreview{
		Proxies:      proxy.FinalizeSubscriptionEntries(entries),
		RoutingLists: lists,
	}, nil
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

	// Replay the consent the user gave when adding this subscription. If
	// they accepted plaintext at AddSubscription time, refresh keeps using
	// http; if not, an http URL refresh will fail with ErrInsecureSubscription
	// and the UI must re-prompt before retrying.
	entries, up, down, tot, exp, iconURL, profileTitle, lists, embedded, err := a.fetchSubscriptionFromURL(sub.URL, sub.AllowInsecure)
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
	provided := append(append([]config.RoutingList(nil), lists...), embeddedRoutingListDeclarations(embedded)...)
	if err := a.syncSubscriptionRoutingLists(subID, provided, nil, embedded); err != nil {
		a.log.Warning(fmt.Sprintf("Ошибка синхронизации списков маршрутизации подписки: %v", err))
	}

	visibleCount := len(entries)
	memberIDs := make(map[string]bool)
	for _, e := range entries {
		if e.Type == "AUTO" {
			var extra map[string]interface{}
			if err := json.Unmarshal(e.Extra, &extra); err == nil {
				if mems, ok := extra["members"].([]interface{}); ok {
					for _, m := range mems {
						if ms, ok := m.(string); ok {
							memberIDs[ms] = true
						}
					}
				}
			}
		}
	}
	visibleCount = 0
	for _, e := range entries {
		if !memberIDs[e.ID] {
			visibleCount++
		}
	}

	a.log.Success(fmt.Sprintf("Подписка '%s' обновлена: %d серверов", displayName, visibleCount))
	return entries, nil
}

// AddSubscription stores a new subscription. allowInsecure=true must be
// passed explicitly for http:// URLs after the user has confirmed the
// warning. The consent is persisted on the Subscription record so
// RefreshSubscription doesn't need to re-prompt.
func (a *App) AddSubscription(name, subURL string, allowInsecure bool, source string, disabledListURLs []string) ([]config.ProxyEntry, error) {
	if a.config == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}

	cfg := a.config.GetConfig()
	staleIdx := -1
	for i, s := range cfg.Subscriptions {
		if s.URL != subURL {
			continue
		}
		hasProxies := false
		for _, p := range cfg.Proxies {
			if p.SubscriptionURL == subURL {
				hasProxies = true
				break
			}
		}
		if hasProxies {
			return nil, fmt.Errorf("подписка с этим URL уже добавлена")
		}
		// Subscription record is orphaned (proxies were deleted but the
		// subscription metadata stayed). Drop it so the new import can take
		// its place.
		staleIdx = i
		break
	}
	if staleIdx >= 0 {
		staleSubID := cfg.Subscriptions[staleIdx].ID
		cfg.Subscriptions = append(cfg.Subscriptions[:staleIdx], cfg.Subscriptions[staleIdx+1:]...)
		a.removeSubscriptionRoutingLists(&cfg, staleSubID)
		if err := a.config.SaveConfig(cfg); err != nil {
			return nil, fmt.Errorf("clearing stale subscription: %w", err)
		}
	}

	entries, up, down, tot, exp, iconURL, profileTitle, lists, embedded, err := a.fetchSubscriptionFromURL(subURL, allowInsecure)
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
		Source:          strings.TrimSpace(source),
		// Only mark as allow-insecure when the URL actually is plaintext —
		// no need to flag https:// subscriptions, which would be misleading.
		AllowInsecure: allowInsecure && isInsecureSubURL(subURL),
	}

	for i := range entries {
		entries[i].Provider = displayName
	}

	cfg.Subscriptions = append(cfg.Subscriptions, sub)
	if err := a.config.SaveConfig(cfg); err != nil {
		return nil, fmt.Errorf("saving subscription: %w", err)
	}
	provided := append(append([]config.RoutingList(nil), lists...), embeddedRoutingListDeclarations(embedded)...)
	if err := a.syncSubscriptionRoutingLists(sub.ID, provided, disabledListURLs, embedded); err != nil {
		a.log.Warning(fmt.Sprintf("[ROUTING] Не удалось применить списки подписки %q: %v", displayName, err))
	}

	visibleCount := len(entries)
	memberIDs := make(map[string]bool)
	for _, e := range entries {
		if e.Type == "AUTO" {
			var extra map[string]interface{}
			if err := json.Unmarshal(e.Extra, &extra); err == nil {
				if mems, ok := extra["members"].([]interface{}); ok {
					for _, m := range mems {
						if ms, ok := m.(string); ok {
							memberIDs[ms] = true
						}
					}
				}
			}
		}
	}
	visibleCount = 0
	for _, e := range entries {
		if !memberIDs[e.ID] {
			visibleCount++
		}
	}

	a.log.Success(fmt.Sprintf("Подписка '%s' добавлена: %d серверов", displayName, visibleCount))
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
	a.removeSubscriptionRoutingLists(&cfg, subID)
	if err := a.config.SaveConfig(cfg); err != nil {
		return err
	}
	a.syncRoutingListSpecs()
	return nil
}

// visibleSubscriptionCount returns the number of entries the user actually
// sees in the list view: every entry minus the ones hidden behind an AUTO
// group as members.
func visibleSubscriptionCount(entries []config.ProxyEntry) int {
	memberIDs := make(map[string]bool)
	for _, e := range entries {
		if !strings.EqualFold(e.Type, "AUTO") || len(e.Extra) == 0 {
			continue
		}
		var extra map[string]interface{}
		if err := json.Unmarshal(e.Extra, &extra); err != nil {
			continue
		}
		mems, _ := extra["members"].([]interface{})
		for _, m := range mems {
			if ms, ok := m.(string); ok {
				memberIDs[ms] = true
			}
		}
	}
	visible := 0
	for _, e := range entries {
		if !memberIDs[e.ID] {
			visible++
		}
	}
	return visible
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
	// Release the single-instance lock the moment we commit to quitting (tray
	// "Выход", or the elevation restart for tunnel mode). Otherwise the
	// successor process — the elevated copy, or a user relaunch during our slow
	// shutdown — finds the mutex still held, bounces itself as a "second
	// instance" and exits with no window. Freeing it here lets that process
	// acquire the lock and show a real window.
	system.ReleaseSingletonLock()
}

func (a *App) restoreMainWindow() {
	if a.ctx == nil {
		return
	}
	// If we're already quitting, the window is being torn down on purpose. A
	// late activation (e.g. a second instance pinging us mid-shutdown) must not
	// fight that teardown — and must not reach verifyWindowOrExit, whose
	// os.Exit(0) would abort the in-flight network cleanup in shutdown() and
	// strand the system proxy / DNS / tun.
	a.stateMu.Lock()
	quitting := a.quitRequested
	a.stateMu.Unlock()
	if quitting {
		return
	}
	a.trayHidden.Store(0)
	wailsRuntime.WindowUnminimise(a.ctx)
	wailsRuntime.WindowShow(a.ctx)

	wailsRuntime.WindowSetAlwaysOnTop(a.ctx, true)
	wailsRuntime.WindowSetAlwaysOnTop(a.ctx, false)

	go a.verifyWindowOrExit()
}

// verifyWindowOrExit checks that the main window became visible after a restore
// attempt. If WebView2 has crashed (common after system sleep/suspend), the
// window will never appear even though the process is still alive. In that case
// we exit so the user can relaunch cleanly — the singleton mutex releases on
// exit, unblocking the new instance.
func (a *App) verifyWindowOrExit() {
	time.Sleep(400 * time.Millisecond)
	if !system.IsMainWindowVisible() {
		a.log.Warning("Окно не стало видимым после восстановления — возможно WebView2 завис. Перезапустите приложение.")
		os.Exit(0)
	}
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
		a.setWindowTitleConnected(*status.CurrentProxy)
	} else {
		a.tray.SetDisconnected()
		a.setWindowTitleDisconnected()
	}
	a.tray.UpdateProxyList(a.filterTrayProxiesDiag(cfg.Proxies), selectedID)
}

// filterTrayProxiesDiag wraps filterTrayProxies with structured logging that
// surfaces the two failure modes responsible for "click X, connect to Y":
//
//  1. Duplicate IDs in cfg.Proxies — connectFromTray's linear lookup picks
//     the FIRST match, so the visually different second entry routes to the
//     first one's IP.
//  2. AUTO.Extra.members listing IDs that don't exist in cfg.Proxies
//     ("orphan members") — filter does not hide them because there's
//     nothing to hide, but they're also unreachable from ResolveAutoCandidates.
//
// Both anomalies are silent corruption that the user can only detect by
// observing wrong-server behaviour; logging them here puts a breadcrumb in
// the log so support can immediately tell what's wrong with a saved config.
func (a *App) filterTrayProxiesDiag(proxies []config.ProxyEntry) []config.ProxyEntry {
	if len(proxies) == 0 {
		return proxies
	}
	// ID duplicate detection.
	idCounts := make(map[string]int, len(proxies))
	for _, p := range proxies {
		if p.ID == "" {
			continue
		}
		idCounts[p.ID]++
	}
	for id, n := range idCounts {
		if n > 1 && a.log != nil {
			var names []string
			for _, p := range proxies {
				if p.ID == id {
					name := strings.TrimSpace(p.Name)
					if name == "" {
						name = fmt.Sprintf("%s:%d", p.IP, p.Port)
					}
					names = append(names, name)
				}
			}
			a.log.Warning(fmt.Sprintf("[TRAY] коллизия ID=%s между %d прокси: %s — выбор из трея может попасть не на тот сервер. Удалите дубликаты.",
				id, n, strings.Join(names, " / ")))
		}
	}
	// Orphan AUTO members detection.
	existingIDs := make(map[string]struct{}, len(proxies))
	for _, p := range proxies {
		if p.ID != "" {
			existingIDs[p.ID] = struct{}{}
		}
	}
	for _, p := range proxies {
		if !strings.EqualFold(p.Type, "AUTO") || len(p.Extra) == 0 {
			continue
		}
		var parsed struct {
			Members []string `json:"members"`
		}
		if p.Extra[0] == '"' {
			var s string
			if err := json.Unmarshal(p.Extra, &s); err == nil {
				_ = json.Unmarshal([]byte(s), &parsed)
			}
		} else {
			_ = json.Unmarshal(p.Extra, &parsed)
		}
		orphans := 0
		for _, mid := range parsed.Members {
			if mid == "" {
				continue
			}
			if _, ok := existingIDs[mid]; !ok {
				orphans++
			}
		}
		if orphans > 0 && a.log != nil {
			autoLabel := strings.TrimSpace(p.Name)
			if autoLabel == "" {
				autoLabel = p.ID
			}
			a.log.Warning(fmt.Sprintf("[TRAY] AUTO «%s» ссылается на %d несуществующих узлов из %d — обновите подписку, иначе клик по AUTO попадёт не туда",
				autoLabel, orphans, len(parsed.Members)))
		}
	}
	return filterTrayProxies(proxies)
}

// filterTrayProxies returns the proxy entries shown in the tray Servers
// submenu.
//
// Decision (user-driven, 2026-05-28 follow-up #2): the tray shows ONLY the
// "individual" servers carrying their original subscription name — never
// AUTO heads, never AUTO members. Specifically it hides:
//
//   - AUTO group heads (Type == "AUTO") — auto-routing silently picks a
//     different-country member, so showing the head in the tray caused
//     "click Austria, connect to Germany".
//   - AUTO members — entries the subscription packed into the auto-group
//     and that the import path renamed to generic "<flag> TYPE #N" labels.
//     They duplicate the same backends as the human-readable individuals
//     (subscriptions like impVPN/Remnawave commonly emit both), so showing
//     them just clutters the tray with anonymous rows.
//   - SECTION entries — subscription group labels with no IP/Port, never
//     connectable.
//
// AUTO heads (and the routing they perform across renamed members) remain
// available from the main React UI. The tray is the "I know exactly which
// server I want, by name" surface; the main window is the "let the app
// pick" surface.
func filterTrayProxies(proxies []config.ProxyEntry) []config.ProxyEntry {
	if len(proxies) == 0 {
		return proxies
	}
	memberIDs := make(map[string]struct{})
	for _, p := range proxies {
		if !strings.EqualFold(p.Type, "AUTO") || len(p.Extra) == 0 {
			continue
		}
		var parsed struct {
			Members []string `json:"members"`
		}
		if p.Extra[0] == '"' {
			var s string
			if err := json.Unmarshal(p.Extra, &s); err == nil {
				_ = json.Unmarshal([]byte(s), &parsed)
			}
		} else {
			_ = json.Unmarshal(p.Extra, &parsed)
		}
		for _, id := range parsed.Members {
			if id != "" {
				memberIDs[id] = struct{}{}
			}
		}
	}
	out := make([]config.ProxyEntry, 0, len(proxies))
	for _, p := range proxies {
		t := strings.ToUpper(strings.TrimSpace(p.Type))
		if t == "SECTION" {
			continue
		}
		if t == "AUTO" {
			continue
		}
		if _, isMember := memberIDs[p.ID]; isMember {
			continue
		}
		out = append(out, p)
	}
	return out
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

// proxyLogLabel names a proxy entry for the log without exposing a
// subscription provider's backend address.
//
// A provider's node domains and IPs are theirs, not ours to publish: the log
// is user-visible and exportable, so one shared log hands out the whole
// backend list. internal/proxy has enforced this from the start — every
// address it prints sits behind `SubscriptionURL == ""` (manager.Connect,
// BuildTunnelModeConfig's readiness line) and newSingBoxLogWriter scrubs the
// server out of engine output — but app.go's AUTO paths printed member
// host:port directly. This is the missing guard, in one place so the next
// caller inherits it.
//
// Manual entries keep "name (host:port)": the user owns that server, already
// sees the address in the UI, and there the address IS the diagnostic.
func proxyLogLabel(p config.ProxyEntry) string {
	name := strings.TrimSpace(p.Name)
	country := strings.ToUpper(strings.TrimSpace(p.Country))
	addr := strings.TrimSpace(p.IP)

	if strings.TrimSpace(p.SubscriptionURL) == "" && addr != "" {
		if name != "" {
			return fmt.Sprintf("«%s» (%s:%d)", name, addr, p.Port)
		}
		return fmt.Sprintf("%s:%d", addr, p.Port)
	}

	switch {
	case name != "" && country != "":
		return fmt.Sprintf("«%s» (%s)", name, country)
	case name != "":
		return fmt.Sprintf("«%s»", name)
	case country != "":
		return fmt.Sprintf("узел (%s)", country)
	default:
		// Nothing safe left to say. Naming the node at all beats falling back
		// to the address, which is the one thing that must not appear.
		return "узел подписки"
	}
}

// formatAutoPickLine reports which member an AUTO group resolved to. It is the
// single surviving AUTO diagnostic: the per-member probe table that used to
// accompany it listed every node's host:port and RTT, which meant a routine
// tray click dumped the provider's entire backend list into the log.
func formatAutoPickLine(autoLabel string, count int, pick config.ProxyEntry) string {
	return fmt.Sprintf("[PROXY] AUTO «%s»: кандидатов %d, первый — %s",
		strings.TrimSpace(autoLabel), count, proxyLogLabel(pick))
}

// formatAutoSweepTimingLine reports how long the ranking took. Counts and
// milliseconds only: this line lands in a log the user can export, so it must
// never name a node. See proxyLogLabel for the same rule on the pick line.
func formatAutoSweepTimingLine(groupName string, probed int, diag proxy.AutoProbeDiagnostics) string {
	if diag.FromCache {
		return fmt.Sprintf("[PROXY] AUTO «%s»: результат из кэша (%d узлов), опрос не потребовался",
			strings.TrimSpace(groupName), probed)
	}
	// "фаза 2 0ms" would read two ways — instantaneous, or never started (phase
	// 1 found nobody to shortlist). An empty Phase2 tells the two apart, so say
	// which one it is instead of leaving the reader to guess from a plausible
	// but impossible zero.
	phase2 := fmt.Sprintf("%dms", diag.Phase2Dur.Milliseconds())
	if len(diag.Phase2) == 0 {
		phase2 = "не запускалась"
	}
	return fmt.Sprintf("[PROXY] AUTO «%s»: опрошено %d узлов — фаза 1 %dms, фаза 2 %s",
		strings.TrimSpace(groupName), probed,
		diag.Phase1Dur.Milliseconds(), phase2)
}

// extractAutoMembers reads the member ID list out of an AUTO head's Extra.
// Providers' Extra reaches us in two shapes — raw JSON object, or that object
// encoded as a JSON string — so both are handled.
func extractAutoMembers(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var parsed struct {
		Members []string `json:"members"`
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			_ = json.Unmarshal([]byte(s), &parsed)
		}
	} else {
		_ = json.Unmarshal(raw, &parsed)
	}
	return parsed.Members
}

// getLastAutoNodeKey reads lastAutoNodeKey under RLock. See the field comment
// on App.lastAutoNodeKeyMu for why this can't be a bare field read: tray
// clicks run concurrently, one goroutine's read here can race another's write
// in setLastAutoNodeKey.
func (a *App) getLastAutoNodeKey() string {
	a.lastAutoNodeKeyMu.RLock()
	defer a.lastAutoNodeKeyMu.RUnlock()
	return a.lastAutoNodeKey
}

// setLastAutoNodeKey writes lastAutoNodeKey under Lock. See getLastAutoNodeKey.
func (a *App) setLastAutoNodeKey(key string) {
	a.lastAutoNodeKeyMu.Lock()
	defer a.lastAutoNodeKeyMu.Unlock()
	a.lastAutoNodeKey = key
}

// autoOutcomeKey returns the AutoNodeKey to record a connect outcome under
// for the index-th candidate connectFromTray just tried on proxyID, or "" if
// nothing should be recorded.
//
// Two guards, both mirroring ReportAutoConnectOutcome (the frontend's
// equivalent path):
//
//   - isAuto: a plain (non-AUTO) server has nothing in a.autoCandidates and
//     no AUTO cache to fold a result into — the frontend's retry loop
//     deliberately skips reportAutoConnectOutcome with the same `if (isAuto)`
//     guard (useDaemonControl.js). Without it, connecting to a plain server
//     from the tray would overwrite the global lastAutoNodeKey with a key no
//     AUTO group's cache contains (every AUTO row would then revert to
//     showing the member minimum) and write a junk entry to node_stats.json.
//
//   - the key comes from a.autoCandidates[proxyID][index] — the UNSTAMPED
//     entry RankAutoCandidates produced and ResolveAutoCandidates cached —
//     not from AutoNodeKey(candidate) computed on the identity-stamped copy
//     connectFromTray actually dials. ResolveAutoCandidates overwrites a
//     blank member SubscriptionURL with the head's before returning
//     candidates, and AutoNodeKey hashes SubscriptionURL, so stamping can
//     change the key. Hashing the stamped copy would then produce a key
//     ReportAutoConnectOutcome (which always reads the cache) could never
//     reproduce, splitting one node's history across two keys.
func (a *App) autoOutcomeKey(proxyID string, isAuto bool, index int) string {
	if !isAuto {
		return ""
	}
	a.autoCandidatesMu.Lock()
	cached := a.autoCandidates[proxyID]
	a.autoCandidatesMu.Unlock()
	if index < 0 || index >= len(cached) {
		return ""
	}
	return proxy.AutoNodeKey(cached[index])
}

// ReportAutoConnectOutcome lets the UI retry loop feed real connection results
// into node statistics. The UI connects by candidate address, so the backend
// cannot observe which candidate was tried without being told.
//
// The key is NOT rebuilt from ip/port. AutoNodeKey hashes Username, Password
// and Extra as well as the address, so a key reconstructed from the address
// alone would never match the one the probe recorded, and the two halves of
// a node's history would accumulate under different keys while appearing to
// work. Instead the candidate is looked up by candidateIndex in the list
// ResolveAutoCandidates cached for this proxyID and keyed off the full entry.
//
// candidateIndex is the position the UI iterated to (its retry loop's `i`),
// not a value derived from the address: CDN-fronted and multi-account panels
// can issue several distinct members sharing one host:port:type
// (AutoNodeKey's doc comment), so an address alone does not identify a
// unique candidate. ip/port are still required and checked against the
// cached entry at that index — if the cache was replaced since the UI got
// its list (e.g. a subscription refresh ran ResolveAutoCandidates again),
// the index could now point at a different node. Misattributing a result to
// the wrong node is worse than losing a datapoint, so any mismatch or
// out-of-range index is a silent no-op.
//
// No reason is accepted from the UI: res.message is user-facing text that can
// contain a host:port, and node_stats.json is unencrypted. Failures are
// recorded as the canned "error".
func (a *App) ReportAutoConnectOutcome(proxyID string, candidateIndex int, ip string, port int, ok bool) {
	a.autoCandidatesMu.Lock()
	cached := a.autoCandidates[proxyID]
	a.autoCandidatesMu.Unlock()

	if candidateIndex < 0 || candidateIndex >= len(cached) {
		return
	}
	c := cached[candidateIndex]
	if c.IP != ip || c.Port != port {
		return
	}
	key := proxy.AutoNodeKey(c)
	if ok {
		proxy.RecordConnectOutcome(key, true, "")
		a.setLastAutoNodeKey(key)
	} else {
		proxy.RecordConnectOutcome(key, false, "error")
	}
}

// AutoGroupStatus reports which member an AUTO group currently resolves to and
// its measured RTT, so the UI can show the node actually in use instead of the
// group minimum — one unusually fast member used to set the number for the
// whole group.
type AutoGroupStatus struct {
	NodeName string `json:"nodeName"`
	NodeIP   string `json:"nodeIp"`
	RTTms    int64  `json:"rttMs"`
	Known    bool   `json:"known"`
}

// GetAutoGroupStatus reports the node that proxyID's AUTO group currently
// resolves to (per the last successful connect) and its measured RTT.
//
// lastAutoNodeKey is a single global value, not scoped to one group: with more
// than one AUTO group configured, matching it without checking group
// membership would make every group's row echo whichever group connected
// last. Restricting the search to a.autoCandidates[proxyID] — the member list
// ResolveAutoCandidates cached for THIS group — keeps a match scoped to nodes
// that actually belong to it.
//
// NodeName/NodeIP are read from that cache rather than stored in dedicated
// App fields: both call sites that set lastAutoNodeKey (ReportAutoConnectOutcome,
// connectFromTray) already hold the full config.ProxyEntry they keyed off, and
// the cache holds that same entry, so there is nothing else to keep in sync.
//
// EWMARTTms can be meaningful even before a successful connect — a node that
// was only probed still has a rolling RTT — so this does not require ConnectOK
// to be nonzero, only that lastAutoNodeKey has been set at all (which happens
// on connect, not on probe; see the field comment on App.lastAutoNodeKeyMu).
func (a *App) GetAutoGroupStatus(proxyID string) AutoGroupStatus {
	key := a.getLastAutoNodeKey()
	if key == "" {
		return AutoGroupStatus{}
	}

	a.autoCandidatesMu.Lock()
	cached := a.autoCandidates[proxyID]
	a.autoCandidatesMu.Unlock()

	var node *config.ProxyEntry
	for i := range cached {
		if proxy.AutoNodeKey(cached[i]) == key {
			node = &cached[i]
			break
		}
	}
	if node == nil {
		return AutoGroupStatus{}
	}

	st := proxy.LookupNodeStat(key)
	if st.EWMARTTms <= 0 {
		return AutoGroupStatus{}
	}
	return AutoGroupStatus{
		NodeName: node.Name,
		NodeIP:   node.IP,
		RTTms:    int64(st.EWMARTTms + 0.5),
		Known:    true,
	}
}

// ResolveAutoCandidates returns the ranked connect candidates for an entry.
//
// This is the single selection entry point: the tray path and the frontend
// both call it. Before it existed the two disagreed — the frontend ranked by
// its cached ping sweep while the tray re-probed serially — so any improvement
// to selection had to be made twice or it silently missed one of them.
//
// Non-AUTO entries resolve to themselves. An AUTO head with no reachable
// member resolves to the head itself, preserving the previous fallback.
func (a *App) ResolveAutoCandidates(proxyID string) []config.ProxyEntry {
	if a.config == nil {
		return nil
	}
	cfg := a.config.GetConfig()

	var head *config.ProxyEntry
	for i := range cfg.Proxies {
		if cfg.Proxies[i].ID == proxyID {
			head = &cfg.Proxies[i]
			break
		}
	}
	if head == nil {
		return nil
	}
	if !strings.EqualFold(head.Type, "AUTO") {
		return []config.ProxyEntry{*head}
	}

	memberIDs := extractAutoMembers(head.Extra)
	members := make([]config.ProxyEntry, 0, len(memberIDs))
	for _, id := range memberIDs {
		if id == "" {
			continue
		}
		for i := range cfg.Proxies {
			if cfg.Proxies[i].ID == id {
				members = append(members, cfg.Proxies[i])
				break
			}
		}
	}

	// The per-member probe table that used to be logged here is gone: it
	// printed one "name [TYPE] host:port — NNms" row per node, so a single tray
	// click published the provider's entire backend list to a log the user can
	// export. The diag phases it was built from are dropped with it; what a
	// user actually needs — which node was picked — is the one line below.
	// CountProbeableNodes, not len(members): the sweep drops SECTION labels and
	// address-less rows before dialing anything (see isProbeableNode), and this
	// line exists precisely to state an honest measured figure — reporting the
	// raw member count would be the one number in it that is not true.
	probeable := proxy.CountProbeableNodes(members)
	ranked, diag := proxy.RankAutoCandidates(a.ctx, members, a.getLastAutoNodeKey())
	if a.log != nil {
		a.log.Info(formatAutoSweepTimingLine(head.Name, probeable, diag))
	}

	// Cache the member entries before the identity stamp below overwrites their
	// ID and Name. ReportAutoConnectOutcome must rebuild the exact same
	// AutoNodeKey the probe used, and that key hashes Username, Password and
	// Extra as well as the address — the UI only ever tells us an address, so
	// without the full entry here the reported outcome would land under a key
	// that never matches and a node's history would split in two.
	a.autoCandidatesMu.Lock()
	a.autoCandidates[head.ID] = ranked
	a.autoCandidatesMu.Unlock()

	if len(ranked) == 0 {
		if a.log != nil {
			// probeable, not len(memberIDs), for the same reason as the timing
			// line above: an unresolvable member ID or a SECTION label was
			// never dialed, so counting it here would overstate how many nodes
			// actually failed.
			a.log.Warning(fmt.Sprintf("[PROXY] AUTO «%s»: ни один из %d узлов не доступен — попытка по AUTO-записи",
				strings.TrimSpace(head.Name), probeable))
		}
		return []config.ProxyEntry{*head}
	}

	// Keep the AUTO head's identity on every candidate: manager.Connect
	// branches its log output on SubscriptionURL != "", and the UI keys the
	// row by the head's ID. Without this the logs leak the member host:port.
	out := make([]config.ProxyEntry, 0, len(ranked))
	for _, c := range ranked {
		c.ID = head.ID
		c.Name = head.Name
		if strings.TrimSpace(c.SubscriptionURL) == "" {
			c.SubscriptionURL = head.SubscriptionURL
		}
		if strings.TrimSpace(c.Provider) == "" {
			c.Provider = head.Provider
		}
		out = append(out, c)
	}
	if a.log != nil {
		// ranked[0], not out[0]: the loop above stamps the AUTO head's name
		// onto every candidate, which would report the group instead of the
		// member that won. proxyLogLabel keeps the member's own name and
		// country while leaving its address out.
		a.log.Info(formatAutoPickLine(head.Name, len(out), ranked[0]))
	}
	return out
}

func (a *App) connectFromTray(proxyID string) error {
	if proxyID == "" || a.config == nil {
		return fmt.Errorf("proxy id is empty")
	}
	cfg := a.config.GetConfig()
	// Detect ID duplicates in cfg.Proxies — they cause the wrong entry to
	// match here and silently route the user to a different server. Manual
	// proxies use Date.now()+index (frontend) while subscription members use
	// time.Now().UnixMilli()+index (backend); both can collide if produced in
	// the same millisecond window. When we hit a duplicate we still pick the
	// first match (legacy behaviour) but warn loudly so the user can clean
	// the config instead of chasing a phantom server selection bug.
	var selected *config.ProxyEntry
	matchCount := 0
	for i := range cfg.Proxies {
		if cfg.Proxies[i].ID == proxyID {
			matchCount++
			if selected == nil {
				selected = &cfg.Proxies[i]
			}
		}
	}
	if selected == nil {
		if a.log != nil {
			a.log.Error(fmt.Sprintf("[TRAY] клик по серверу id=%s — записи нет в конфигурации (%d прокси загружено)", proxyID, len(cfg.Proxies)))
		}
		return fmt.Errorf("proxy %s not found", proxyID)
	}
	if matchCount > 1 && a.log != nil {
		a.log.Warning(fmt.Sprintf("[TRAY] коллизия ID: %d записей делят id=%s — будет использована первая (%s). Удалите дубликаты в списке прокси.",
			matchCount, proxyID, proxyLogLabel(*selected)))
	}

	isAuto := strings.EqualFold(selected.Type, "AUTO")

	clickedLabel := strings.TrimSpace(selected.Name)
	if clickedLabel == "" {
		if selected.IP != "" {
			clickedLabel = fmt.Sprintf("%s:%d", selected.IP, selected.Port)
		} else {
			clickedLabel = proxyID
		}
	}
	if a.log != nil {
		a.log.Info(fmt.Sprintf("[TRAY] клик по «%s» (id=%s, type=%s)", clickedLabel, proxyID, selected.Type))
	}

	// ResolveAutoCandidates is the single selection entry point shared with
	// the frontend: for a non-AUTO entry it resolves to itself, for an AUTO
	// head it returns up to 5 members ranked best-first. Consuming it here
	// (instead of resolving one member and giving up, as the tray used to)
	// gives the tray the same failover the frontend already had.
	candidates := a.ResolveAutoCandidates(proxyID)
	if len(candidates) == 0 {
		return fmt.Errorf("proxy %s not resolvable", proxyID)
	}

	cfg.Settings.LastSelectedProxyID = proxyID
	if err := a.config.SaveConfig(cfg); err != nil {
		return err
	}

	// SubscriptionURL/Provider MUST be propagated: manager.Connect branches
	// log output on SubscriptionURL != "" — without this the user sees the
	// raw member IP in logs whenever they connect via the tray (which is
	// exactly the leak the user complained about for AUTO routing).
	var lastErr error
	for i, candidate := range candidates {
		if i > 0 {
			if a.log != nil {
				a.log.Info(fmt.Sprintf("[TRAY] AUTO: узел не поднялся, пробуем следующий — %s", proxyLogLabel(candidate)))
			}
			// Tear down only the engine here, NOT a.Disconnect(): that also
			// disables the kill switch (app.go's Disconnect, ~line 733-736),
			// which would drop firewall protection for the gap between this
			// failed candidate and the next connect attempt — precisely when
			// there is no tunnel up and the kill switch is the only thing
			// stopping a leak. Leaving the kill switch enabled here is the
			// whole point of this branch. The tray/title/proxy:disconnected
			// event are left alone too: we're still mid-failover, not actually
			// disconnected, so telling the UI otherwise would be wrong.
			if a.proxy != nil {
				_ = a.proxy.Disconnect()
			}
		}
		result, err := a.Connect(proxy.ProxyConfig{
			ID:              candidate.ID,
			IP:              candidate.IP,
			Port:            candidate.Port,
			Type:            candidate.Type,
			Username:        candidate.Username,
			Password:        candidate.Password,
			URI:             candidate.URI,
			Extra:           candidate.Extra,
			SubscriptionURL: candidate.SubscriptionURL,
		}, cfg.RoutingRules, cfg.Settings.KillSwitch)
		// key is "" (and every RecordConnectOutcome/setLastAutoNodeKey call
		// below a silent no-op) unless this is an AUTO group — mirrors the
		// frontend's `if (isAuto)` guard in useDaemonControl.js. A plain
		// server connected from the tray has no AUTO cache to report into;
		// recording one would overwrite the global lastAutoNodeKey with a key
		// no AUTO group's cache contains (every row would then read Known
		// but resolve to nothing) and pile junk entries into node_stats.json.
		// See autoOutcomeKey for why the key comes from the cache, not from
		// AutoNodeKey(candidate) directly.
		key := a.autoOutcomeKey(proxyID, isAuto, i)
		if err != nil {
			// Never pass err.Error() — it embeds the remote address and this
			// store is written to disk unencrypted. sanitizeStatReason in
			// nodestats.go is the backstop, but call sites must not rely on it.
			if key != "" {
				proxy.RecordConnectOutcome(key, false, "error")
			}
			lastErr = err
			continue
		}
		if result.Success {
			if key != "" {
				proxy.RecordConnectOutcome(key, true, "")
				a.setLastAutoNodeKey(key)
			}
			a.refreshTrayProxyList()
			return nil
		}
		// result.Message is a user-facing Russian string that can contain a
		// host:port — same leak concern as err.Error() above.
		if key != "" {
			proxy.RecordConnectOutcome(key, false, "error")
		}
		lastErr = errors.New(result.Message)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no candidate connected")
	}
	// Every candidate failed. Unlike the between-candidates case above, there
	// is no next attempt left to protect, so the reason to keep the kill
	// switch armed and the engine silent is gone — and leaving it that way
	// would strand the user behind firewall rules with no engine running and
	// nothing telling the UI it's disconnected. Run the full Disconnect() here
	// to restore the pre-failover end state: kill switch disabled, tray/title
	// reset, proxy:disconnected emitted. a.Disconnect() itself no-ops when
	// a.proxy is nil.
	_ = a.Disconnect()
	return lastErr
}

// resolveProxyID returns the canonical ID for a ProxyConfig. Callers must
// populate proxyDTO.ID — the (IP, Port, Type) fallback exists only to
// tolerate legacy code paths that built a ProxyConfig manually. The fallback
// is unsafe when two subscriptions share the same address (typical for
// Hysteria2 backbones), so a hit there is logged as a warning to surface
// missing ID propagation early.
func (a *App) resolveProxyID(proxyDTO proxy.ProxyConfig) string {
	if proxyDTO.ID != "" {
		return proxyDTO.ID
	}
	if a.config == nil {
		return ""
	}
	cfg := a.config.GetConfig()
	for _, p := range cfg.Proxies {
		if p.IP == proxyDTO.IP && p.Port == proxyDTO.Port && strings.EqualFold(p.Type, proxyDTO.Type) {
			if a.log != nil {
				a.log.Warning(fmt.Sprintf("[PROXY] resolveProxyID: fallback по (IP,Port,Type) для %s %s → id=%s (ID не передан, возможен выбор не того сервера)", strings.ToUpper(proxyDTO.Type), proxyLogLabel(p), p.ID))
			}
			return p.ID
		}
	}
	return ""
}

// resolveProxyDisplayName returns the human-readable proxy name (e.g.
// "Австрия | TROJAN TCP | №2") used in the window/taskbar title.
// ProxyConfig itself doesn't carry Name — it's stored in config.ProxyEntry,
// so we look it up by ID or (IP, Port, Type). Falls back to "IP:Port".
func (a *App) resolveProxyDisplayName(proxyDTO proxy.ProxyConfig) string {
	if a.config != nil {
		cfg := a.config.GetConfig()
		for _, p := range cfg.Proxies {
			match := (proxyDTO.ID != "" && p.ID == proxyDTO.ID) ||
				(p.IP == proxyDTO.IP && p.Port == proxyDTO.Port && strings.EqualFold(p.Type, proxyDTO.Type))
			if match && strings.TrimSpace(p.Name) != "" {
				return strings.TrimSpace(p.Name)
			}
		}
	}
	return fmt.Sprintf("%s:%d", proxyDTO.IP, proxyDTO.Port)
}

// setWindowTitleConnected updates the OS window/taskbar title to reflect the
// active proxy. Format: "ResultV — {server name}". Safe to call before ctx is
// initialized.
func (a *App) setWindowTitleConnected(proxyDTO proxy.ProxyConfig) {
	if a.ctx == nil {
		return
	}
	wailsRuntime.WindowSetTitle(a.ctx, "ResultV — "+a.resolveProxyDisplayName(proxyDTO))
}

func (a *App) setWindowTitleDisconnected() {
	if a.ctx == nil {
		return
	}
	wailsRuntime.WindowSetTitle(a.ctx, "ResultV")
}

func (a *App) setWindowTitleKillSwitch() {
	if a.ctx == nil {
		return
	}
	wailsRuntime.WindowSetTitle(a.ctx, "ResultV — Kill Switch")
}

func (a *App) initSmartBlockedDomains(userDataPath, rootDir string) {
	if a.proxy == nil {
		return
	}
	cachePath := filepath.Join(userDataPath, "blocked_cache.json")
	cidrCachePath := filepath.Join(userDataPath, "blocked_cidr_cache.json")
	localPaths := []string{
		filepath.Join(rootDir, "list-general.txt"),
		filepath.Join(rootDir, "list-google.txt"),
	}
	a.smartProvider = proxy.NewHTTPBlockedListProvider(userDataPath)
	router := a.proxy.GetRouter()

	// Apply last-session's lists from cache INSTANTLY — no network, so the user
	// waits nothing and Smart mode has its block-list populated before the
	// frontend auto-connect snapshots it into the live route. Loading remotely
	// here (the old behaviour) blocked startup for ~40s on a post-crash restart
	// while a stale DNS override starved the fetch, and connect meanwhile baked in
	// an empty list — sending YouTube & co. direct. The remote refresh now runs
	// in the background, after leftover cleanup, via startSmartBlockedRefresh.
	domRes := proxy.LoadCachedBlockedDomains(cachePath, localPaths...)
	if router != nil && len(domRes.Domains) > 0 {
		router.SetBlockedDomains(domRes.Domains)
	}
	// Пользовательские «сайты через VPN» из конфига объединяются с
	// block-листами до auto-connect — иначе первый route их не увидит.
	if router != nil {
		router.SetCustomBlockedDomains(a.config.GetConfig().RoutingRules.CustomBlockedDomains)
	}
	// Push cached routing-list specs to the proxy manager before auto-connect,
	// mirroring the block-list cache-first load above — the first connect must
	// see any lists fetched in a prior session.
	a.syncRoutingListSpecs()
	if domRes.Country != "" {
		a.log.Info(fmt.Sprintf("[SMART] Источник списков: %s (%s), записей: %d", domRes.Source, strings.ToUpper(domRes.Country), len(domRes.Domains)))
	} else {
		a.log.Info(fmt.Sprintf("[SMART] Источник списков: %s, записей: %d", domRes.Source, len(domRes.Domains)))
	}

	// IP-subnet block-list (Telegram MTProto + Discord voice): these clients
	// dial their servers by IP with no domain/SNI, so domain rules can't catch
	// them — Smart mode needs the ranges. Cache-first, always resolves (static
	// fallback).
	cidrRes := proxy.LoadCachedBlockedCIDRs(cidrCachePath)
	if router != nil && len(cidrRes.CIDRs) > 0 {
		router.SetBlockedCIDRs(cidrRes.CIDRs)
	}
	a.log.Info(fmt.Sprintf("[SMART] IP-подсети (Telegram+Discord): источник %s, записей: %d", cidrRes.Source, len(cidrRes.CIDRs)))

	a.startSmartBlockedRefresh(cachePath, cidrCachePath)
	a.startRoutingListRefresh()
}

// waitForLeftoverCleanup blocks until startup leftover recovery has restored OS
// network state (so the DNS override from a prior crash is gone) or the cap
// elapses — whichever first. Bounded so a stuck/elevation-blocked cleanup never
// strands the refresh; the cache is already applied, so a missed refresh is
// harmless.
func (a *App) waitForLeftoverCleanup(max time.Duration) {
	if a.leftoverDone == nil {
		return
	}
	timer := time.NewTimer(max)
	defer timer.Stop()
	select {
	case <-a.leftoverDone:
	case <-timer.C:
	case <-a.ctx.Done():
	}
}

func (a *App) startSmartBlockedRefresh(cachePath, cidrCachePath string) {
	if a.ctx == nil || a.proxy == nil || a.smartProvider == nil {
		return
	}
	go func() {
		// Initial refresh: wait (bounded) for leftover cleanup so the network
		// fetch runs only after a post-crash DNS override is reverted — otherwise
		// every source times out and we needlessly burn ~40s. Updates the router
		// for the NEXT connect (sing-box has no in-place route reload, so the live
		// session keeps its snapshot; a rare upstream list change applies on the
		// user's next reconnect).
		a.waitForLeftoverCleanup(15 * time.Second)
		a.refreshSmartBlockedOnce(cachePath, cidrCachePath)

		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				a.refreshSmartBlockedOnce(cachePath, cidrCachePath)
			}
		}
	}()
}

// refreshSmartBlockedOnce fetches the block-lists remotely and, on success,
// applies them to the router (for the next connect) and re-persists the cache.
// Bounded by its own deadline so a slow/hung source can't run unbounded.
func (a *App) refreshSmartBlockedOnce(cachePath, cidrCachePath string) {
	ctx, cancel := context.WithTimeout(a.ctx, 60*time.Second)
	defer cancel()

	res := proxy.RefreshRemoteBlockedDomains(ctx, a.smartProvider, cachePath)
	if res.Err != nil {
		a.log.Warning(fmt.Sprintf("[SMART] Не удалось обновить списки: %v", res.Err))
	} else {
		router := a.proxy.GetRouter()
		if router != nil && len(res.Domains) > 0 {
			router.SetBlockedDomains(res.Domains)
		}
		if res.Country != "" {
			a.log.Info(fmt.Sprintf("[SMART] Списки обновлены (%s), записей: %d", strings.ToUpper(res.Country), len(res.Domains)))
		} else {
			a.log.Info(fmt.Sprintf("[SMART] Списки обновлены, записей: %d", len(res.Domains)))
		}
		// Compile the fresh list into a binary rule-set off the connect path, so
		// the next connect finds it ready instead of paying ~140ms for the
		// compile. Best-effort: the manager compiles on demand anyway.
		if len(res.Domains) > 0 {
			domains := append([]string(nil), res.Domains...)
			go func() {
				if _, err := proxy.CompileSmartRuleSet(a.getUserDataPath(), domains); err != nil {
					a.log.Warning(fmt.Sprintf("[SMART] Не удалось скомпилировать rule-set: %v", err))
				}
			}()
		}
	}

	cidrRes := proxy.ResolveBlockedCIDRs(ctx, a.smartProvider, cidrCachePath)
	if cidrRes.Source == "remote" {
		router := a.proxy.GetRouter()
		if router != nil && len(cidrRes.CIDRs) > 0 {
			router.SetBlockedCIDRs(cidrRes.CIDRs)
		}
		a.log.Info(fmt.Sprintf("[SMART] IP-подсети обновлены, записей: %d", len(cidrRes.CIDRs)))
	} else if cidrRes.Err != nil {
		a.log.Warning(fmt.Sprintf("[SMART] Не удалось обновить IP-подсети: %v", cidrRes.Err))
	}
}

func (a *App) prepareForUpdateInstall() error {
	// Abort any in-flight connect attempt so update teardown is deterministic.
	a.CancelConnect()
	if err := a.Disconnect(); err != nil {
		return fmt.Errorf("disconnect before update install: %w", err)
	}
	// Always call Disable() before installer handover. This mirrors shutdown():
	// stale in-memory state must not leave firewall rules behind.
	if a.killSwitch != nil {
		if err := a.killSwitch.Disable(); err != nil {
			return fmt.Errorf("disable kill switch before update install: %w", err)
		}
	}
	return nil
}

// StartUpdate begins the in-app update: check manifest → download → verify → install.
// Progress and status are emitted as Wails events:
//   - update:progress  { downloaded, total, speedBps }
//   - update:verifying (no payload)
//   - update:verified  (no payload)
//   - update:installing (no payload)
//   - update:failed    { stage, message }
//
// If another update is already in progress this call is a no-op.
func (a *App) StartUpdate() {
	a.updateMu.Lock()
	if a.updateCancel != nil {
		a.updateMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.updateCancel = cancel
	a.updateMu.Unlock()

	go func() {
		defer func() {
			a.updateMu.Lock()
			a.updateCancel = nil
			a.updateMu.Unlock()
		}()

		emit := func(event string, payload interface{}) {
			if a.ctx != nil {
				wailsRuntime.EventsEmit(a.ctx, event, payload)
			}
		}
		failEvent := func(stage, message string) {
			emit("update:failed", map[string]interface{}{
				"stage": stage, "message": message,
			})
		}

		u := updater.New()

		manifest, err := u.Check(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			failEvent("check", err.Error())
			return
		}

		asset := manifest.ResolveAsset()
		if asset == nil {
			failEvent("check", "no in-app update available for this platform")
			return
		}

		path, err := u.Download(ctx, asset, func(downloaded, total int64, speedBps float64) {
			emit("update:progress", map[string]interface{}{
				"downloaded": downloaded,
				"total":      total,
				"speedBps":   speedBps,
			})
		})
		if err != nil {
			if ctx.Err() != nil {
				failEvent("download", "download cancelled")
				return
			}
			failEvent("download", err.Error())
			return
		}

		emit("update:verifying", nil)

		if err := u.Verify(path, asset.SHA256); err != nil {
			failEvent("verify", err.Error())
			return
		}

		emit("update:verified", nil)
		if err := a.prepareForUpdateInstall(); err != nil {
			failEvent("install", err.Error())
			return
		}

		emit("update:installing", nil)
		if err := u.Install(path); err != nil {
			failEvent("install", err.Error())
			return
		}
		// Install is staged. Gracefully quit through Wails so OnShutdown runs
		// and all proxy/kill-switch cleanup mirrors a normal app exit.
		emit("update:restarting", nil)
		time.Sleep(300 * time.Millisecond) // let the event reach the frontend
		// Ensure BeforeClose allows real process exit (not "hide to tray"),
		// otherwise the updater handover script cannot replace the running exe.
		a.markQuitRequested()
		wailsRuntime.Quit(a.ctx)
	}()
}

// CancelUpdate cancels an in-progress update download.
// Has no effect if no update is running.
func (a *App) CancelUpdate() {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	if a.updateCancel != nil {
		a.updateCancel()
	}
}
