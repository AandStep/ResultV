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

package system

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/energye/systray"
	"resultproxy-wails/internal/config"
)

// TrayCallbacks bundles the hooks the tray fires back into the app layer.
// Each callback may be nil — the tray no-ops in that case.
type TrayCallbacks struct {
	// OnShowWindow fires on a left-click of the tray icon.
	OnShowWindow func()

	// OnConnect fires when the user clicks "Connect" in the main menu —
	// the app should connect to the currently-selected proxy.
	OnConnect func()
	// OnDisconnect tears down the active connection.
	OnDisconnect func()
	// OnConnectSelected fires when the user clicks a specific server in the
	// "Servers" submenu. The proxyID is captured by the menu-item closure
	// so this is always the authoritative ID — no fuzzy IP+Port resolution.
	OnConnectSelected func(proxyID string)
	// OnSelectProxy marks a proxy as the user's choice without connecting.
	// Currently invoked alongside OnConnectSelected so the chosen server is
	// remembered as "last selected" for future plain-Connect clicks.
	OnSelectProxy func(proxyID string)
	// OnQuit terminates the application.
	OnQuit func()
	// OnUnexpectedExit fires when the systray goroutine dies without Stop()
	// being called.
	OnUnexpectedExit func()
}

type connectionState int

const (
	connectionStateDisconnected connectionState = iota
	connectionStateConnected
	connectionStateKillSwitch
)

// Tray owns the notification-area icon and the native HMENU (right-click
// menu). A left-click is dispatched to OnShowWindow; right-click is handled
// by energye/systray's default behaviour, which calls TrackPopupMenu on the
// HMENU built in onReady.
type Tray struct {
	mu            sync.Mutex
	icon          []byte
	callbacks     TrayCallbacks
	running       bool
	exited        chan struct{}
	stopRequested bool

	// Static menu items, created once in onReady.
	miConnect    *systray.MenuItem
	miDisconnect *systray.MenuItem
	miServers    *systray.MenuItem // submenu head
	miShowWindow *systray.MenuItem
	miQuit       *systray.MenuItem

	// Server submenu items, keyed by proxy ID. energye/systray cannot
	// physically remove menu items, only Hide() them; on a list refresh we
	// reuse existing items (SetTitle, Check/Uncheck) and Hide() the ones
	// that disappeared. New IDs get freshly added.
	serverItems map[string]*systray.MenuItem

	selectedProxyID  string
	connectedProxyID string
	currentState     connectionState
	fallbackName     string
	proxyLookup      map[string]config.ProxyEntry

	// pending* hold UpdateProxyList input that arrived before onReady fired.
	// onReady applies them after the menu skeleton is built.
	pendingApply       bool
	pendingProxies     []config.ProxyEntry
	pendingSelectedID  string
	pendingStateInit   bool
	pendingState       connectionState
	pendingConnectedID string
	pendingFallback    string
}

// NewTray returns a tray bound to the given .ico bytes (used as the
// notification-area icon) and the callback bundle.
func NewTray(icon []byte, cb TrayCallbacks) *Tray {
	return &Tray{
		icon:        icon,
		callbacks:   cb,
		exited:      make(chan struct{}),
		serverItems: make(map[string]*systray.MenuItem),
		proxyLookup: make(map[string]config.ProxyEntry),
	}
}

// Start launches the native systray loop on its own goroutine. The loop
// owns one OS thread (systray pins to it in init()), so we cannot call
// Start twice for the same process.
func (t *Tray) Start() {
	go systray.Run(t.onReady, t.onExit)
}

// Stop requests systray teardown and waits up to two seconds for the
// goroutine to drain.
func (t *Tray) Stop() {
	t.mu.Lock()
	running := t.running
	t.stopRequested = true
	t.mu.Unlock()

	if running {
		systray.Quit()
		select {
		case <-t.exited:
		case <-time.After(2 * time.Second):
		}
	}
}

