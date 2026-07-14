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

//go:build windows

package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	proxySnapshotFile   = "proxy-snapshot.json"
)

var (
	modWinINET            = windows.NewLazyDLL("wininet.dll")
	procInternetSetOption = modWinINET.NewProc("InternetSetOptionW")
)

const (
	internetOptionSettingsChanged = 39 // INTERNET_OPTION_SETTINGS_CHANGED
	internetOptionRefresh         = 37 // INTERNET_OPTION_REFRESH
)

// notifyWinINETSettingsChanged tells running WinINET clients (browsers, store
// apps, …) to re-read the system proxy configuration. Writing the HKCU Internet
// Settings registry values is NOT enough on its own: WinINET caches the proxy
// in-process, so without this broadcast a browser keeps routing through the
// (now dead) local proxy port after we disable it — the registry reads
// ProxyEnable=0 yet the internet stays broken until the app is relaunched.
// Best-effort: ignore errors (the call only fails if wininet.dll is unavailable,
// which never happens on a real desktop).
func notifyWinINETSettingsChanged() {
	procInternetSetOption.Call(0, internetOptionSettingsChanged, 0, 0)
	procInternetSetOption.Call(0, internetOptionRefresh, 0, 0)
}

// proxySnapshot is the on-disk marker written whenever we enable the system
// proxy. Its presence at startup means a previous run set the proxy and never
// cleanly disabled it (crash / force-kill) — the anchor for leftover cleanup.
type proxySnapshot struct {
	Addr string `json:"addr"`
}

func newSystemProxy(router *Router) SystemProxy {
	return NewWindowsSystemProxy(router)
}

type WindowsSystemProxy struct {
	router       *Router
	snapshotPath string
}

func NewWindowsSystemProxy(router *Router) *WindowsSystemProxy {
	return &WindowsSystemProxy{
		router:       router,
		snapshotPath: filepath.Join(resultProxyDataDir(), proxySnapshotFile),
	}
}

func (w *WindowsSystemProxy) Set(addr string, bypass []string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening Internet Settings registry key: %w", err)
	}
	defer key.Close()

	if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
		return fmt.Errorf("setting ProxyEnable: %w", err)
	}

	if err := key.SetStringValue("ProxyServer", addr); err != nil {
		return fmt.Errorf("setting ProxyServer: %w", err)
	}

	override := w.buildBypassList(bypass)
	if err := key.SetStringValue("ProxyOverride", override); err != nil {
		return fmt.Errorf("setting ProxyOverride: %w", err)
	}

	_ = key.DeleteValue("AutoConfigURL")

	// Persist a marker so a crash/force-kill leftover can be detected and
	// offered for cleanup at next launch (see LeftoverActive). Best-effort:
	// failure only costs us the detection, not the proxy itself.
	_ = writeProxySnapshot(w.snapshotPath, addr)

	_ = hiddenCommand("ipconfig", "/flushdns").Run()

	notifyWinINETSettingsChanged()

	return nil
}

func (w *WindowsSystemProxy) Disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening Internet Settings registry key: %w", err)
	}
	defer key.Close()

	if err := key.SetDWordValue("ProxyEnable", 0); err != nil {
		return fmt.Errorf("disabling ProxyEnable: %w", err)
	}

	_ = key.DeleteValue("ProxyServer")
	_ = key.DeleteValue("ProxyOverride")
	_ = key.DeleteValue("AutoConfigURL")

	_ = os.Remove(w.snapshotPath)

	_ = hiddenCommand("ipconfig", "/flushdns").Run()

	_ = hiddenCommand("netsh", "winhttp", "reset", "proxy").Run()

	notifyWinINETSettingsChanged()

	return nil
}

func (w *WindowsSystemProxy) DisableSync() {
	// Clear the registry BEFORE removing the leftover marker. If the process is
	// killed mid-way (the 10s shutdown force-exit), the worst state is then
	// "marker present, ProxyEnable=0" — which LeftoverActive() correctly treats
	// as no leftover. The reverse order could leave "marker gone, ProxyEnable=1"
	// — an undetectable stale proxy that startup recovery can't see.
	if key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE); err == nil {
		_ = key.SetDWordValue("ProxyEnable", 0)
		_ = key.DeleteValue("ProxyServer")
		_ = key.DeleteValue("ProxyOverride")
		key.Close()
	}

	_ = os.Remove(w.snapshotPath)

	// Push the change to running WinINET clients — without this a browser keeps
	// using the dead local proxy and the user has "no internet" despite a clean
	// registry. This is the fast path used at quit, so we skip the slower
	// flushdns/winhttp-reset that Disable() runs; the broadcast is what actually
	// un-sticks live apps.
	notifyWinINETSettingsChanged()
}

func (w *WindowsSystemProxy) ApplyKillSwitch() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening registry key for kill switch: %w", err)
	}
	defer key.Close()

	if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
		return fmt.Errorf("enabling kill switch proxy: %w", err)
	}
	if err := key.SetStringValue("ProxyServer", "127.0.0.1:65535"); err != nil {
		return fmt.Errorf("setting kill switch proxy address: %w", err)
	}

	_ = key.DeleteValue("ProxyOverride")

	_ = hiddenCommand("ipconfig", "/flushdns").Run()

	notifyWinINETSettingsChanged()

	return nil
}

func (w *WindowsSystemProxy) buildBypassList(whitelist []string) string {
	if len(whitelist) == 0 {
		return "<local>"
	}

	safeList := w.router.GetSafeOSWhitelist(whitelist)
	if len(safeList) == 0 {
		return "<local>"
	}

	var parts []string
	for _, d := range safeList {
		parts = append(parts, d)
		parts = append(parts, "*."+d)
	}
	parts = append(parts, "<local>")
	return strings.Join(parts, ";")
}

func hiddenCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

// LeftoverActive reports whether a previous run left the system proxy enabled.
// True only when both signals agree: our marker file exists (we set the proxy
// and never cleanly disabled it) AND the OS still has ProxyEnable=1 (the user
// hasn't already turned it off via Windows settings). This avoids prompting to
// "remove leftovers" when there is nothing left to remove.
func (w *WindowsSystemProxy) LeftoverActive() bool {
	if _, err := os.Stat(w.snapshotPath); err != nil {
		return false
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.QUERY_VALUE)
	if err != nil {
		// Can't verify — assume a leftover rather than silently stranding the
		// user behind a dead proxy.
		return true
	}
	defer key.Close()
	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil {
		return true
	}
	return enabled == 1
}

// writeProxySnapshot atomically writes the leftover marker. Mirrors the DNS
// snapshot writer: temp file + rename so a crash never leaves a half-written
// marker that fails to parse.
func writeProxySnapshot(path, addr string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(proxySnapshot{Addr: addr})
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
