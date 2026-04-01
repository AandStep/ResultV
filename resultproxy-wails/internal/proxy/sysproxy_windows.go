// Copyright (C) 2026 ResultProxy
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build windows

package proxy

import (
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
)

// newSystemProxy creates its platform-specific SystemProxy implementation.
func newSystemProxy(router *Router) SystemProxy {
	return NewWindowsSystemProxy(router)
}

// WindowsSystemProxy manages Windows system proxy via registry.
// Uses native Go registry API instead of exec("reg add ...").
type WindowsSystemProxy struct {
	router *Router
}

// NewWindowsSystemProxy creates a new Windows system proxy manager.
func NewWindowsSystemProxy(router *Router) *WindowsSystemProxy {
	return &WindowsSystemProxy{router: router}
}

// Set enables the system proxy in Windows Internet Settings registry.
func (w *WindowsSystemProxy) Set(addr string, bypass []string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening Internet Settings registry key: %w", err)
	}
	defer key.Close()

	// ProxyEnable = 1
	if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
		return fmt.Errorf("setting ProxyEnable: %w", err)
	}

	// ProxyServer = addr (e.g. "127.0.0.1:14081")
	if err := key.SetStringValue("ProxyServer", addr); err != nil {
		return fmt.Errorf("setting ProxyServer: %w", err)
	}

	// ProxyOverride = bypass list
	override := w.buildBypassList(bypass)
	if err := key.SetStringValue("ProxyOverride", override); err != nil {
		return fmt.Errorf("setting ProxyOverride: %w", err)
	}

	// Remove AutoConfigURL if present (ignore errors).
	_ = key.DeleteValue("AutoConfigURL")

	// Flush DNS.
	_ = exec.Command("ipconfig", "/flushdns").Run()

	return nil
}

// Disable removes the system proxy from Windows.
func (w *WindowsSystemProxy) Disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening Internet Settings registry key: %w", err)
	}
	defer key.Close()

	// ProxyEnable = 0
	if err := key.SetDWordValue("ProxyEnable", 0); err != nil {
		return fmt.Errorf("disabling ProxyEnable: %w", err)
	}

	// Delete proxy values (ignore errors — they may not exist).
	_ = key.DeleteValue("ProxyServer")
	_ = key.DeleteValue("ProxyOverride")
	_ = key.DeleteValue("AutoConfigURL")

	// Flush DNS.
	_ = exec.Command("ipconfig", "/flushdns").Run()

	return nil
}

// DisableSync performs synchronous cleanup during shutdown.
// This is critical — if the app exits without cleaning proxy settings,
// the user loses internet access.
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

// ApplyKillSwitch sets a dead proxy (127.0.0.1:65535) to block all traffic.
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

	// Remove bypass list — everything goes to dead proxy.
	_ = key.DeleteValue("ProxyOverride")

	// Flush DNS.
	_ = exec.Command("ipconfig", "/flushdns").Run()

	return nil
}

// buildBypassList constructs the ProxyOverride value from domain whitelist.
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
		parts = append(parts, "*."+d)
		parts = append(parts, "*"+d+"*")
	}
	parts = append(parts, "<local>")
	return strings.Join(parts, ";")
}