func (t *Tray) onReady() {
	if len(t.icon) > 0 {
		systray.SetIcon(t.icon)
	}

	// Left-click → restore main window. Right-click intentionally not
	// bound: energye/systray's default handler shows the HMENU built
	// below via TrackPopupMenu when SetOnRClick is unset.
	systray.SetOnClick(func(_ systray.IMenu) {
		t.safeCall(func() {
			if t.callbacks.OnShowWindow != nil {
				t.callbacks.OnShowWindow()
			}
		})
	})

	t.buildMenu()

	// Force the HMENU into Win11 dark theme — no-op on non-Windows.
	enableTrayDarkMode()

	t.mu.Lock()
	t.running = true
	pendingApply := t.pendingApply
	proxies := t.pendingProxies
	selectedID := t.pendingSelectedID
	pendingStateInit := t.pendingStateInit
	state := t.pendingState
	connectedID := t.pendingConnectedID
	fallback := t.pendingFallback
	t.pendingApply = false
	t.pendingProxies = nil
	t.pendingStateInit = false
	t.mu.Unlock()

	if pendingApply {
		t.applyProxyList(proxies, selectedID)
	}
	if pendingStateInit {
		t.applyConnectionState(state, connectedID, fallback)
	}

	t.refreshTooltipAndItems()
}

func (t *Tray) onExit() {
	t.mu.Lock()
	t.running = false
	wasExpected := t.stopRequested
	cb := t.callbacks.OnUnexpectedExit
	t.mu.Unlock()
	close(t.exited)
	if !wasExpected && cb != nil {
		go cb()
	}
}

// buildMenu constructs the static menu skeleton. Server submenu items are
// added lazily by applyProxyList — at startup the Servers head exists but
// is empty.
//
// Every Click callback dispatches its work to a fresh goroutine. The
// energye/systray Windows backend invokes click handlers synchronously on
// its OS-locked message-pump thread (see systray_windows.go WM_COMMAND
// handler), so anything that takes more than a few milliseconds must run
// off-thread or the tray becomes unresponsive AND, for callbacks that end
// up calling proxy.Connect (mode switch, server pick), the long sync call
// blocks the very thread that needs to process Win32 messages the engine
// generates — causing observed silent rollbacks (engine times out, mode
// reverts).
func (t *Tray) buildMenu() {
	t.miConnect = systray.AddMenuItem("Подключить", "Подключиться к выбранному серверу")
	t.miConnect.Click(func() {
		go t.safeCall(func() {
			if t.callbacks.OnConnect != nil {
				t.callbacks.OnConnect()
			}
		})
	})

	t.miDisconnect = systray.AddMenuItem("Отключить", "Разорвать текущее соединение")
	t.miDisconnect.Click(func() {
		go t.safeCall(func() {
			if t.callbacks.OnDisconnect != nil {
				t.callbacks.OnDisconnect()
			}
		})
	})

	systray.AddSeparator()

	t.miServers = systray.AddMenuItem("Серверы", "Выбрать сервер для подключения")

	systray.AddSeparator()

	t.miShowWindow = systray.AddMenuItem("Открыть окно", "Показать главное окно ResultV")
	t.miShowWindow.Click(func() {
		go t.safeCall(func() {
			if t.callbacks.OnShowWindow != nil {
				t.callbacks.OnShowWindow()
			}
		})
	})

	systray.AddSeparator()

	t.miQuit = systray.AddMenuItem("Выход", "Завершить ResultV")
	t.miQuit.Click(func() {
		go t.safeCall(func() {
			if t.callbacks.OnQuit != nil {
				t.callbacks.OnQuit()
			}
		})
	})
}

// SetConnected updates the tooltip with a server label only — used by code
// paths that don't have the full proxy ID handy.
func (t *Tray) SetConnected(serverName string) {
	t.SetConnectedProxy("", serverName)
}

// SetConnectedProxy marks the tray as connected to the given proxy and
// refreshes the tooltip + menu enabled states.
func (t *Tray) SetConnectedProxy(proxyID, serverName string) {
	t.applyConnectionState(connectionStateConnected, proxyID, serverName)
}

func (t *Tray) SetDisconnected() {
	t.applyConnectionState(connectionStateDisconnected, "", "")
}

func (t *Tray) SetKillSwitchActive() {
	t.mu.Lock()
	t.currentState = connectionStateKillSwitch
	running := t.running
	if !running {
		t.pendingStateInit = true
		t.pendingState = connectionStateKillSwitch
		t.pendingConnectedID = t.connectedProxyID
		t.pendingFallback = t.fallbackName
	}
	t.mu.Unlock()
	if running {
		t.refreshTooltipAndItems()
	}
}

