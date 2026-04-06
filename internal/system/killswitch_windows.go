// Copyright (C) 2026 ResultProxy
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build windows

package system

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
)

const (
	firewallRuleName = "ResultProxy_KillSwitch"
)

// WindowsKillSwitch blocks internet traffic via Windows Firewall rules.
// Strategy:
//  1. If admin: use netsh advfirewall to block all outbound except to the proxy server.
//  2. Fallback: set dead proxy 127.0.0.1:65535 via registry (handled by sysproxy).
//
// The firewall approach is more reliable — it blocks ALL apps,
// not just those that respect system proxy settings.
type WindowsKillSwitch struct {
	mu      sync.Mutex
	enabled bool
	isAdmin bool
}

// NewKillSwitch creates a platform-specific kill switch.
func NewKillSwitch() KillSwitch {
	return &WindowsKillSwitch{
		isAdmin: IsAdmin(),
	}
}

// Enable activates the kill switch.
// proxyAddr is "ip:port" — traffic to this address is allowed (for the proxy itself).
func (ks *WindowsKillSwitch) Enable(proxyAddr string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if ks.enabled {
		return nil
	}

	if ks.isAdmin {
		if err := ks.enableFirewall(proxyAddr); err != nil {
			return fmt.Errorf("enabling firewall kill switch: %w", err)
		}
	} else {
		return fmt.Errorf("kill switch requires administrator privileges for firewall rules")
	}

	ks.enabled = true
	return nil
}

// Disable deactivates the kill switch and restores connectivity.
func (ks *WindowsKillSwitch) Disable() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if !ks.enabled {
		return nil
	}

	if err := ks.disableFirewall(); err != nil {
		return fmt.Errorf("disabling firewall kill switch: %w", err)
	}

	ks.enabled = false
	return nil
}

// IsEnabled returns whether the kill switch is active.
func (ks *WindowsKillSwitch) IsEnabled() bool {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.enabled
}

// enableFirewall adds Windows Firewall rules that block all outbound traffic
// except to the proxy server and local network.
func (ks *WindowsKillSwitch) enableFirewall(proxyAddr string) error {
	// First remove any existing rules.
	_ = ks.disableFirewall()

	// Rule 1: Block ALL outbound traffic.
	blockCmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+firewallRuleName+"_BlockAll",
		"dir=out",
		"action=block",
		"enable=yes",
		"profile=any",
		"protocol=any",
	)
	if out, err := blockCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("adding block rule: %s: %w", string(out), err)
	}

	// Rule 2: Allow traffic to the proxy server (higher priority).
	if proxyIP := extractValidIP(proxyAddr); proxyIP != "" {
		allowProxyCmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
			"name="+firewallRuleName+"_AllowProxy",
			"dir=out",
			"action=allow",
			"enable=yes",
			"profile=any",
			"protocol=any",
			"remoteip="+proxyIP,
		)
		if out, err := allowProxyCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("adding allow proxy rule: %s: %w", string(out), err)
		}
	}

	// Rule 3: Allow loopback traffic.
	allowLocalCmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+firewallRuleName+"_AllowLocal",
		"dir=out",
		"action=allow",
		"enable=yes",
		"profile=any",
		"protocol=any",
		"remoteip=127.0.0.0/8,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16",
	)
	if out, err := allowLocalCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("adding allow local rule: %s: %w", string(out), err)
	}

	// Rule 4: Allow DNS (needed for proxy resolution).
	allowDNSCmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+firewallRuleName+"_AllowDNS",
		"dir=out",
		"action=allow",
		"enable=yes",
		"profile=any",
		"protocol=udp",
		"remoteport=53",
	)
	if out, err := allowDNSCmd.CombinedOutput(); err != nil {
		// DNS rule is nice-to-have, don't fail.
		_ = out
	}

	return nil
}

// extractValidIP parses the host from "ip:port" and validates it.
// Handles IPv4 ("1.2.3.4:443"), IPv6 brackets ("[::1]:443"), and bare IPs.
// Returns empty string if the address is empty or the IP is invalid.
func extractValidIP(addr string) string {
	if addr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = strings.TrimSpace(addr)
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return ""
	}
	if net.ParseIP(host) == nil {
		return ""
	}
	return host
}

// disableFirewall removes all ResultProxy kill switch firewall rules.
func (ks *WindowsKillSwitch) disableFirewall() error {
	// Delete all rules whose names start with our prefix.
	suffixes := []string{"_BlockAll", "_AllowProxy", "_AllowLocal", "_AllowDNS"}
	for _, suffix := range suffixes {
		cmd := exec.Command("netsh", "advfirewall", "firewall", "delete", "rule",
			"name="+firewallRuleName+suffix,
		)
		_ = cmd.Run() // Ignore errors — rule may not exist.
	}
	return nil
}

// DetectGPOConflict checks if Group Policy is overriding proxy settings.
// Returns true if GPO proxy settings exist (common in corporate environments).
func DetectGPOConflict() bool {
	// Check Machine-level GPO proxy settings.
	cmd := exec.Command("reg", "query",
		`HKLM\Software\Policies\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxySettingsPerUser",
	)
	out, err := cmd.CombinedOutput()
	if err == nil && len(out) > 0 {
		// If this key exists with value 0, proxy is managed per-machine (GPO).
		return true
	}

	// Check if GPO-managed proxy is enabled.
	cmd2 := exec.Command("reg", "query",
		`HKLM\Software\Policies\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyEnable",
	)
	out2, err2 := cmd2.CombinedOutput()
	if err2 == nil && len(out2) > 0 {
		return true
	}

	return false
}
