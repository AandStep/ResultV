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
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const (
	internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
)


func newSystemProxy(router *Router) SystemProxy {
	return NewWindowsSystemProxy(router)
}



type WindowsSystemProxy struct {
	router *Router
}


func NewWindowsSystemProxy(router *Router) *WindowsSystemProxy {
	return &WindowsSystemProxy{router: router}
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

	
	_ = hiddenCommand("ipconfig", "/flushdns").Run()

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

	
	_ = hiddenCommand("ipconfig", "/flushdns").Run()

	return nil
}




func (w *WindowsSystemProxy) DisableSync() {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()

	_ = key.SetDWordValue("ProxyEnable", 0)
	_ = key.DeleteValue("ProxyServer")
	_ = key.DeleteValue("ProxyOverride")
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