func (t *Tray) applyConnectionState(state connectionState, proxyID, fallbackName string) {
	t.mu.Lock()
	t.currentState = state
	if state == connectionStateConnected {
		t.connectedProxyID = proxyID
		if proxyID != "" {
			t.selectedProxyID = proxyID
		}
		t.fallbackName = fallbackName
	} else {
		t.connectedProxyID = ""
		t.fallbackName = ""
	}
	running := t.running
	if !running {
		t.pendingStateInit = true
		t.pendingState = state
		t.pendingConnectedID = t.connectedProxyID
		t.pendingFallback = t.fallbackName
	}
	t.mu.Unlock()
	if running {
		t.refreshTooltipAndItems()
		t.refreshSelectedCheckmark()
	}
}

// UpdateProxyList rebuilds the Servers submenu. The caller is expected to
// pre-filter the list (no AUTO heads, no SECTION entries) — the tray does
// not implement AUTO routing.
func (t *Tray) UpdateProxyList(proxies []config.ProxyEntry, selectedProxyID string) {
	t.mu.Lock()
	running := t.running
	if !running {
		t.pendingApply = true
		// Defensive copy: caller's slice may be mutated after this call.
		t.pendingProxies = append([]config.ProxyEntry(nil), proxies...)
		t.pendingSelectedID = selectedProxyID
		t.mu.Unlock()
		return
	}
	t.mu.Unlock()
	t.applyProxyList(proxies, selectedProxyID)
}

func (t *Tray) applyProxyList(proxies []config.ProxyEntry, selectedProxyID string) {
	t.mu.Lock()
	if selectedProxyID != "" {
		t.selectedProxyID = selectedProxyID
	}
	// Rebuild the lookup map for tooltips.
	t.proxyLookup = make(map[string]config.ProxyEntry, len(proxies))
	for _, p := range proxies {
		t.proxyLookup[p.ID] = p
	}

	seen := make(map[string]struct{}, len(proxies))
	miServers := t.miServers
	t.mu.Unlock()

	if miServers == nil {
		return
	}

	for _, p := range proxies {
		if p.ID == "" {
			continue
		}
		seen[p.ID] = struct{}{}
		label := serverMenuLabel(p)
		tooltip := serverMenuTooltip(p)

		t.mu.Lock()
		item, exists := t.serverItems[p.ID]
		t.mu.Unlock()

		if exists {
			item.SetTitle(label)
			item.SetTooltip(tooltip)
			item.Show()
		} else {
			id := p.ID // capture for closure
			item = miServers.AddSubMenuItemCheckbox(label, tooltip, false)
			item.Click(func() {
				// connectFromTray persists LastSelectedProxyID itself, so a
				// separate OnSelectProxy is redundant. Dispatched to a fresh
				// goroutine for the same reason as the main-menu handlers
				// (see buildMenu comment).
				go t.safeCall(func() {
					if t.callbacks.OnConnectSelected != nil {
						t.callbacks.OnConnectSelected(id)
					}
				})
			})
			t.mu.Lock()
			t.serverItems[p.ID] = item
			t.mu.Unlock()
		}
	}

	// Hide items for proxies no longer in the list. energye/systray has no
	// remove API, so we accept the residue in the underlying HMENU.
	t.mu.Lock()
	for id, item := range t.serverItems {
		if _, ok := seen[id]; !ok {
			item.Hide()
		}
	}
	t.mu.Unlock()

	t.refreshSelectedCheckmark()
	t.refreshTooltipAndItems()
}

// refreshSelectedCheckmark updates the radio-like checkmark in the Servers
// submenu so only the currently-selected (or, when connected, the connected)
// proxy is checked.
func (t *Tray) refreshSelectedCheckmark() {
	t.mu.Lock()
	target := t.connectedProxyID
	if target == "" {
		target = t.selectedProxyID
	}
	items := make(map[string]*systray.MenuItem, len(t.serverItems))
	for k, v := range t.serverItems {
		items[k] = v
	}
	t.mu.Unlock()

	for id, item := range items {
		if id == target {
			item.Check()
		} else {
			item.Uncheck()
		}
	}
}

// refreshTooltipAndItems updates the icon tooltip and the enabled state of
// Connect/Disconnect according to the current connection state.
func (t *Tray) refreshTooltipAndItems() {
	t.mu.Lock()
	state := t.currentState
	tooltip := t.tooltipForStateLocked()
	miConnect := t.miConnect
	miDisconnect := t.miDisconnect
	hasSelection := t.selectedProxyID != ""
	running := t.running
	t.mu.Unlock()

	if !running {
		return
	}
	systray.SetTooltip(tooltip)

	if miConnect != nil {
		if hasSelection && state != connectionStateConnected {
			miConnect.Enable()
		} else {
			miConnect.Disable()
		}
	}
	if miDisconnect != nil {
		if state == connectionStateConnected || state == connectionStateKillSwitch {
			miDisconnect.Enable()
		} else {
			miDisconnect.Disable()
		}
	}
}

