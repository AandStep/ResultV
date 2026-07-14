// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build linux

package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const desktopEntryFile = "resultv.desktop"

func desktopEntryPath() (string, error) {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "applications", desktopEntryFile), nil
}

func renderDeeplinkDesktopEntry(exePath string) string {
	execLine := quoteDesktopExec(exePath) + " %u"
	var b strings.Builder
	b.WriteString("[Desktop Entry]\n")
	b.WriteString("Type=Application\n")
	b.WriteString("Name=ResultV\n")
	b.WriteString("Comment=ResultV proxy client\n")
	b.WriteString("Exec=" + execLine + "\n")
	b.WriteString("Terminal=false\n")
	b.WriteString("Categories=Network;\n")
	b.WriteString("MimeType=x-scheme-handler/resultv;\n")
	b.WriteString("StartupWMClass=resultv\n")
	return b.String()
}

// RegisterResultVProtocol creates the .desktop entry that maps the
// resultv:// URL scheme to this executable, then registers it with xdg-mime.
// Idempotent: only writes when the file is missing or the Exec line differs.
func RegisterResultVProtocol() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	path, err := desktopEntryPath()
	if err != nil {
		return err
	}
	body := renderDeeplinkDesktopEntry(exe)

	if existing, err := os.ReadFile(path); err == nil && string(existing) == body {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create applications dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	// Best-effort: register the scheme handler and refresh the desktop DB.
	// Both tools may be missing on minimal systems; failures here don't stop
	// the rest of the app from working — argv-based deeplink delivery still
	// works as long as the file manager / browser knows about the entry.
	_ = exec.Command("xdg-mime", "default", desktopEntryFile, "x-scheme-handler/resultv").Run()
	_ = exec.Command("update-desktop-database", filepath.Dir(path)).Run()
	return nil
}
