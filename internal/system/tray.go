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
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

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

	selectedProxyID  string
	connectedProxyID string
	currentState     connectionState
	fallbackName     string

	rows       []trayServerRow
	rowTitles  map[string]string
	autoHeadOf map[string]string
	// slotIDs[i] is the proxy behind the i-th Servers item, "" for a header.
	slotIDs []string

	// energye/systray orders items by creation and cannot delete them, so
	// the Servers items are positional slots: slot i shows row i, and slots
	// past the end of the list are hidden.
	renderMu      sync.Mutex
	slots         []*traySlot
	serversActive bool
}

// TrayServerGroup is one block of the Servers submenu. An empty Label means
// no header line.
type TrayServerGroup struct {
	Label   string
	Proxies []config.ProxyEntry
}

// TrayServerList is the content of the Servers submenu, in display order.
type TrayServerList struct {
	Groups []TrayServerGroup
	// AutoHeadOf maps an AUTO member to the AUTO head listed in its place:
	// a connection made through the head is reported with the member's ID.
	AutoHeadOf map[string]string
}

type trayServerRow struct {
	proxyID string
	title   string
	icon    string
}

type traySlot struct {
	item    *systray.MenuItem
	visible bool
	header  bool
	title   string
	icon    string
	mark    trayMark
	iconSet bool
}

// NewTray returns a tray bound to the given .ico bytes (used as the
// notification-area icon) and the callback bundle.
func NewTray(icon []byte, cb TrayCallbacks) *Tray {
	return &Tray{
		icon:          icon,
		callbacks:     cb,
		exited:        make(chan struct{}),
		serversActive: true,
	}
}

// Start launches the native systray loop on its own goroutine, pinned to one
// OS thread: the tray window belongs to the thread that created it, and only
// that thread's GetMessage sees its clicks.
// Returns false when no tray host can be reached: energye/systray would
// otherwise panic on a nil D-Bus connection (Linux, elevated instance).
func (t *Tray) Start() bool {
	if !trayHostReachable() {
		return false
	}
	go func() {
		runtime.LockOSThread()
		systray.Run(t.onReady, t.onExit)
	}()
	return true
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
	t.mu.Unlock()

	t.renderServers()
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
	t.mu.Unlock()
	t.refreshTooltipAndItems()
	t.renderServers()
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
	t.mu.Unlock()
	t.refreshTooltipAndItems()
	t.renderServers()
}

// UpdateServerList replaces the content of the Servers submenu.
func (t *Tray) UpdateServerList(list TrayServerList, selectedProxyID string) {
	var rows []trayServerRow
	titles := make(map[string]string)
	for _, g := range list.Groups {
		if g.Label != "" {
			rows = append(rows, trayServerRow{title: g.Label})
		}
		for _, p := range g.Proxies {
			if p.ID == "" {
				continue
			}
			row := trayServerRow{proxyID: p.ID, title: trayServerTitle(p), icon: trayServerIcon(p)}
			rows = append(rows, row)
			titles[p.ID] = row.title
		}
	}

	t.mu.Lock()
	if selectedProxyID != "" {
		t.selectedProxyID = selectedProxyID
	}
	t.rows = rows
	t.rowTitles = titles
	t.autoHeadOf = list.AutoHeadOf
	t.mu.Unlock()

	t.renderServers()
	t.refreshTooltipAndItems()
}

// listedIDLocked maps a proxy ID to the row that stands for it: itself, or
// the AUTO head it was reached through. "" when nothing in the list matches.
func (t *Tray) listedIDLocked(id string) string {
	if id == "" {
		return ""
	}
	if _, ok := t.rowTitles[id]; ok {
		return id
	}
	if head, ok := t.autoHeadOf[id]; ok {
		if _, listed := t.rowTitles[head]; listed {
			return head
		}
	}
	return ""
}