func (t *Tray) tooltipForStateLocked() string {
	switch t.currentState {
	case connectionStateKillSwitch:
		return "ResultV — Kill Switch активен"
	case connectionStateConnected:
		return "ResultV — " + t.connectedLabelLocked()
	default:
		return "ResultV — Отключено"
	}
}

func (t *Tray) connectedLabelLocked() string {
	if entry, ok := t.proxyLookup[t.connectedProxyID]; ok {
		name := strings.TrimSpace(entry.Name)
		if name != "" {
			return name
		}
		return fmt.Sprintf("%s:%d", entry.IP, entry.Port)
	}
	if t.fallbackName != "" {
		return t.fallbackName
	}
	return "сервер"
}

// SelectedProxyID returns the proxy currently marked as the user's choice.
func (t *Tray) SelectedProxyID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.selectedProxyID
}

// ConnectedProxyID returns the proxy the engine is currently connected to,
// or "" when disconnected.
func (t *Tray) ConnectedProxyID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connectedProxyID
}

// ConnectionStateLabel returns one of "disconnected", "connected",
// "killswitch".
func (t *Tray) ConnectionStateLabel() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch t.currentState {
	case connectionStateKillSwitch:
		return "killswitch"
	case connectionStateConnected:
		return "connected"
	default:
		return "disconnected"
	}
}

func (t *Tray) safeCall(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[tray] recovered panic: %v", r)
		}
	}()
	fn()
}

// serverMenuLabel renders the menu line for one proxy.
//
// Regular servers:   "🇦🇹  Austria"
// AUTO group heads:  "[AUTO] Austria  ⚡"  (NO country flag prefix, even if
//   the AUTO head somehow has one — auto-routing picks the best-pinging
//   member which may sit in a different country, so showing a flag would
//   mislead the user into thinking they're locking onto that country).
//
// The "[AUTO]" prefix is intentionally ASCII so it renders identically on
// every Windows locale/font combination — Segoe UI Emoji has spotty support
// for some regional-indicator pairs and we don't want the marker to ever
// degrade to invisible.
func serverMenuLabel(p config.ProxyEntry) string {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = fmt.Sprintf("%s:%d", p.IP, p.Port)
	}
	if strings.EqualFold(strings.TrimSpace(p.Type), "AUTO") {
		return "[AUTO] " + name + "  ⚡"
	}
	if flag := flagEmoji(p.Country); flag != "" {
		return flag + "  " + name
	}
	return name
}

// serverMenuTooltip explains in plain Russian what clicking the entry will
// do. For AUTO entries the tooltip names the routing behaviour explicitly
// so the user is not surprised when a click resolves to a different country
// than the AUTO group's display name.
func serverMenuTooltip(p config.ProxyEntry) string {
	if strings.EqualFold(strings.TrimSpace(p.Type), "AUTO") {
		return "Авто-выбор: подключится к лучшему по пингу узлу подписки"
	}
	parts := make([]string, 0, 3)
	if t := strings.TrimSpace(p.Type); t != "" {
		parts = append(parts, strings.ToUpper(t))
	}
	if p.IP != "" && p.Port > 0 {
		parts = append(parts, fmt.Sprintf("%s:%d", p.IP, p.Port))
	}
	return strings.Join(parts, " · ")
}

// flagEmoji returns the regional-indicator emoji pair for a 2-letter ISO
// country code. Returns "" for any input that isn't exactly two ASCII
// letters.
func flagEmoji(country string) string {
	c := strings.ToUpper(strings.TrimSpace(country))
	if len(c) != 2 {
		return ""
	}
	for i := 0; i < 2; i++ {
		if c[i] < 'A' || c[i] > 'Z' {
			return ""
		}
	}
	// 'A' (0x41) → regional indicator 'A' (0x1F1E6). Offset = 0x1F1A5.
	const offset = rune(0x1F1E6 - 'A')
	return string([]rune{rune(c[0]) + offset, rune(c[1]) + offset})
}
