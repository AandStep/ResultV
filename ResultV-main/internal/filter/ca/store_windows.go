// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build windows

package ca

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

const installedMarker = "ResultV AdBlock Root CA"

// IsInstalled reports whether our root CA is in the local machine root store.
func IsInstalled() bool {
	cmd := exec.Command("certutil", "-store", "Root")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), installedMarker) || strings.Contains(string(out), caCN)
}

// Install adds the root certificate to LocalMachine\Root (admin required).
func Install(certPath string) error {
	cmd := exec.Command("certutil", "-addstore", "-f", "Root", certPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certutil install: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Uninstall removes the root CA by subject CN.
func Uninstall() error {
	cmd := exec.Command("certutil", "-delstore", "Root", caCN)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certutil uninstall: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
