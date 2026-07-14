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

//go:build darwin

package system

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	launchAgentLabel    = "com.resultv.autostart"
	launchAgentFileName = launchAgentLabel + ".plist"
	autostartTrayToken  = "--autostart"
)

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentFileName), nil
}

func renderLaunchAgentPlist(exePath string, args []string) string {
	progArgs := append([]string{exePath, autostartTrayToken}, args...)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n")
	b.WriteString("<dict>\n")
	b.WriteString("\t<key>Label</key>\n")
	b.WriteString("\t<string>" + launchAgentLabel + "</string>\n")
	b.WriteString("\t<key>ProgramArguments</key>\n")
	b.WriteString("\t<array>\n")
	for _, a := range progArgs {
		b.WriteString("\t\t<string>" + html.EscapeString(a) + "</string>\n")
	}
	b.WriteString("\t</array>\n")
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	b.WriteString("\t<key>KeepAlive</key>\n\t<false/>\n")
	b.WriteString("\t<key>ProcessType</key>\n\t<string>Interactive</string>\n")
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

func EnableAutostart(exePath string, args ...string) error {
	path, err := launchAgentPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(renderLaunchAgentPlist(exePath, args)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	// Best-effort load. If launchctl isn't available the plist will still be
	// picked up on next login.
	_ = exec.Command("launchctl", "unload", "-w", path).Run()
	if err := exec.Command("launchctl", "load", "-w", path).Run(); err != nil {
		// Not fatal: file is written; macOS will load it on next login.
		return nil
	}
	return nil
}

func DisableAutostart() error {
	path, err := launchAgentPath()
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", "-w", path).Run()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

func IsAutostartEnabled() bool {
	path, err := launchAgentPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// SetRunAsAdminFlag is a no-op on macOS — privilege escalation is handled
// via PolicyKit/SMJobBless at the call site, not via a per-binary flag.
func SetRunAsAdminFlag(_ string, _ bool) error { return nil }