// renderServers brings the Servers items in line with t.rows. Each energye
// call rewrites the native item, so only what changed is touched.
func (t *Tray) renderServers() {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()

	t.mu.Lock()
	rows := t.rows
	parent := t.miServers
	running := t.running
	markKind := trayMarkSelected
	markID := t.listedIDLocked(t.selectedProxyID)
	if t.currentState == connectionStateConnected {
		markKind = trayMarkConnected
		markID = t.listedIDLocked(t.connectedProxyID)
	}
	t.mu.Unlock()

	if !running || parent == nil {
		return
	}

	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.proxyID
		header := row.proxyID == ""
		mark := trayMarkNone
		if !header && row.proxyID == markID {
			mark = markKind
		}

		if i == len(t.slots) {
			t.slots = append(t.slots, t.newServerSlot(parent, i, row.title))
		}
		s := t.slots[i]
		if !s.visible {
			s.item.Show()
			s.visible = true
		}
		if s.title != row.title {
			s.item.SetTitle(trayMenuText(row.title))
			s.title = row.title
		}
		if !s.iconSet || s.header != header || s.icon != row.icon || s.mark != mark {
			if header {
				setTrayItemIcon(s.item, trayHeaderIcon())
			} else {
				setTrayItemIcon(s.item, trayMenuIcon(row.icon, mark))
			}
			s.icon, s.mark, s.iconSet = row.icon, mark, true
		}
		if s.header != header {
			if header {
				s.item.Disable()
			} else {
				s.item.Enable()
			}
			s.header = header
		}
	}
	for _, s := range t.slots[len(rows):] {
		if s.visible {
			s.item.Hide()
			s.visible = false
		}
	}

	if active := len(rows) > 0; active != t.serversActive {
		if active {
			parent.Enable()
		} else {
			parent.Disable()
		}
		t.serversActive = active
	}

	t.mu.Lock()
	t.slotIDs = ids
	t.mu.Unlock()
}

// newServerSlot adds the next Servers item. Its click resolves the proxy
// through slotIDs, because the slot shows whatever row lands at its index.
func (t *Tray) newServerSlot(parent *systray.MenuItem, index int, title string) *traySlot {
	item := parent.AddSubMenuItem(trayMenuText(title), "")
	item.Click(func() {
		t.mu.Lock()
		var id string
		if index < len(t.slotIDs) {
			id = t.slotIDs[index]
		}
		t.mu.Unlock()
		if id == "" {
			return
		}
		// connectFromTray persists LastSelectedProxyID itself, so a separate
		// OnSelectProxy is redundant. Dispatched to a fresh goroutine for the
		// same reason as the main-menu handlers (see buildMenu comment).
		go t.safeCall(func() {
			if t.callbacks.OnConnectSelected != nil {
				t.callbacks.OnConnectSelected(id)
			}
		})
	})
	return &traySlot{item: item, visible: true, title: title}
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
	if id := t.listedIDLocked(t.connectedProxyID); id != "" {
		return t.rowTitles[id]
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

// trayServerTitle mirrors formatProxyDisplayName in the main window: the
// flag the provider put in front of the name is drawn as the icon, so the
// emoji goes, and so does a country code repeating it.
func trayServerTitle(p config.ProxyEntry) string {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return fmt.Sprintf("%s:%d", p.IP, p.Port)
	}
	s := name
	if _, rest, ok := cutFlagEmoji(s); ok {
		s = strings.TrimLeftFunc(rest, unicode.IsSpace)
	}
	if cc := trayCountry(p); cc != "" && len(s) > 2 && strings.EqualFold(s[:2], cc) {
		if r, _ := utf8.DecodeRuneInString(s[2:]); unicode.IsSpace(r) {
			if next := strings.TrimSpace(s[2:]); next != "" {
				s = next
			}
		}
	}
	if s = strings.TrimSpace(s); s == "" {
		return name
	}
	return s
}

func trayServerIcon(p config.ProxyEntry) string {
	if strings.EqualFold(strings.TrimSpace(p.Type), "AUTO") {
		return trayIconAuto
	}
	if cc := trayCountry(p); cc != "" && trayHasIcon(cc) {
		return cc
	}
	return trayIconGlobe
}

// trayCountry returns the lowercase ISO code of the server's country: the
// Country field (a code or a flag emoji), else the flag the name starts with.
func trayCountry(p config.ProxyEntry) string {
	c := strings.TrimSpace(p.Country)
	if len(c) == 2 && isASCIILetter(c[0]) && isASCIILetter(c[1]) {
		return strings.ToLower(c)
	}
	if cc, _, ok := cutFlagEmoji(c); ok {
		return cc
	}
	if cc, _, ok := cutFlagEmoji(strings.TrimSpace(p.Name)); ok {
		return cc
	}
	return ""
}

// cutFlagEmoji splits a leading regional-indicator pair off s and returns it
// as a lowercase country code.
func cutFlagEmoji(s string) (cc, rest string, ok bool) {
	const first, last = 0x1F1E6, 0x1F1FF
	r1, n1 := utf8.DecodeRuneInString(s)
	if r1 < first || r1 > last {
		return "", s, false
	}
	r2, n2 := utf8.DecodeRuneInString(s[n1:])
	if r2 < first || r2 > last {
		return "", s, false
	}
	return string([]rune{'a' + r1 - first, 'a' + r2 - first}), s[n1+n2:], true
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
