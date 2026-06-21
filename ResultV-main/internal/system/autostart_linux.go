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

//go:build linux

package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	autostartDesktopFile = "resultv.desktop"
	autostartTrayToken   = "--autostart"
)

func autostartFilePath() (string, error) {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "autostart", autostartDesktopFile), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "autostart", autostartDesktopFile), nil
}

func renderAutostartDesktopEntry(exePath string, args []string) string {
	execLine := quoteDesktopExec(exePath) + " " + autostartTrayToken
	for _, a := range args {
		execLine += " " + quoteDesktopExec(a)
	}
	var b strings.Builder
	b.WriteString("[Desktop Entry]\n")
	b.WriteString("Type=Application\n")
	b.WriteString("Name=ResultV\n")
	b.WriteString("Comment=ResultV proxy client\n")
	b.WriteString("Exec=" + execLine + "\n")
	b.WriteString("Terminal=false\n")
	b.WriteString("X-GNOME-Autostart-enabled=true\n")
	b.WriteString("X-KDE-autostart-after=panel\n")
	b.WriteString("Categories=Network;\n")
	return b.String()
}

// quoteDesktopExec applies the minimal escaping required by the Desktop Entry
// spec for the Exec field: paths with spaces or special chars are wrapped in
// double quotes; embedded backslash and double-quote are backslash-escaped.
func quoteDesktopExec(s string) string {
	if !strings.ContainsAny(s, " \t\n\"\\$`") {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}

func EnableAutostart(exePath string, args ...string) error {
	path, err := autostartFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create autostart dir: %w", err)
	}
	body := renderAutostartDesktopEntry(exePath, args)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func DisableAutostart() error {
	path, err := autostartFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

func IsAutostartEnabled() bool {
	path, err := autostartFilePath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// SetRunAsAdminFlag is a no-op on Linux — privilege escalation is handled via
// pkexec/sudo at the call site, not via a per-binary flag.
func SetRunAsAdminFlag(_ string, _ bool) error { return nil }
